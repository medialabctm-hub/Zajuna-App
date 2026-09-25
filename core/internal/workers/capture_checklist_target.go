package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/security"
	"github.com/zajuna-app/core/internal/zajuna"
)

type checklistTargetParams struct {
	JobID      string
	Input      CaptureChecklistInput
	Target     checklist.CaptureTarget
	BaseURL    *url.URL
	Session    zajuna.Session
	Password   string
	OwnerName  string
	UseBrowser bool
	CookieMu   *sync.Mutex
	// Sessions reuses authenticated Chromium sessions across targets of one
	// run. Nil (single-target jobs or reuseSession off) opens and closes a
	// session per target.
	Sessions *browserSessionPool
	// AutoRenew retries the target once with a fresh login when the capture
	// lands on the Zajuna login page.
	AutoRenew bool
	// DeleteSkippedEvidence drops the slot's previous evidence when its row
	// batch turns out empty, while the slot lock is still held.
	DeleteSkippedEvidence bool
}

type targetOutcome struct {
	captured        bool
	evidenceRecords int
	failure         string
	// skipped marks a row-batch slot that starts after the last row: nothing
	// to capture, not a failure. Its previous evidence (if any) is stale.
	skipped bool
	// absent: the page loaded but has nothing that proves the item
	// (capture.ErrContentAbsent). Its previous evidence is stale.
	absent bool
	// coveredItemCodes lists every item an absent target stood for, so each
	// one reports the absence (e.g. 9.1.7 shares the capture of 9.1.6).
	coveredItemCodes []string
}

func (w *CaptureChecklistWorker) openChecklistBrowserSession(ctx context.Context, baseURL *url.URL, input CaptureChecklistInput, password string) (*capture.BrowserSession, error) {
	loginURL := *baseURL
	loginURL.Path = "/zajuna/login/index.php"
	loginURL.RawQuery = ""
	return w.runtime.OpenBrowserSession(ctx, capture.BrowserCredentials{
		LoginURL:     loginURL.String(),
		DocumentType: input.DocumentType,
		Document:     input.Username,
		Password:     password,
	})
}

// reusableBrowserSession tells whether a session is still authenticated and
// usable after a capture. "Selector not found" and empty row batches are
// page-level outcomes; login redirects, challenges and navigation errors may
// leave the session in an unknown state, so it is replaced.
func reusableBrowserSession(captureErr error, finalURL string) bool {
	if captureErr == nil {
		return !isZajunaLoginURL(finalURL)
	}
	return errors.Is(captureErr, capture.ErrSelectorNotFound) || errors.Is(captureErr, capture.ErrNoRowsInBatch) || errors.Is(captureErr, capture.ErrForumAccessDenied)
}

// browserSessionExpired tells whether a capture failed only because Zajuna
// closed the session, so a fresh login can recover the target.
func browserSessionExpired(captureErr error, finalURL string) bool {
	if captureErr != nil {
		return errors.Is(captureErr, capture.ErrLoginPage)
	}
	return isZajunaLoginURL(finalURL)
}

// captureWithBrowserSession returns openErr when no authenticated session
// could be opened, and captureErr for the capture itself.
func (w *CaptureChecklistWorker) captureWithBrowserSession(ctx context.Context, params checklistTargetParams, outputPath string, options capture.CaptureOptions) (result capture.CaptureResult, captureErr error, openErr error) {
	attempts := 1
	if params.AutoRenew {
		attempts = 2
	}
	for attempt := 0; attempt < attempts; attempt++ {
		var browserSession checklistBrowserSession
		switch {
		case params.Sessions != nil && attempt > 0:
			browserSession, openErr = params.Sessions.acquireFresh(ctx)
		case params.Sessions != nil:
			browserSession, openErr = params.Sessions.acquire(ctx)
		default:
			browserSession, openErr = w.openChecklistBrowserSession(ctx, params.BaseURL, params.Input, params.Password)
		}
		if openErr != nil {
			return capture.CaptureResult{}, nil, openErr
		}
		result, captureErr = browserSession.CaptureURLWithMetadataAndOptions(ctx, params.Target.URL, outputPath, options)
		if params.Sessions != nil {
			params.Sessions.release(browserSession, reusableBrowserSession(captureErr, result.FinalURL))
		} else {
			browserSession.Close()
		}
		if !browserSessionExpired(captureErr, result.FinalURL) || ctx.Err() != nil {
			break
		}
	}
	return result, captureErr, nil
}

