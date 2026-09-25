package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/coursemaps"
	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/secrets"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/zajuna"
)

const (
	CaptureChecklistWorkerID       = "capture-checklist"
	CaptureChecklistTargetWorkerID = "capture-checklist-target"
)

type CaptureChecklistInput struct {
	FichaID      string   `json:"fichaId"`
	Username     string   `json:"username"`
	DocumentType string   `json:"documentType"`
	ItemCodes    []string `json:"itemCodes,omitempty"`
	MaxTargets   int      `json:"maxTargets,omitempty"`
	FullPage     *bool    `json:"fullPage,omitempty"`
	ReuseSession *bool    `json:"reuseSession,omitempty"`
	AutoRenew    *bool    `json:"autoRenew,omitempty"`
}

// fichaCaptureLocks serializes captures of one ficha; entries are dropped
// once nobody holds or waits for them (see keyedLocks).
var fichaCaptureLocks = &keyedLocks{slots: make(map[string]*keyedLock)}

type checklistCaptureFichaStore interface {
	GetFicha(context.Context, string) (sqlite.FichaRecord, error)
}

type checklistActivitySelectionStore interface {
	ListSelectedActivityIDs(context.Context, string) (map[string]bool, error)
}

type checklistRouteReviewStore interface {
	ListRouteReviews(context.Context, string) ([]checklist.RouteReview, error)
}

type CaptureChecklistWorker struct {
	runtime     capture.Runtime
	dataDir     string
	client      authenticatedCaptureClient
	credentials secrets.Store
	mapStore    coursemaps.Store
	fichaStore  checklistCaptureFichaStore
	evidence    evidence.Store
	concurrency int
	// allowPrivateTargets lets same-package tests capture a loopback fixture.
	// It is never set in production: Zajuna routes must not resolve into the
	// local network (see security.ValidateHTTPURL).
	allowPrivateTargets bool

	loadPreferences func(context.Context) CapturePreferences
}

func NewCaptureChecklistWorker(runtime capture.Runtime, dataDir string, client authenticatedCaptureClient, credentials secrets.Store, mapStore coursemaps.Store, fichaStore checklistCaptureFichaStore, evidenceStore evidence.Store) (*CaptureChecklistWorker, error) {
	if strings.TrimSpace(dataDir) == "" || client == nil || credentials == nil || mapStore == nil || fichaStore == nil || evidenceStore == nil {
		return nil, errors.New("capture checklist worker requires runtime, data directory, client, credentials and stores")
	}
	return &CaptureChecklistWorker{
		runtime: runtime, dataDir: dataDir, client: client, credentials: credentials,
		mapStore: mapStore, fichaStore: fichaStore, evidence: evidenceStore,
	}, nil
}

func (w *CaptureChecklistWorker) ID() string { return CaptureChecklistWorkerID }

// SetConcurrency limits parallel Chromium sessions used by checklist fan-out.
// Clamped via jobs.ResolveConcurrency (default 2, range 1–4).
func (w *CaptureChecklistWorker) SetConcurrency(n int) {
	w.concurrency = jobs.ResolveConcurrency(n)
}

func (w *CaptureChecklistWorker) fanoutConcurrency() int {
	if w.concurrency < 1 {
		return jobs.DefaultConcurrency
	}
	return w.concurrency
}

