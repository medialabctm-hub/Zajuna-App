package workers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/reports"
)

const ExportReportWorkerID = "export-report"

type ExportReportInput struct {
	Title         string `json:"title"`
	Format        string `json:"format"`
	FichaID       string `json:"fichaId,omitempty"`
	OutputPath    string `json:"outputPath,omitempty"`
	EvidenceLimit int    `json:"evidenceLimit,omitempty"`
}

type ExportReportWorker struct {
	dataDir       string
	evidenceStore evidence.Store
	reportStore   reports.Store
	browser       capture.Runtime
}

func NewExportReportWorker(dataDir string, evidenceStore evidence.Store, reportStore reports.Store, browser capture.Runtime) (*ExportReportWorker, error) {
	if dataDir == "" || evidenceStore == nil || reportStore == nil {
		return nil, errors.New("export report worker requires data directory and stores")
	}
	return &ExportReportWorker{dataDir: dataDir, evidenceStore: evidenceStore, reportStore: reportStore, browser: browser}, nil
}

func (w *ExportReportWorker) ID() string { return ExportReportWorkerID }

func (w *ExportReportWorker) Execute(ctx context.Context, job jobs.Job, reporter jobs.Reporter) jobs.Result {
	var input ExportReportInput
	if err := json.Unmarshal(job.Input, &input); err != nil {
		return jobs.Result{ErrorCode: "invalid_input", ErrorMessage: "input de reporte inválido"}
	}
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format == "" {
		format = "pdf"
	}
	if format != "pdf" && format != "html" {
		return jobs.Result{ErrorCode: "invalid_input", ErrorMessage: "format debe ser pdf o html"}
	}
	if input.Title == "" {
		input.Title = "Reporte de evidencias Zajuna"
	}
	limit := input.EvidenceLimit
	if limit <= 0 {
		limit = 100
	}
	var evidences []evidence.Record
	var groups []evidence.Group
	totalGroups := 0
	var err error
	if strings.TrimSpace(input.FichaID) != "" {
		if groupStore, ok := w.evidenceStore.(evidence.GroupStore); ok {
			if _, err := groupStore.RebuildEvidenceGroups(ctx, input.FichaID); err != nil {
				return jobs.Result{ErrorCode: "evidence_group_failed", ErrorMessage: err.Error()}
			}
			groups, err = groupStore.ListEvidenceGroups(ctx, input.FichaID)
			if err != nil {
				return jobs.Result{ErrorCode: "evidence_group_read_failed", ErrorMessage: err.Error()}
			}
			// Each merged group is one image in the report, so the limit
			// counts report entries rather than the rows folded into them.
			groups = mergeReportGroupsByImage(groups)
			totalGroups = len(groups)
			if len(groups) > limit {
				groups = groups[:limit]
			}
			for _, group := range groups {
				evidences = append(evidences, group.Evidences...)
			}
		} else {
			evidences, err = w.evidenceStore.ListEvidences(ctx, limit)
			if err != nil {
				return jobs.Result{ErrorCode: "evidence_read_failed", ErrorMessage: err.Error()}
			}
		}
	} else {
		evidences, err = w.evidenceStore.ListEvidences(ctx, limit)
		if err != nil {
			return jobs.Result{ErrorCode: "evidence_read_failed", ErrorMessage: err.Error()}
		}
	}
	root := filepath.Join(w.dataDir, "reports")
	fallback := fmt.Sprintf("%s.%s", job.ID, format)
	requestedPath := input.OutputPath
	if requestedPath != "" && !filepath.IsAbs(requestedPath) {
		requestedPath = filepath.Join(w.dataDir, requestedPath)
	}
	outputPath, err := safeArtifactPath(root, requestedPath, fallback)
	if err != nil {
		return jobs.Result{ErrorCode: "invalid_output_path", ErrorMessage: err.Error()}
	}
	if err := reporter.Progress(ctx, "rendering", 25, fmt.Sprintf("Preparando reporte con %d evidencias", len(evidences))); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
	}
	omittedGroups := totalGroups - len(groups)
	htmlContent := buildReportHTML(w.dataDir, input.Title, evidences)
	if len(groups) > 0 {
		htmlContent = buildGroupedReportHTML(w.dataDir, input.Title, input.FichaID, groups, omittedGroups)
	}
	if format == "html" {
		if err := os.WriteFile(outputPath, []byte(htmlContent), 0o600); err != nil {
			return jobs.Result{ErrorCode: "report_write_failed", ErrorMessage: err.Error()}
		}
	} else {
		if !w.browser.Installed() {
			return jobs.Result{ErrorCode: "browser_not_installed", ErrorMessage: fmt.Sprintf("runtime Chromium no instalado en %s", w.browser.Root)}
		}
		if err := w.browser.RenderHTMLToPDF(ctx, htmlContent, outputPath); err != nil {
			return jobs.Result{ErrorCode: "report_pdf_failed", ErrorMessage: err.Error(), Retryable: true}
		}
	}
	hash, err := fileSHA256(outputPath)
	if err != nil {
		return jobs.Result{ErrorCode: "report_hash_failed", ErrorMessage: err.Error()}
	}
	reportID := artifactID("report", "", input.Title, hash)
	metadata, _ := json.Marshal(map[string]any{"title": input.Title, "fichaId": input.FichaID, "evidenceCount": len(evidences), "groupCount": len(groups), "evidenceLimit": limit, "omittedGroups": omittedGroups, "jobId": job.ID})
	if err := w.reportStore.CreateReport(ctx, reports.Record{ID: reportID, Name: input.Title, FilePath: outputPath, Format: format, Status: "completed", SHA256: hash, Metadata: metadata, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); err != nil {
		return jobs.Result{ErrorCode: "report_persist_failed", ErrorMessage: err.Error()}
	}
	if err := reporter.Progress(ctx, "completed", 100, "Reporte local generado"); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
	}
	return jobs.Result{Output: map[string]any{"reportId": reportID, "path": outputPath, "format": format, "sha256": hash, "evidenceCount": len(evidences), "groupCount": len(groups), "omittedGroups": omittedGroups}}
}