func (w *CaptureChecklistWorker) captureChecklistTarget(ctx context.Context, params checklistTargetParams) targetOutcome {
	target := params.Target
	parsedTarget, parseErr := security.ValidateHTTPURL(target.URL, []string{params.BaseURL.String()}, w.allowPrivateTargets)
	if parseErr != nil || parsedTarget.Host == "" || parsedTarget.Scheme != params.BaseURL.Scheme || parsedTarget.Host != params.BaseURL.Host {
		return targetOutcome{failure: target.ItemCode + ": origen de URL no permitido"}
	}
	outputPath := filepath.Join(w.dataDir, "evidences", "checklist", safePathPart(params.Input.FichaID), safePathPart(target.ItemCode), fmt.Sprintf("slot-%d.png", target.SlotNumber))
	// Two runs (a full checklist job and a single-target job, or two jobs for
	// the same ficha) must not write the same slot file and evidence rows at
	// once: the second waits until the first has captured and persisted.
	unlock, lockErr := checklistSlotLocks.lockAll(ctx, checklistSlotLockKeys(params.Input.FichaID, target))
	if lockErr != nil {
		return targetOutcome{failure: target.ItemCode + ": captura cancelada"}
	}
	defer unlock()
	options := capture.CaptureOptions{
		Selector: target.CSSSelector, Selectors: target.CSSSelectorFallbacks,
		RevealSelectors: target.RevealSelectors, HideSelectors: target.HideSelectors,
		ViewportWidth: target.ViewportWidth, ViewportHeight: target.ViewportHeight,
		FullPage: target.FullPage, LabelHint: target.LabelHint, OwnerName: params.OwnerName,
		// Always strict, also for targets replayed from older job payloads:
		// checklist evidence is never a generic full-page fallback.
		RequireSelector: true, OwnerOnly: target.OwnerOnly,
		RowSelector: target.RowSelector, RowsPerShot: target.RowsPerShot, RowBatch: target.RowBatch,
		OptionalSlot: target.OptionalSlot, RowMatch: target.RowMatch, RowRequireReply: target.RowRequireReply,
		CourseLayout: target.CourseLayout, AbsenceSelector: target.AbsenceSelector,
		MaxWidth: target.MaxCaptureWidth, ColumnBatch: target.ColumnBatch,
		// ExpandEmbeddedSheets stays off: on real Google Sheets it resized the
		// widget so the grid rendered blank (ficha 3135429, 1.1.x and 1.2.x);
		// capturePage always runs prepareEmbeddedSheets, which grows the frame
		// from the nested #pageswitcher-content and renders the whole sheet.
	}

	var captureResult capture.CaptureResult
	var captureErr error
	if params.UseBrowser {
		var openErr error
		captureResult, captureErr, openErr = w.captureWithBrowserSession(ctx, params, outputPath, options)
		if openErr != nil {
			return targetOutcome{failure: target.ItemCode + ": " + openErr.Error()}
		}
	} else {
		if params.CookieMu != nil {
			params.CookieMu.Lock()
		}
		cookies := make([]capture.BrowserCookie, 0)
		for _, cookie := range params.Session.CookiesForURL(target.URL) {
			converted, convErr := capture.BrowserCookieForTarget(cookie, parsedTarget)
			if convErr != nil {
				continue
			}
			cookies = append(cookies, converted)
		}
		if params.CookieMu != nil {
			params.CookieMu.Unlock()
		}
		if len(cookies) == 0 {
			return targetOutcome{failure: target.ItemCode + ": sesión sin cookies para la ruta"}
		}
		captureResult, captureErr = w.runtime.CaptureURLWithMetadataAndCookiesAndOptions(ctx, target.URL, outputPath, cookies, options)
	}
	if captureErr != nil {
		if errors.Is(captureErr, capture.ErrNoRowsInBatch) {
			// The list is fully covered by earlier slots. Stale evidence for
			// this slot is removed by the reference-counted prune/delete, never
			// by deleting the file: another kept row may still point to it.
			if params.DeleteSkippedEvidence {
				w.deleteSlotEvidence(ctx, params.Input.FichaID, target)
			}
			return targetOutcome{skipped: true}
		}
		if errors.Is(captureErr, capture.ErrLoginPage) {
			return targetOutcome{failure: target.ItemCode + ": sesión de Zajuna expirada o página de login"}
		}
		if errors.Is(captureErr, capture.ErrChallengePage) {
			return targetOutcome{failure: target.ItemCode + ": Zajuna pidió CAPTCHA o MFA"}
		}
		if errors.Is(captureErr, capture.ErrForumAccessDenied) {
			return targetOutcome{failure: target.ItemCode + ": el foro asignado no está disponible para tu cuenta; vuelve a buscar las rutas del curso"}
		}
		if errors.Is(captureErr, capture.ErrContentAbsent) {
			return targetOutcome{absent: true, failure: target.ItemCode + ": " + absenceMessage(target, captureErr), coveredItemCodes: coveredItemCodes(target)}
		}
		return targetOutcome{failure: target.ItemCode + ": " + captureErr.Error()}
	}
	if isZajunaLoginURL(captureResult.FinalURL) {
		return targetOutcome{failure: target.ItemCode + ": Zajuna redirigió a login"}
	}
	if _, finalErr := security.ValidateHTTPURL(captureResult.FinalURL, []string{params.BaseURL.String()}, w.allowPrivateTargets); finalErr != nil {
		return targetOutcome{failure: target.ItemCode + ": redirección fuera del origen permitido"}
	}
	hash, hashErr := fileSHA256(outputPath)
	if hashErr != nil {
		return targetOutcome{failure: target.ItemCode + ": no se pudo calcular el hash"}
	}
	metadata, _ := json.Marshal(map[string]any{
		"url": security.RedactURL(target.URL), "finalUrl": security.RedactURL(captureResult.FinalURL),
		"title": security.RedactText(captureResult.Title), "routeKey": target.RouteKey, "reviewStatus": target.ReviewStatus,
		"selector": captureResult.Selector, "selectorFallbacks": target.CSSSelectorFallbacks, "selectorMatched": captureResult.SelectorMatched,
		"labelHint": target.LabelHint, "routeKind": target.RouteKind, "groupName": target.GroupName,
		"revealSelectors": target.RevealSelectors, "hideSelectors": target.HideSelectors,
		"viewportWidth": target.ViewportWidth, "viewportHeight": target.ViewportHeight, "fullPage": target.FullPage,
		"phaseSection": target.PhaseSection, "jobId": params.JobID,
		"activityId": target.ActivityID, "activityTitle": target.ActivityTitle, "technical": target.Technical, "ownerOnly": target.OwnerOnly,
		"coveredItemCodes": coveredItemCodes(target), "captureUnitKey": target.RouteKey,
		"rowSelector": target.RowSelector, "rowsPerShot": target.RowsPerShot, "rowBatch": target.RowBatch,
		"rowsTotal": captureResult.RowsTotal, "rowStart": captureResult.RowStart, "contentItems": captureResult.ContentItems,
		"rowMatch": target.RowMatch, "rowRequireReply": target.RowRequireReply, "semanticCheck": target.SemanticCheck, "courseLayout": target.CourseLayout,
		"maxCaptureWidth": target.MaxCaptureWidth, "columnBatch": target.ColumnBatch, "columnWindows": captureResult.ColumnWindows,
	})
	capturedAt := time.Now().UTC()
	evidenceRecords := 0
	for _, itemCode := range coveredItemCodes(target) {
		evidenceID := artifactID("evidence", params.Input.FichaID, itemCode+"#"+strconv.Itoa(target.SlotNumber), "")
		if err := w.evidence.CreateEvidence(ctx, evidence.Record{
			ID: evidenceID, FichaID: params.Input.FichaID, ItemCode: itemCode, SlotNumber: target.SlotNumber,
			Name: target.Name, FilePath: outputPath, Format: "png", Source: "capture-checklist", SHA256: hash,
			Metadata: metadata, CapturedAt: capturedAt,
		}); err != nil {
			return targetOutcome{failure: itemCode + ": no se pudo registrar la evidencia"}
		}
		evidenceRecords++
	}
	return targetOutcome{captured: true, evidenceRecords: evidenceRecords}
}