func (w *CaptureChecklistWorker) Execute(ctx context.Context, job jobs.Job, reporter jobs.Reporter) jobs.Result {
	var input CaptureChecklistInput
	if err := json.Unmarshal(job.Input, &input); err != nil {
		return jobs.Result{ErrorCode: "invalid_input", ErrorMessage: "la entrada de captura del checklist no es válida"}
	}
	input.FichaID = strings.TrimSpace(input.FichaID)
	input.Username = strings.TrimSpace(input.Username)
	input.DocumentType = strings.TrimSpace(input.DocumentType)
	if input.FichaID == "" || input.Username == "" {
		return jobs.Result{ErrorCode: "invalid_input", ErrorMessage: "fichaId y usuario de Zajuna son obligatorios"}
	}
	unlock, lockErr := lockFichaCapture(ctx, input.FichaID)
	if lockErr != nil {
		return jobs.Result{ErrorCode: "capture_cancelled", ErrorMessage: lockErr.Error()}
	}
	defer unlock()
	if input.DocumentType == "" {
		input.DocumentType = "CC"
	}
	if !w.runtime.Installed() {
		return jobs.Result{ErrorCode: "browser_not_installed", ErrorMessage: fmt.Sprintf("runtime Chromium no instalado en %s; ejecuta npm run browser:install", w.runtime.Root)}
	}

	ficha, err := w.fichaStore.GetFicha(ctx, input.FichaID)
	if err != nil {
		return jobs.Result{ErrorCode: "ficha_not_found", ErrorMessage: "no se encontró la ficha local seleccionada"}
	}
	record, err := w.mapStore.GetCourseMap(ctx, ficha.CourseID)
	if err != nil {
		return jobs.Result{ErrorCode: "course_map_not_found", ErrorMessage: "la ficha todavía no tiene un mapa de rutas; ejecuta Buscar rutas primero"}
	}
	selectedActivityIDs := map[string]bool(nil)
	selectionStore, hasSelectionStore := w.fichaStore.(checklistActivitySelectionStore)
	if hasSelectionStore {
		selectedActivityIDs, err = selectionStore.ListSelectedActivityIDs(ctx, input.FichaID)
		if err != nil {
			return jobs.Result{ErrorCode: "activity_selection_read_failed", ErrorMessage: fmt.Sprintf("no se pudieron leer las actividades seleccionadas: %v", err), Retryable: true}
		}
	}
	if hasSelectionStore && len(checklist.TechnicalSelectionForRecord(record, selectedActivityIDs)) == 0 && captureRequiresActivitySelection(input.ItemCodes) {
		return jobs.Result{ErrorCode: "activities_not_selected", ErrorMessage: "selecciona primero las actividades técnicas que pertenecen al instructor para filtrar fechas y evidencias"}
	}
	targets, summary, err := checklist.BuildCaptureTargetsForActivities(record, selectedActivityIDs)
	if err != nil {
		return jobs.Result{ErrorCode: "checklist_map_invalid", ErrorMessage: err.Error()}
	}
	if reviewStore, ok := w.fichaStore.(checklistRouteReviewStore); ok {
		reviews, reviewErr := reviewStore.ListRouteReviews(ctx, input.FichaID)
		if reviewErr != nil {
			return jobs.Result{ErrorCode: "route_review_read_failed", ErrorMessage: "no se pudieron leer las revisiones de rutas", Retryable: true}
		}
		targets = checklist.ApplyRouteReviews(targets, reviews)
	} else {
		targets = checklist.ApplyRouteReviews(targets, nil)
	}
	prefs := w.preferences(ctx)
	// An explicit value in the request wins over the saved preference.
	if input.FullPage != nil {
		prefs.FullPage = *input.FullPage
	}
	if input.ReuseSession != nil {
		prefs.ReuseSession = *input.ReuseSession
	}
	if input.AutoRenew != nil {
		prefs.AutoRenew = *input.AutoRenew
	}
	targets = applyCapturePreferences(targets, prefs)
	plannedTargets := targets
	targets = filterCaptureTargets(targets, input.ItemCodes)
	if input.MaxTargets > 0 && len(targets) > input.MaxTargets {
		targets = targets[:input.MaxTargets]
	}
	summary.CaptureUnitCount = len(targets)
	summary.CoverageCount = captureTargetCoverageCount(targets)
	if len(targets) == 0 {
		return jobs.Result{ErrorCode: "checklist_map_empty", ErrorMessage: "el mapa no tiene rutas asociadas a los items seleccionados del checklist"}
	}
	if err := reporter.Progress(ctx, "credentials", 5, "Preparando captura dirigida por checklist"); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
	}
	_ = reporter.Event(ctx, "capture_preferences", "Preferencias de captura aplicadas", prefs)
	password, err := w.credentials.Get(input.Username)
	if err != nil || password == "" {
		return jobs.Result{ErrorCode: "credential_unavailable", ErrorMessage: "no se encontró la contraseña de Zajuna en el almacén seguro"}
	}
	if err := reporter.Progress(ctx, "login", 12, "Validando sesión de Zajuna para las evidencias"); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
	}
	session, err := w.client.Login(ctx, zajuna.Credentials{DocumentType: input.DocumentType, Document: input.Username, Password: password})
	if err != nil {
		return jobs.Result{Retryable: retryableZajunaError(err), ErrorCode: "zajuna_login_failed", ErrorMessage: fmt.Sprintf("no se pudo iniciar sesión para el checklist: %v", err)}
	}
	baseURL, err := url.Parse(session.BaseURL)
	if err != nil || baseURL.Host == "" {
		return jobs.Result{ErrorCode: "invalid_zajuna_session", ErrorMessage: "la sesión de Zajuna no tiene un origen válido"}
	}
	useBrowser := strings.EqualFold(baseURL.Hostname(), "zajuna.sena.edu.co")
	ownerName := ""
	var identitySeed *capture.BrowserSession
	if useBrowser && checklist.RequiresInstructorIdentity(targets) {
		if err := reporter.Progress(ctx, "identity", 16, "Identificando al instructor autenticado para filtrar foros y anuncios"); err != nil {
			return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
		}
		identitySession, identityErr := w.openChecklistBrowserSession(ctx, baseURL, input, password)
		if identityErr != nil {
			return jobs.Result{Retryable: retryableZajunaError(identityErr) && !errors.Is(identityErr, capture.ErrBlockedPage), ErrorCode: "zajuna_browser_login_failed", ErrorMessage: fmt.Sprintf("no se pudo iniciar sesión en Chromium para el checklist: %v", identityErr)}
		}
		ownerName, err = identitySession.AuthenticatedOwnerName(ctx, record.ProfileURL)
		if err != nil {
			identitySession.Close()
			return jobs.Result{ErrorCode: "instructor_identity_unavailable", ErrorMessage: fmt.Sprintf("no se pudo verificar el instructor autenticado: %v", err)}
		}
		identitySeed = identitySession
	}

	targetItemCodes := make(map[string]bool)
	for _, target := range targets {
		for _, itemCode := range coveredItemCodes(target) {
			targetItemCodes[itemCode] = true
		}
	}

	outcomes := make([]targetOutcome, len(targets))
	var completed int64
	concurrency := w.fanoutConcurrency()
	if err := reporter.Progress(ctx, "capture", 18, fmt.Sprintf("Capturando %d evidencias con hasta %d sesiones en paralelo", len(targets), concurrency)); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
	}

	// Contiguous FIFO fan-out of target indices. Each in-flight target uses its
	// own Chromium session when browser auth is required: BrowserSession is not
	// safe to share across goroutines. Trade-off: up to C parallel logins.
	var cookieMu sync.Mutex
	var sessions *browserSessionPool
	if useBrowser {
		if prefs.ReuseSession {
			sessions = newBrowserSessionPool(func(openCtx context.Context) (checklistBrowserSession, error) {
				return w.openChecklistBrowserSession(openCtx, baseURL, input, password)
			})
			defer sessions.closeAll()
			if identitySeed != nil {
				// The identity check already logged in: reuse that session.
				sessions.all = append(sessions.all, identitySeed)
				sessions.release(identitySeed, true)
			}
		} else if identitySeed != nil {
			identitySeed.Close()
			identitySeed = nil
		}
	} else if identitySeed != nil {
		identitySeed.Close()
	}
	fanoutErr := orderedFanout(ctx, len(targets), concurrency, func(taskCtx context.Context, index int) error {
		target := targets[index]
		if err := taskCtx.Err(); err != nil {
			outcomes[index] = targetOutcome{failure: target.ItemCode + ": captura cancelada"}
			return err
		}
		outcome := w.captureChecklistTarget(taskCtx, checklistTargetParams{
			JobID:      job.ID,
			Input:      input,
			Target:     target,
			BaseURL:    baseURL,
			Session:    session,
			Password:   password,
			OwnerName:  ownerName,
			UseBrowser: useBrowser,
			CookieMu:   &cookieMu,
			Sessions:   sessions,
			AutoRenew:  prefs.AutoRenew,
		})
		outcomes[index] = outcome
		done := int(atomic.AddInt64(&completed, 1))
		percent := 18 + ((done * 76) / len(targets))
		_ = reporter.Progress(taskCtx, "capture", percent, fmt.Sprintf("Captura checklist %d de %d", done, len(targets)))
		if outcome.captured {
			_ = reporter.Event(taskCtx, "evidence_captured", "Evidencia guardada", map[string]any{
				"itemCode": target.ItemCode, "coveredItemCodes": coveredItemCodes(target),
				"slotNumber": target.SlotNumber, "index": index,
			})
		} else if outcome.failure != "" && !outcome.skipped {
			// Keep every failure in the job history, not only the first one
			// that fits in the final message.
			_ = reporter.Event(taskCtx, "evidence_failed", outcome.failure, map[string]any{
				"itemCode": target.ItemCode, "coveredItemCodes": coveredItemCodes(target),
				"slotNumber": target.SlotNumber, "index": index,
			})
		}
		if outcome.skipped {
			_ = reporter.Event(taskCtx, "evidence_skipped", "Lote de filas vacío: espacio no necesario", map[string]any{
				"itemCode": target.ItemCode, "coveredItemCodes": coveredItemCodes(target),
				"slotNumber": target.SlotNumber, "index": index,
			})
		}
		return nil
	})
	tally := tallyTargetOutcomes(outcomes)
	captured, skipped, failed, evidenceRecords, failures := tally.captured, tally.skipped, tally.failed, tally.evidenceRecords, tally.failures
	partialOutput := func(stage string) map[string]any {
		return map[string]any{
			"partial": true, "stage": stage, "fichaId": input.FichaID, "courseId": ficha.CourseID, "targets": len(targets),
			"captured": captured, "failed": failed, "skipped": skipped, "coverageCount": evidenceRecords,
			"failedItemCodes": failedItemCodes(failures), "failures": failures,
			"absent": tally.absent, "absences": tally.absences, "preferences": prefs,
		}
	}
	if fanoutErr != nil && ctx.Err() == nil && !errors.Is(fanoutErr, context.Canceled) {
		return jobs.Result{ErrorCode: "capture_fanout_failed", ErrorMessage: fanoutErr.Error(), Output: partialOutput("capture")}
	}
	// A cancelled run never prunes: its outcomes do not cover the plan.
	if err := ctx.Err(); err != nil {
		return jobs.Result{ErrorCode: "capture_cancelled", ErrorMessage: err.Error(), Output: partialOutput("capture")}
	}

	prunedEvidences := 0
	if pruneStore, ok := w.evidence.(captureChecklistPruneStore); ok {
		itemCodes, keep := captureChecklistPrunePlan(plannedTargets, targets, outcomes)
		pruned, pruneErr := pruneStore.PruneCaptureChecklistEvidence(ctx, input.FichaID, itemCodes, keep)
		if pruneErr != nil {
			return jobs.Result{ErrorCode: "evidence_prune_failed", ErrorMessage: fmt.Sprintf("no se pudieron retirar las evidencias obsoletas: %v", pruneErr), Retryable: true}
		}
		prunedEvidences = pruned
		if pruned > 0 {
			_ = reporter.Event(ctx, "evidence_pruned", "Evidencias obsoletas retiradas", map[string]any{"fichaId": input.FichaID, "pruned": pruned})
		}
	}

	groupCount := 0
	if groupStore, ok := w.evidence.(evidence.GroupStore); ok {
		groups, groupErr := groupStore.RebuildEvidenceGroups(ctx, input.FichaID)
		if groupErr != nil {
			return jobs.Result{ErrorCode: "evidence_group_failed", ErrorMessage: fmt.Sprintf("no se pudieron construir los grupos de evidencia: %v", groupErr), Retryable: true}
		}
		groupCount = len(groups)
		_ = reporter.Event(ctx, "evidence_groups_rebuilt", "Evidencias agrupadas para evitar duplicados", map[string]any{"fichaId": input.FichaID, "groupCount": groupCount})
	}
	output := map[string]any{
		"fichaId": input.FichaID, "courseId": ficha.CourseID, "targets": len(targets), "captured": captured,
		"failed": failed, "skipped": skipped, "prunedEvidences": prunedEvidences, "unresolved": summary.UnresolvedItems, "slotCount": len(targets), "captureUnitCount": len(targets), "coverageCount": evidenceRecords,
		"targetItems": len(targetItemCodes), "itemCount": summary.ItemCount, "groupCount": groupCount, "failures": failures,
		"absent": tally.absent, "absences": tally.absences, "preferences": prefs,
	}
	if ctx.Err() != nil {
		return jobs.Result{ErrorCode: "capture_cancelled", ErrorMessage: ctx.Err().Error(), Output: output}
	}
	// Best effort: prepare the review screen. A verification error never fails
	// the capture.
	if verifier, ok := w.evidence.(evidence.ReviewVerifier); ok {
		if report, verifyErr := verifier.VerifyEvidenceReviews(ctx, input.FichaID); verifyErr == nil {
			_ = reporter.Event(ctx, "evidence_reviews_verified", "Evidencias revisadas automáticamente", map[string]any{"fichaId": input.FichaID, "approved": report.Summary.Approved, "pending": report.Summary.Pending, "rejected": report.Summary.Rejected})
		}
	}
	absentNote := ""
	if tally.absent > 0 {
		absentNote = fmt.Sprintf(", %d sin contenido en Zajuna: %s", tally.absent, strings.Join(failedItemCodes(tally.absences), ", "))
	}
	if err := reporter.Progress(ctx, "completed", 100, fmt.Sprintf("Captura dirigida terminada: %d guardadas, %d omitidas, %d con error%s", captured, skipped, failed, absentNote)); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error(), Output: output}
	}
	if failed > 0 {
		message := fmt.Sprintf("captura incompleta: %d guardadas, %d omitidas, %d con error", captured, skipped, failed)
		// La UI muestra el mensaje del fallo (el Output parcial es para
		// detalle): listamos todos los ítems afectados, no solo el primero.
		if codes := failedItemCodes(failures); len(codes) > 0 {
			message += " (ítems " + strings.Join(codes, ", ") + ")"
		}
		if len(failures) > 0 {
			message += ". Primer error: " + failures[0]
		}
		if tally.absent > 0 {
			// Al final y sin "(ítems …)": el frontend lee esa lista como los
			// ítems que fallaron.
			message += fmt.Sprintf(". Sin contenido en Zajuna: %s", strings.Join(failedItemCodes(tally.absences), ", "))
		}
		output["partial"], output["stage"], output["failedItemCodes"] = true, "completed", failedItemCodes(failures)
		return jobs.Result{ErrorCode: "capture_partial_failure", ErrorMessage: message, Output: output}
	}
	return jobs.Result{Output: output}
}

