package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // register the JPEG decoder for manual uploads
	_ "image/png"  // register the PNG decoder for captures
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/checklist"
)

// Review statuses and sources persisted in evidence_reviews.
const (
	ReviewApproved = "approved"
	ReviewPending  = "pending"
	ReviewRejected = "rejected"

	ReviewSourceAuto   = "auto"
	ReviewSourceManual = "manual"
)

// Automatic review reason codes.
const (
	ReasonFileMissing      = "file_missing"
	ReasonLoginPage        = "login_page"
	ReasonTooWide          = "too_wide"
	ReasonTooTall          = "too_tall"
	ReasonTooSmall         = "too_small"
	ReasonMostlyBlank      = "mostly_blank"
	ReasonGenericSelector  = "generic_selector"
	ReasonDuplicateContent = "duplicate_content"
	ReasonEmptySection     = "empty_section"
	ReasonOutdatedRule     = "outdated_rule"
)

const (
	reviewMaxWidth  = 4000
	reviewMaxHeight = 9000
	reviewMinWidth  = 200
	reviewMinHeight = 120
	// Moodle pages are white by design: 96 % flagged real text sections. Only
	// a practically empty capture (≥ 99,5 % near-white) is reported.
	reviewBlankRatio = 0.995
	// A tall shot whose longest empty band covers this share is unrendered.
	reviewBlankBandRatio     = 0.5
	reviewBlankAreaMinHeight = 600
	reviewNearWhite          = 245
	reviewMaxSamples         = 250_000
	reviewMaxDecodePixels    = 20_000_000
	missingItemReason        = "No se capturó evidencia para este ítem."
)

var genericReviewSelectors = map[string]bool{
	"#region-main":                 true,
	"#page-content":                true,
	"#region-main .course-content": true,
	".course-content":              true,
}

// ReviewReason is one verifiable problem found in an evidence.
type ReviewReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Review is the persisted review state of one evidence.
type Review struct {
	EvidenceID string
	FichaID    string
	Status     string
	Source     string
	Reasons    []ReviewReason
	Note       string
	SHA256     string
	Width      int
	Height     int
	UpdatedAt  time.Time
}

// ReviewStore is the persistence contract the verifier needs.
type ReviewStore interface {
	ListEvidencesByFicha(ctx context.Context, fichaID string, limit int) ([]Record, error)
	ListEvidenceReviews(ctx context.Context, fichaID string) ([]Review, error)
	UpsertEvidenceReviews(ctx context.Context, reviews []Review) error
}

// AbsenceReasonStore is optional: it returns, per item code, why the latest
// capture found nothing to show in Zajuna (e.g. an empty section or a forum
// without instructor replies). Missing items then say what to fix in Zajuna
// instead of suggesting a recapture that cannot help.
type AbsenceReasonStore interface {
	CaptureAbsenceReasons(ctx context.Context, fichaID string) (map[string]string, error)
}

// ReviewVerifier is optional on evidence stores: capture workers call it at the
// end of a run so the review screen is ready.
type ReviewVerifier interface {
	VerifyEvidenceReviews(ctx context.Context, fichaID string) (ReviewReport, error)
}

type ReviewSummary struct {
	Total             int `json:"total"`
	Approved          int `json:"approved"`
	Pending           int `json:"pending"`
	Rejected          int `json:"rejected"`
	ItemsWithEvidence int `json:"itemsWithEvidence"`
	ItemsApproved     int `json:"itemsApproved"`
	ItemsPending      int `json:"itemsPending"`
	ItemsMissing      int `json:"itemsMissing"`
}

type ReviewEntry struct {
	EvidenceID      string         `json:"evidenceId"`
	ItemCode        string         `json:"itemCode"`
	ItemDescription string         `json:"itemDescription"`
	SlotNumber      int            `json:"slotNumber"`
	Name            string         `json:"name"`
	Status          string         `json:"status"`
	Source          string         `json:"source"`
	Note            string         `json:"note"`
	Reasons         []ReviewReason `json:"reasons"`
	Width           int            `json:"width"`
	Height          int            `json:"height"`
	SHA256          string         `json:"sha256"`
	SharedWith      []string       `json:"sharedWith"`
}