// semanticAbsenceMarker prefixes the message of an absence found by a
// semantic rule, in the job result shown to the person.
const semanticAbsenceMarker = "sin contenido válido en Zajuna"

// absenceMessage explains an ErrContentAbsent in plain words.
func absenceMessage(target checklist.CaptureTarget, captureErr error) string {
	if target.SemanticCheck == checklist.SemanticForumDates {
		return semanticAbsenceMarker + ": el foro no muestra fechas de apertura y cierre"
	}
	return captureErr.Error()
}

func (w *CaptureChecklistWorker) deleteSlotEvidence(ctx context.Context, fichaID string, target checklist.CaptureTarget) {
	deleteStore, ok := w.evidence.(evidence.DeleteStore)
	if !ok {
		return
	}
	for _, itemCode := range coveredItemCodes(target) {
		evidenceID := artifactID("evidence", fichaID, itemCode+"#"+strconv.Itoa(target.SlotNumber), "")
		_, _ = deleteStore.DeleteEvidence(ctx, evidenceID)
	}
}

// CaptureChecklistTargetInput is the payload for a single-target checklist job.
type CaptureChecklistTargetInput struct {
	ParentJobID  string                  `json:"parentJobId,omitempty"`
	Index        int                     `json:"index"`
	FichaID      string                  `json:"fichaId"`
	Username     string                  `json:"username"`
	DocumentType string                  `json:"documentType"`
	OwnerName    string                  `json:"ownerName,omitempty"`
	UseBrowser   bool                    `json:"useBrowser,omitempty"`
	Target       checklist.CaptureTarget `json:"target"`
}