// reportImageKey identifies the visual content of a record so the report never
// embeds the same image twice: SHA-256 first, the cleaned file path otherwise.
func reportImageKey(item evidence.Record) string {
	if hash := strings.ToLower(strings.TrimSpace(item.SHA256)); hash != "" {
		return "hash:" + hash
	}
	if path := strings.TrimSpace(item.FilePath); path != "" {
		return "path:" + strings.ToLower(filepath.Clean(path))
	}
	return ""
}

// mergeReportGroupsByImage folds groups whose representative image is the same
// content (same SHA-256 or same file) into the first one, accumulating the
// covered item codes. Group order is preserved.
func mergeReportGroupsByImage(groups []evidence.Group) []evidence.Group {
	merged := make([]evidence.Group, 0, len(groups))
	indexByKey := make(map[string]int)
	for _, group := range groups {
		key := ""
		if len(group.Evidences) > 0 {
			key = reportImageKey(latestEvidence(group.Evidences))
		}
		if key == "" {
			merged = append(merged, group)
			continue
		}
		if index, ok := indexByKey[key]; ok {
			target := &merged[index]
			for _, code := range group.ItemCodes {
				target.ItemCodes = appendUniqueString(target.ItemCodes, code)
			}
			target.EvidenceIDs = append(target.EvidenceIDs, group.EvidenceIDs...)
			target.Evidences = append(target.Evidences, group.Evidences...)
			continue
		}
		group.ItemCodes = append([]string(nil), group.ItemCodes...)
		indexByKey[key] = len(merged)
		merged = append(merged, group)
	}
	for index := range merged {
		sort.Strings(merged[index].ItemCodes)
	}
	return merged
}