type MissingItem struct {
	ItemCode    string `json:"itemCode"`
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

type ReviewReport struct {
	FichaID      string        `json:"fichaId"`
	VerifiedAt   string        `json:"verifiedAt"`
	Summary      ReviewSummary `json:"summary"`
	Evidences    []ReviewEntry `json:"evidences"`
	MissingItems []MissingItem `json:"missingItems"`
}

type reviewMetadata struct {
	URL               string   `json:"url"`
	ContentItems      *int     `json:"contentItems"`
	FinalURL          string   `json:"finalUrl"`
	Selector          string   `json:"selector"`
	SelectorFallbacks []string `json:"selectorFallbacks"`
	CoveredItemCodes  []string `json:"coveredItemCodes"`
	SemanticCheck     string   `json:"semanticCheck"`
}

// ImageStats are the measured properties of an evidence image.
type ImageStats struct {
	Width      int
	Height     int
	BlankRatio float64 // -1 when not analysed
	// BlankBandRatio is the longest run of entirely near-white rows over the
	// image height (-1 when not analysed). Text leaves many short gaps; one
	// band covering most of a tall shot is an unrendered area (e.g. a Google
	// Sheet that did not paint its grid).
	BlankBandRatio float64
}

// ResolveEvidencePath returns the absolute path of an evidence file and whether
// it stays inside <dataDir>/evidences (symlinks resolved when the file exists).
func ResolveEvidencePath(dataDir, path string) (string, bool) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(dataDir) == "" {
		return "", false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dataDir, path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	absRoot, err := filepath.Abs(filepath.Join(dataDir, "evidences"))
	if err != nil {
		return "", false
	}
	if realRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = realRoot
	}
	if realPath, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = realPath
	}
	relative, err := filepath.Rel(absRoot, absPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return absPath, false
	}
	return absPath, true
}

// AnalyzeImage reads dimensions first and only decodes the image for the blank
// analysis when it is small enough to keep memory bounded.
func AnalyzeImage(path string) (ImageStats, error) {
	stats := ImageStats{BlankRatio: -1, BlankBandRatio: -1}
	file, err := os.Open(path)
	if err != nil {
		return stats, err
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return stats, fmt.Errorf("decode image config: %w", err)
	}
	stats.Width, stats.Height = config.Width, config.Height
	if stats.Width <= 0 || stats.Height <= 0 || int64(stats.Width)*int64(stats.Height) > reviewMaxDecodePixels {
		return stats, nil
	}
	if _, err := file.Seek(0, 0); err != nil {
		return stats, err
	}
	img, _, err := image.Decode(file)
	if err != nil {
		return stats, fmt.Errorf("decode image: %w", err)
	}
	stats.BlankRatio = blankRatio(img)
	stats.BlankBandRatio = blankBandRatio(img)
	return stats, nil
}

func blankRatio(img image.Image) float64 {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return 1
	}
	step := int(math.Ceil(math.Sqrt(float64(width) * float64(height) / reviewMaxSamples)))
	if step < 1 {
		step = 1
	}
	samples, blank := 0, 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			samples++
			if nearWhite(img.At(x, y)) {
				blank++
			}
		}
	}
	if samples == 0 {
		return 1
	}
	return float64(blank) / float64(samples)
}

// blankBandRatio samples on the same grid as blankRatio and returns the
// longest run of sampled rows whose samples are all near-white, over the
// number of sampled rows.
func blankBandRatio(img image.Image) float64 {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return 1
	}
	step := int(math.Ceil(math.Sqrt(float64(width) * float64(height) / reviewMaxSamples)))
	if step < 1 {
		step = 1
	}
	rows, run, longest := 0, 0, 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		rows++
		blank := true
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			if !nearWhite(img.At(x, y)) {
				blank = false
				break
			}
		}
		if blank {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	if rows == 0 {
		return 1
	}
	return float64(longest) / float64(rows)
}

func nearWhite(c color.Color) bool {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	if n.A == 0 {
		return true
	}
	return n.R >= reviewNearWhite && n.G >= reviewNearWhite && n.B >= reviewNearWhite
}