// CaptureChecklistTargetWorker captures one checklist target as its own job.
// Parent capture-checklist fans out in-process for UI compatibility; this
// worker stays registered so the same unit can also ride the jobs FIFO.
type CaptureChecklistTargetWorker struct {
	parent *CaptureChecklistWorker
}

func NewCaptureChecklistTargetWorker(parent *CaptureChecklistWorker) (*CaptureChecklistTargetWorker, error) {
	if parent == nil {
		return nil, errors.New("capture checklist target worker requires parent worker")
	}
	return &CaptureChecklistTargetWorker{parent: parent}, nil
}

func (w *CaptureChecklistTargetWorker) ID() string { return CaptureChecklistTargetWorkerID }

func (w *CaptureChecklistTargetWorker) Execute(ctx context.Context, job jobs.Job, reporter jobs.Reporter) jobs.Result {
	var input CaptureChecklistTargetInput
	if err := json.Unmarshal(job.Input, &input); err != nil {
		return jobs.Result{ErrorCode: "invalid_input", ErrorMessage: "la entrada de captura de objetivo del checklist no es válida"}
	}
	input.FichaID = strings.TrimSpace(input.FichaID)
	input.Username = strings.TrimSpace(input.Username)
	input.DocumentType = strings.TrimSpace(input.DocumentType)
	if input.DocumentType == "" {
		input.DocumentType = "CC"
	}
	if input.FichaID == "" || input.Username == "" || strings.TrimSpace(input.Target.URL) == "" {
		return jobs.Result{ErrorCode: "invalid_input", ErrorMessage: "fichaId, usuario y objetivo son obligatorios"}
	}
	// Same ficha lock as the full capture: both write the same slot files.
	unlock, lockErr := lockFichaCapture(ctx, input.FichaID)
	if lockErr != nil {
		return jobs.Result{ErrorCode: "capture_cancelled", ErrorMessage: lockErr.Error()}
	}
	defer unlock()
	password, err := w.parent.credentials.Get(input.Username)
	if err != nil || password == "" {
		return jobs.Result{ErrorCode: "credential_unavailable", ErrorMessage: "no se encontró la contraseña de Zajuna en el almacén seguro"}
	}
	session, err := w.parent.client.Login(ctx, zajuna.Credentials{DocumentType: input.DocumentType, Document: input.Username, Password: password})
	if err != nil {
		return jobs.Result{Retryable: retryableZajunaError(err), ErrorCode: "zajuna_login_failed", ErrorMessage: fmt.Sprintf("no se pudo iniciar sesión para el objetivo: %v", err)}
	}
	baseURL, err := url.Parse(session.BaseURL)
	if err != nil || baseURL.Host == "" {
		return jobs.Result{ErrorCode: "invalid_zajuna_session", ErrorMessage: "la sesión de Zajuna no tiene un origen válido"}
	}
	useBrowser := input.UseBrowser || strings.EqualFold(baseURL.Hostname(), "zajuna.sena.edu.co")
	if err := reporter.Progress(ctx, "capture", 20, fmt.Sprintf("Capturando objetivo %s", input.Target.ItemCode)); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
	}
	prefs := w.parent.preferences(ctx)
	outcome := w.parent.captureChecklistTarget(ctx, checklistTargetParams{
		JobID:  job.ID,
		Input:  CaptureChecklistInput{FichaID: input.FichaID, Username: input.Username, DocumentType: input.DocumentType},
		Target: applyCapturePreferences([]checklist.CaptureTarget{input.Target}, prefs)[0], BaseURL: baseURL, Session: session, Password: password,
		OwnerName: input.OwnerName, UseBrowser: useBrowser, AutoRenew: prefs.AutoRenew,
		// Empty row batch: the slot is not needed. Drop any evidence a previous
		// (longer) list left in it so the checklist does not show a stale shot.
		DeleteSkippedEvidence: true,
	})
	if outcome.skipped {
		_ = reporter.Progress(ctx, "completed", 100, "Objetivo omitido: el lote de filas está vacío")
		return jobs.Result{Output: map[string]any{
			"fichaId": input.FichaID, "itemCode": input.Target.ItemCode, "slotNumber": input.Target.SlotNumber,
			"index": input.Index, "parentJobId": input.ParentJobID, "evidenceRecords": 0, "skipped": true,
		}}
	}
	if !outcome.captured {
		return jobs.Result{ErrorCode: "capture_target_failed", ErrorMessage: outcome.failure}
	}
	_ = reporter.Progress(ctx, "completed", 100, "Objetivo capturado")
	return jobs.Result{Output: map[string]any{
		"fichaId": input.FichaID, "itemCode": input.Target.ItemCode, "slotNumber": input.Target.SlotNumber,
		"index": input.Index, "parentJobId": input.ParentJobID, "evidenceRecords": outcome.evidenceRecords,
	}}
}