// lockFichaCapture serializes captures of the same ficha (they write the same
// slots). Waiting honours ctx, so a cancelled job does not hold a worker until
// the running capture ends.
func lockFichaCapture(ctx context.Context, fichaID string) (func(), error) {
	return fichaCaptureLocks.lock(ctx, fichaID)
}

// captureChecklistPruneStore is optional (like evidence.GroupStore) so test
// doubles and older stores keep working without stale-evidence pruning.
type captureChecklistPruneStore interface {
	PruneCaptureChecklistEvidence(ctx context.Context, fichaID string, itemCodes []string, keep map[string]map[int]bool) (int, error)
}

type targetOutcomeTally struct {
	captured, skipped, failed, absent, evidenceRecords int
	failures, absences                                 []string
}

// absentContent recognizes failures that mean "Zajuna has nothing to show
// here" (no instructor post in a forum, an activity without a grading
// table) rather than a capture problem. They are reported separately and do
// not turn the whole capture into a failure.
func absentContent(failure string) bool {
	return strings.Contains(failure, "no tiene publicaciones del instructor") ||
		strings.Contains(failure, "no tiene respuestas del instructor") ||
		strings.Contains(failure, semanticAbsenceMarker) ||
		strings.Contains(failure, "table.generaltable (candidatos=0)")
}