// VerifyRecord applies every automatic rule to one evidence. all contains every
// evidence of the ficha (used for duplicate detection).
func VerifyRecord(dataDir string, record Record, all []Record, now time.Time) Review {
	review := Review{
		EvidenceID: record.ID, FichaID: record.FichaID, Source: ReviewSourceAuto,
		SHA256: record.SHA256, UpdatedAt: now.UTC(), Reasons: []ReviewReason{},
	}
	var metadata reviewMetadata
	if len(record.Metadata) > 0 {
		_ = json.Unmarshal(record.Metadata, &metadata)
	}

	path, inside := ResolveEvidencePath(dataDir, record.FilePath)
	fileOK := false
	if inside {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fileOK = true
		}
	}
	if !fileOK {
		review.Reasons = append(review.Reasons, ReviewReason{Code: ReasonFileMissing, Message: "El archivo de la evidencia no existe en este equipo."})
	}
	if strings.Contains(strings.ToLower(metadata.FinalURL), "/login/") {
		review.Reasons = append(review.Reasons, ReviewReason{Code: ReasonLoginPage, Message: "Se capturó la página de inicio de sesión en lugar del contenido."})
	}

	if fileOK && isReviewImage(record.Format, path) {
		if stats, err := AnalyzeImage(path); err == nil {
			review.Width, review.Height = stats.Width, stats.Height
			for _, reason := range imageReasons(stats) {
				// A single activity card or a forum row is short by nature;
				// "too small" only means something for a course section,
				// where it is a collapsed or empty section.
				if reason.Code == ReasonTooSmall && !strings.Contains(metadata.Selector, ".section") && stats.Height >= 40 {
					continue
				}
				review.Reasons = append(review.Reasons, reason)
			}
		}
	}

	// A course section that shows no activity, resource or file (only its
	// title or collapsed subsections) proves nothing by itself.
	if metadata.ContentItems != nil && *metadata.ContentItems == 0 {
		review.Reasons = append(review.Reasons, ReviewReason{Code: ReasonEmptySection, Message: "La sección no muestra actividades ni archivos: revisa en Zajuna si falta el contenido o si está en una subsección."})
	}

	selector := strings.TrimSpace(metadata.Selector)
	if genericReviewSelectors[selector] && len(metadata.SelectorFallbacks) > 0 {
		if expected := strings.TrimSpace(metadata.SelectorFallbacks[0]); expected != "" && expected != selector {
			review.Reasons = append(review.Reasons, ReviewReason{Code: ReasonGenericSelector, Message: "No se encontró la sección exacta y se capturó un área genérica: puede ser otra página."})
		}
	}

	// Items whose proof depends on content (instructor replies, a
	// conclusion, forum dates) are only valid when the capture enforced that
	// rule; evidence from an older, weaker rule showed the wrong rows.
	if required := checklist.SemanticCheckForItem(record.ItemCode); required != "" && strings.TrimSpace(record.Source) == "capture-checklist" && metadata.SemanticCheck != required {
		review.Reasons = append(review.Reasons, ReviewReason{Code: ReasonOutdatedRule, Message: outdatedRuleMessage(required)})
	}

	if duplicates := duplicateItemCodes(record, metadata, all); len(duplicates) > 0 {
		review.Reasons = append(review.Reasons, ReviewReason{Code: ReasonDuplicateContent, Message: "Es idéntica a la evidencia del ítem " + strings.Join(duplicates, ", ") + "."})
	}

	review.Status = statusForReasons(review.Reasons)
	return review
}

func outdatedRuleMessage(rule string) string {
	switch rule {
	case checklist.SemanticForumReplies:
		return "Se capturó con una regla anterior que no exige respuestas del instructor: vuelve a capturar el ítem."
	case checklist.SemanticForumConclusion:
		return "Se capturó con una regla anterior que no exige un debate de conclusión: vuelve a capturar el ítem."
	case checklist.SemanticForumDates:
		return "Se capturó con una regla anterior que no exige las fechas del foro: vuelve a capturar el ítem."
	}
	return "Se capturó con una regla anterior: vuelve a capturar el ítem."
}

// sameCaptureTarget compares two capture URLs ignoring what does not change
// the page shown: Moodle's forceview flag, the fragment and parameter order.
// The same forum reached as view.php?id=N and view.php?forceview=1&id=N was
// reported as a duplicate of another item.
func sameCaptureTarget(left, right string) bool {
	return canonicalCaptureURL(left) == canonicalCaptureURL(right)
}

func canonicalCaptureURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	query := parsed.Query()
	query.Del("forceview")
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String()
}