func appendUniqueString(values []string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func buildGroupedReportHTML(dataDir, title, fichaID string, groups []evidence.Group, omitted int) string {
	var summaryRows strings.Builder
	var sections strings.Builder
	groups = mergeReportGroupsByImage(groups)
	embedded := make(map[string]int)
	for index, group := range groups {
		items := html.EscapeString(strings.Join(group.ItemCodes, ", "))
		summaryRows.WriteString("<tr><td>")
		summaryRows.WriteString(fmt.Sprintf("%d", index+1))
		summaryRows.WriteString("</td><td>")
		summaryRows.WriteString(html.EscapeString(group.Title))
		summaryRows.WriteString("</td><td>")
		summaryRows.WriteString(items)
		summaryRows.WriteString("</td><td>")
		summaryRows.WriteString(html.EscapeString(group.Confidence))
		summaryRows.WriteString("</td></tr>")

		sections.WriteString(`<section class="evidence-group"><h2>`)
		sections.WriteString(fmt.Sprintf("%d. %s", index+1, html.EscapeString(group.Title)))
		sections.WriteString(`</h2><p class="meta"><strong>Tareas cubiertas:</strong> `)
		sections.WriteString(items)
		sections.WriteString(` · <strong>Confianza:</strong> `)
		sections.WriteString(html.EscapeString(group.Confidence))
		sections.WriteString(` · `)
		sections.WriteString(html.EscapeString(group.Reason))
		sections.WriteString(`</p>`)
		if len(group.Evidences) == 0 {
			sections.WriteString(`<p>No hay archivo asociado a este grupo.</p>`)
		} else {
			representative := latestEvidence(group.Evidences)
			imageKey := reportImageKey(representative)
			if previous, seen := embedded[imageKey]; imageKey != "" && seen {
				sections.WriteString(`<p class="file-note">Misma imagen que el grupo `)
				sections.WriteString(fmt.Sprintf("%d", previous))
				sections.WriteString(`; no se repite en el reporte.</p>`)
			} else if image := embeddedEvidenceImage(dataDir, representative); image != "" {
				if imageKey != "" {
					embedded[imageKey] = index + 1
				}
				sections.WriteString(`<figure class="evidence-figure"><img class="evidence-image" src="`)
				sections.WriteString(image)
				sections.WriteString(`" alt="`)
				sections.WriteString(html.EscapeString(representative.Name))
				sections.WriteString(`"><figcaption class="meta">`)
				if len(group.ItemCodes) > 1 {
					sections.WriteString(`Imagen usada como evidencia en los ítems: `)
				} else {
					sections.WriteString(`Evidencia del ítem: `)
				}
				sections.WriteString(items)
				sections.WriteString(`</figcaption></figure>`)
			} else {
				sections.WriteString(`<p class="file-note">Archivo disponible localmente: `)
				sections.WriteString(html.EscapeString(representative.Name))
				sections.WriteString(` (`)
				sections.WriteString(html.EscapeString(representative.Format))
				sections.WriteString(`).</p>`)
			}
			sections.WriteString(`<p class="meta">Representaciones agrupadas: `)
			sections.WriteString(fmt.Sprintf("%d", len(group.Evidences)))
			sections.WriteString(` · Capturada: `)
			sections.WriteString(html.EscapeString(representative.CapturedAt.Format(time.RFC3339)))
			sections.WriteString(`</p>`)
		}
		sections.WriteString(`</section>`)
	}
	if len(groups) == 0 {
		summaryRows.WriteString(`<tr><td colspan="4">No hay grupos de evidencia locales.</td></tr>`)
	}
	if omitted > 0 {
		summaryRows.WriteString(fmt.Sprintf(`<tr><td colspan="4" class="file-note">Se omitieron %d evidencias por el límite configurado del reporte.</td></tr>`, omitted))
	}
	return fmt.Sprintf(`<!doctype html><html lang="es"><head><meta charset="utf-8"><title>%s</title><style>
body{font-family:Arial,sans-serif;color:#172033;margin:36px}h1{color:#145d5a;margin-bottom:6px}h2{color:#145d5a;font-size:18px;margin:0 0 8px}p{color:#526173}.meta{font-size:11px;line-height:1.5}table{width:100%%;border-collapse:collapse;margin-top:20px}th,td{text-align:left;border-bottom:1px solid #d9e1ea;padding:9px;font-size:11px;vertical-align:top}th{background:#edf5f4}.evidence-group{break-inside:avoid;border-top:2px solid #d9e1ea;margin-top:28px;padding-top:16px}.evidence-image{display:block;max-width:100%%;max-height:720px;object-fit:contain;border:1px solid #d9e1ea;margin-top:12px}.evidence-figure{margin:0}.file-note{background:#f5f7fa;padding:10px}
</style></head><body><h1>%s</h1><p>Reporte agrupado local · Ficha: %s · Generado: %s</p><h2>Resumen de evidencias</h2><table><thead><tr><th>#</th><th>Grupo</th><th>Tareas cubiertas</th><th>Confianza</th></tr></thead><tbody>%s</tbody></table>%s</body></html>`, html.EscapeString(title), html.EscapeString(title), html.EscapeString(fichaID), time.Now().UTC().Format(time.RFC3339), summaryRows.String(), sections.String())
}

func latestEvidence(items []evidence.Record) evidence.Record {
	if len(items) == 0 {
		return evidence.Record{}
	}
	latest := items[0]
	for _, item := range items[1:] {
		if item.CapturedAt.After(latest.CapturedAt) {
			latest = item
		}
	}
	return latest
}

func embeddedEvidenceImage(dataDir string, item evidence.Record) string {
	format := strings.ToLower(strings.TrimSpace(item.Format))
	mimeType := map[string]string{"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg", "webp": "image/webp"}[format]
	if mimeType == "" || item.FilePath == "" {
		return ""
	}
	root, err := filepath.Abs(filepath.Join(dataDir, "evidences"))
	if err != nil {
		return ""
	}
	path := item.FilePath
	if !filepath.IsAbs(path) {
		// Relative paths are stored relative to the data directory, never
		// to the process working directory.
		path = filepath.Join(dataDir, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return ""
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	contents, err := os.ReadFile(path)
	if err != nil || len(contents) == 0 || len(contents) > 12<<20 {
		return ""
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(contents)
}

func buildReportHTML(dataDir, title string, evidences []evidence.Record) string {
	var rows strings.Builder
	var figures strings.Builder
	embedded := make(map[string]int)
	for index, item := range evidences {
		rows.WriteString("<tr><td>")
		rows.WriteString(fmt.Sprintf("%d", index+1))
		rows.WriteString("</td><td>")
		rows.WriteString(html.EscapeString(item.Name))
		rows.WriteString("</td><td>")
		rows.WriteString(html.EscapeString(item.Format))
		rows.WriteString("</td><td>")
		rows.WriteString(html.EscapeString(item.SHA256))
		rows.WriteString("</td><td>")
		rows.WriteString(html.EscapeString(item.CapturedAt.Format(time.RFC3339)))
		rows.WriteString("</td></tr>")

		imageKey := reportImageKey(item)
		if _, seen := embedded[imageKey]; imageKey != "" && seen {
			continue
		}
		image := embeddedEvidenceImage(dataDir, item)
		if image == "" {
			continue
		}
		if imageKey != "" {
			embedded[imageKey] = index + 1
		}
		figures.WriteString(`<figure class="evidence-figure"><img class="evidence-image" src="`)
		figures.WriteString(image)
		figures.WriteString(`" alt="`)
		figures.WriteString(html.EscapeString(item.Name))
		figures.WriteString(`"><figcaption class="meta">`)
		figures.WriteString(fmt.Sprintf("%d. %s", index+1, html.EscapeString(item.Name)))
		figures.WriteString(`</figcaption></figure>`)
	}
	if len(evidences) == 0 {
		rows.WriteString(`<tr><td colspan="5">No hay evidencias locales.</td></tr>`)
	}
	return fmt.Sprintf(`<!doctype html><html lang="es"><head><meta charset="utf-8"><title>%s</title><style>body{font-family:Arial,sans-serif;color:#172033;margin:40px}h1{color:#145d5a}p{color:#526173}.meta{font-size:11px;line-height:1.5}table{width:100%%;border-collapse:collapse;margin-top:24px}th,td{text-align:left;border-bottom:1px solid #d9e1ea;padding:10px;font-size:12px}th{background:#edf5f4}.evidence-figure{break-inside:avoid;margin:28px 0 0}.evidence-image{display:block;max-width:100%%;max-height:720px;object-fit:contain;border:1px solid #d9e1ea}</style></head><body><h1>%s</h1><p>Generado localmente por Zajuna App · %s</p><table><thead><tr><th>#</th><th>Evidencia</th><th>Formato</th><th>SHA-256</th><th>Capturada</th></tr></thead><tbody>%s</tbody></table>%s</body></html>`, html.EscapeString(title), html.EscapeString(title), time.Now().UTC().Format(time.RFC3339), rows.String(), figures.String())
}

func reportHash(contents []byte) string {
	hash := sha256.Sum256(contents)
	return hex.EncodeToString(hash[:])
}