// tallyTargetOutcomes aggregates fan-out results. Skipped slots (empty row
// batches) are neither captured nor failed.
func tallyTargetOutcomes(outcomes []targetOutcome) targetOutcomeTally {
	tally := targetOutcomeTally{failures: make([]string, 0)}
	for _, outcome := range outcomes {
		switch {
		case outcome.captured:
			tally.captured++
			tally.evidenceRecords += outcome.evidenceRecords
		case outcome.skipped:
			tally.skipped++
		case outcome.absent || absentContent(outcome.failure):
			tally.absent++
			tally.absences = append(tally.absences, outcome.failure)
			if primary, detail, ok := strings.Cut(outcome.failure, ": "); ok {
				for _, code := range outcome.coveredItemCodes {
					if code != primary {
						tally.absences = append(tally.absences, code+": "+detail)
					}
				}
			}
		default:
			tally.failed++
			if outcome.failure != "" {
				tally.failures = append(tally.failures, outcome.failure)
			}
		}
	}
	return tally
}

// captureChecklistPrunePlan returns the item codes covered by the executed
// targets and, per item, the slots whose evidence must survive: captured
// slots (fresh evidence), failed slots (previous evidence is kept) and slots
// of the full plan that this run did not execute (filtered out by itemCodes
// or cut by maxTargets). Skipped slots (empty row batches), absent slots (no
// content in Zajuna) and slots no longer in the plan are left out, so their
// evidence is pruned.
func captureChecklistPrunePlan(planned, executed []checklist.CaptureTarget, outcomes []targetOutcome) ([]string, map[string]map[int]bool) {
	keep := make(map[string]map[int]bool)
	mark := func(target checklist.CaptureTarget) {
		for _, itemCode := range coveredItemCodes(target) {
			if keep[itemCode] == nil {
				keep[itemCode] = make(map[int]bool)
			}
			keep[itemCode][normalizedSlot(target.SlotNumber)] = true
		}
	}
	executedKeys := make(map[string]bool, len(executed))
	for _, target := range executed {
		executedKeys[captureTargetSlotKey(target)] = true
	}
	for _, target := range planned {
		if !executedKeys[captureTargetSlotKey(target)] {
			mark(target)
		}
	}
	seen := make(map[string]bool)
	itemCodes := make([]string, 0)
	for index, target := range executed {
		for _, itemCode := range coveredItemCodes(target) {
			if !seen[itemCode] {
				seen[itemCode] = true
				itemCodes = append(itemCodes, itemCode)
			}
		}
		// A typed absence (capture.ErrContentAbsent: the right page loaded
		// and has nothing that proves the item) retires the evidence an
		// earlier run or an older rule left there. Absences recognised only by
		// their message (10.1.x without a grading table) are reported but
		// keep the previous evidence: that message is also what an error or
		// permission page produces.
		if index < len(outcomes) && (outcomes[index].skipped || outcomes[index].absent) {
			continue
		}
		mark(target)
	}
	return itemCodes, keep
}