func imageReasons(stats ImageStats) []ReviewReason {
	reasons := []ReviewReason{}
	if stats.Width > reviewMaxWidth {
		reasons = append(reasons, ReviewReason{Code: ReasonTooWide, Message: fmt.Sprintf("La imagen es demasiado ancha (%d px) y no se leerá en el reporte.", stats.Width)})
	}
	if stats.Height > reviewMaxHeight {
		reasons = append(reasons, ReviewReason{Code: ReasonTooTall, Message: fmt.Sprintf("La imagen es demasiado larga (%d px); revisa que muestre solo lo necesario.", stats.Height)})
	}
	if stats.Height < reviewMinHeight || stats.Width < reviewMinWidth {
		reasons = append(reasons, ReviewReason{Code: ReasonTooSmall, Message: "Solo se capturó un encabezado o una línea: probablemente falta el contenido."})
	}
	if stats.BlankRatio >= reviewBlankRatio || (stats.Height >= reviewBlankAreaMinHeight && stats.BlankBandRatio >= reviewBlankBandRatio) {
		reasons = append(reasons, ReviewReason{Code: ReasonMostlyBlank, Message: "La imagen está casi en blanco."})
	}
	return reasons
}

func isReviewImage(format, path string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png", "jpeg", "jpg":
		return true
	case "":
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".png" || ext == ".jpg" || ext == ".jpeg"
	}
	return false
}

func statusForReasons(reasons []ReviewReason) string {
	status := ReviewApproved
	for _, reason := range reasons {
		if reason.Code == ReasonFileMissing || reason.Code == ReasonLoginPage {
			return ReviewRejected
		}
		status = ReviewPending
	}
	return status
}

func duplicateItemCodes(record Record, metadata reviewMetadata, all []Record) []string {
	hash := strings.ToLower(strings.TrimSpace(record.SHA256))
	if hash == "" {
		return nil
	}
	covered := map[string]bool{record.ItemCode: true}
	for _, code := range metadata.CoveredItemCodes {
		covered[code] = true
	}
	seen := map[string]bool{}
	for _, other := range all {
		if other.ID == record.ID || strings.ToLower(strings.TrimSpace(other.SHA256)) != hash {
			continue
		}
		// Two items that intentionally target the same place (same page and
		// selector) show the same content: that is not a capture error.
		var otherMetadata reviewMetadata
		if len(other.Metadata) > 0 {
			_ = json.Unmarshal(other.Metadata, &otherMetadata)
		}
		if metadata.Selector != "" && otherMetadata.Selector == metadata.Selector &&
			(sameCaptureTarget(otherMetadata.URL, metadata.URL) || (metadata.FinalURL != "" && sameCaptureTarget(otherMetadata.FinalURL, metadata.FinalURL))) {
			continue
		}
		if !covered[other.ItemCode] {
			seen[other.ItemCode] = true
		}
	}
	return sortedKeys(seen)
}

func sharedItemCodes(record Record, all []Record) []string {
	hash := strings.ToLower(strings.TrimSpace(record.SHA256))
	seen := map[string]bool{}
	var metadata reviewMetadata
	if len(record.Metadata) > 0 {
		_ = json.Unmarshal(record.Metadata, &metadata)
	}
	for _, code := range metadata.CoveredItemCodes {
		seen[code] = true
	}
	if hash != "" {
		for _, other := range all {
			if other.ID != record.ID && strings.ToLower(strings.TrimSpace(other.SHA256)) == hash {
				seen[other.ItemCode] = true
			}
		}
	}
	delete(seen, record.ItemCode)
	delete(seen, "")
	return sortedKeys(seen)
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

// VerifyFicha verifies the evidences of a ficha and persists the result.
// A manual decision is kept while the evidence sha256 is unchanged. When
// onlyUnreviewed is true, rows that already have a review for the same sha are
// reused without analysing images again.
func VerifyFicha(ctx context.Context, store ReviewStore, dataDir, fichaID string, onlyUnreviewed bool, now time.Time) (ReviewReport, error) {
	fichaID = strings.TrimSpace(fichaID)
	if fichaID == "" {
		return ReviewReport{}, errors.New("fichaId es obligatorio")
	}
	records, err := store.ListEvidencesByFicha(ctx, fichaID, 10000)
	if err != nil {
		return ReviewReport{}, err
	}
	existing, err := store.ListEvidenceReviews(ctx, fichaID)
	if err != nil {
		return ReviewReport{}, err
	}
	byID := make(map[string]Review, len(existing))
	for _, review := range existing {
		byID[review.EvidenceID] = review
	}
	reviews := make(map[string]Review, len(records))
	changed := make([]Review, 0)
	for _, record := range records {
		if current, ok := byID[record.ID]; ok && current.SHA256 == record.SHA256 {
			if current.Source == ReviewSourceManual || onlyUnreviewed {
				reviews[record.ID] = current
				continue
			}
		}
		review := VerifyRecord(dataDir, record, records, now)
		reviews[record.ID] = review
		changed = append(changed, review)
	}
	if len(changed) > 0 {
		if err := store.UpsertEvidenceReviews(ctx, changed); err != nil {
			return ReviewReport{}, err
		}
	}
	report := BuildReviewReport(fichaID, records, reviews)
	if reasonStore, ok := store.(AbsenceReasonStore); ok {
		if reasons, reasonErr := reasonStore.CaptureAbsenceReasons(ctx, fichaID); reasonErr == nil {
			for index, item := range report.MissingItems {
				if reason := strings.TrimSpace(reasons[item.ItemCode]); reason != "" {
					report.MissingItems[index].Reason = "Sin contenido en Zajuna: " + reason + ". Agrégalo en Zajuna y vuelve a capturar."
				}
			}
		}
	}
	return report, nil
}

// BuildReviewReport assembles the API payload from records and their reviews.
func BuildReviewReport(fichaID string, records []Record, reviews map[string]Review) ReviewReport {
	items := checklist.Items()
	order := make(map[string]int, len(items))
	descriptions := make(map[string]string, len(items))
	for index, item := range items {
		order[item.ItemCode] = index
		descriptions[item.ItemCode] = item.Description
	}
	sorted := append([]Record(nil), records...)
	sort.SliceStable(sorted, func(i, j int) bool {
		oi, iok := order[sorted[i].ItemCode]
		oj, jok := order[sorted[j].ItemCode]
		if iok != jok {
			return iok
		}
		if oi != oj {
			return oi < oj
		}
		if sorted[i].ItemCode != sorted[j].ItemCode {
			return sorted[i].ItemCode < sorted[j].ItemCode
		}
		return sorted[i].SlotNumber < sorted[j].SlotNumber
	})

	report := ReviewReport{FichaID: fichaID, Evidences: make([]ReviewEntry, 0, len(sorted)), MissingItems: []MissingItem{}}
	var latest time.Time
	itemState := map[string]string{}
	for _, record := range sorted {
		review := reviews[record.ID]
		entry := BuildReviewEntry(record, review, records)
		report.Evidences = append(report.Evidences, entry)
		if review.UpdatedAt.After(latest) {
			latest = review.UpdatedAt
		}
		report.Summary.Total++
		switch entry.Status {
		case ReviewApproved:
			report.Summary.Approved++
		case ReviewRejected:
			report.Summary.Rejected++
		default:
			report.Summary.Pending++
		}
		if record.ItemCode == "" {
			continue
		}
		if entry.Status != ReviewApproved {
			itemState[record.ItemCode] = ReviewPending
		} else if _, ok := itemState[record.ItemCode]; !ok {
			itemState[record.ItemCode] = ReviewApproved
		}
	}
	for _, state := range itemState {
		report.Summary.ItemsWithEvidence++
		if state == ReviewApproved {
			report.Summary.ItemsApproved++
		} else {
			report.Summary.ItemsPending++
		}
	}
	for _, item := range items {
		if _, ok := itemState[item.ItemCode]; !ok {
			report.MissingItems = append(report.MissingItems, MissingItem{ItemCode: item.ItemCode, Description: item.Description, Reason: missingItemReason})
		}
	}
	report.Summary.ItemsMissing = len(report.MissingItems)
	if latest.IsZero() {
		latest = time.Now().UTC()
	}
	report.VerifiedAt = latest.UTC().Format(time.RFC3339)
	return report
}

// BuildReviewEntry maps one evidence and its review to the API shape.
func BuildReviewEntry(record Record, review Review, all []Record) ReviewEntry {
	status := review.Status
	if status == "" {
		status = ReviewPending
	}
	source := review.Source
	if source == "" {
		source = ReviewSourceAuto
	}
	reasons := review.Reasons
	if reasons == nil {
		reasons = []ReviewReason{}
	}
	description := ""
	for _, item := range checklist.Items() {
		if item.ItemCode == record.ItemCode {
			description = item.Description
			break
		}
	}
	return ReviewEntry{
		EvidenceID: record.ID, ItemCode: record.ItemCode, ItemDescription: description, SlotNumber: record.SlotNumber,
		Name: record.Name, Status: status, Source: source, Note: review.Note, Reasons: reasons,
		Width: review.Width, Height: review.Height, SHA256: record.SHA256, SharedWith: sharedItemCodes(record, all),
	}
}