func captureTargetSlotKey(target checklist.CaptureTarget) string {
	return target.ItemCode + "#" + fmt.Sprint(normalizedSlot(target.SlotNumber))
}

// normalizedSlot mirrors the store, which persists slot numbers below 1 as 1.
func normalizedSlot(slot int) int {
	if slot < 1 {
		return 1
	}
	return slot
}

// failedItemCodes extracts the unique "<itemCode>" prefix of each
// "<itemCode>: <detalle>" failure, preserving capture order.
func failedItemCodes(failures []string) []string {
	seen := make(map[string]bool, len(failures))
	codes := make([]string, 0, len(failures))
	for _, failure := range failures {
		code, _, found := strings.Cut(failure, ":")
		code = strings.TrimSpace(code)
		if !found || code == "" || strings.Contains(code, " ") || seen[code] {
			continue
		}
		seen[code] = true
		codes = append(codes, code)
	}
	return codes
}

func captureRequiresActivitySelection(itemCodes []string) bool {
	if len(itemCodes) == 0 {
		return true
	}
	for _, itemCode := range itemCodes {
		switch strings.TrimSpace(itemCode) {
		case "6.1", "10.1.1", "10.1.2":
			return true
		}
	}
	return false
}

func filterCaptureTargets(targets []checklist.CaptureTarget, itemCodes []string) []checklist.CaptureTarget {
	if len(itemCodes) == 0 {
		return targets
	}
	allowed := make(map[string]bool, len(itemCodes))
	for _, code := range itemCodes {
		if code = strings.TrimSpace(code); code != "" {
			allowed[code] = true
		}
	}
	filtered := make([]checklist.CaptureTarget, 0, len(targets))
	for _, target := range targets {
		if allowed[target.ItemCode] || anyAllowedCoverage(target, allowed) {
			filtered = append(filtered, target)
		}
	}
	return filtered
}

func coveredItemCodes(target checklist.CaptureTarget) []string {
	if len(target.CoveredItemCodes) == 0 {
		return []string{target.ItemCode}
	}
	return target.CoveredItemCodes
}

func anyAllowedCoverage(target checklist.CaptureTarget, allowed map[string]bool) bool {
	for _, itemCode := range coveredItemCodes(target) {
		if allowed[itemCode] {
			return true
		}
	}
	return false
}

func captureTargetCoverageCount(targets []checklist.CaptureTarget) int {
	count := 0
	for _, target := range targets {
		count += len(coveredItemCodes(target))
	}
	return count
}

func safePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = strings.NewReplacer("\\", "_", "/", "_", ":", "_", "..", "_").Replace(value)
	return value
}
