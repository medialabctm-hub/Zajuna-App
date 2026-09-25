package workers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/security"
)

const CaptureEvidenceWorkerID = "capture-evidence"

const maxEvidenceHTMLSize = 32 << 20

type CaptureEvidenceInput struct {
	URL        string `json:"url"`
	OutputPath string `json:"outputPath,omitempty"`
	FichaID    string `json:"fichaId,omitempty"`
	ItemCode   string `json:"itemCode,omitempty"`
	SlotNumber int    `json:"slotNumber,omitempty"`
	Name       string `json:"name,omitempty"`
}

type CaptureEvidenceWorker struct {
	dataDir        string
	store          evidence.Store
	client         *http.Client
	allowedOrigins []string
}

func NewCaptureEvidenceWorker(dataDir string, store evidence.Store, allowedOrigins ...string) (*CaptureEvidenceWorker, error) {
	if dataDir == "" || store == nil {
		return nil, errors.New("capture evidence worker requires data directory and store")
	}
	worker := &CaptureEvidenceWorker{dataDir: dataDir, store: store, allowedOrigins: append([]string(nil), allowedOrigins...)}
	worker.client = &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			if _, err := security.ValidateHTTPURL(request.URL.String(), worker.allowedOrigins, len(worker.allowedOrigins) == 0); err != nil {
				return fmt.Errorf("redirección bloqueada: %w", err)
			}
			return nil
		},
	}
	return worker, nil
}

func (w *CaptureEvidenceWorker) ID() string { return CaptureEvidenceWorkerID }

func (w *CaptureEvidenceWorker) Execute(ctx context.Context, job jobs.Job, reporter jobs.Reporter) jobs.Result {
	var input CaptureEvidenceInput
	if err := json.Unmarshal(job.Input, &input); err != nil {
		return jobs.Result{ErrorCode: "invalid_input", ErrorMessage: "input de evidencia inválido"}
	}
	input.URL = strings.TrimSpace(input.URL)
	parsed, err := security.ValidateHTTPURL(input.URL, w.allowedOrigins, len(w.allowedOrigins) == 0)
	if err != nil {
		return jobs.Result{ErrorCode: "invalid_input", ErrorMessage: err.Error()}
	}
	// The request keeps its query (tokens included); only what is stored or
	// reported goes through RedactURL.
	input.URL = parsed.String()
	if input.SlotNumber <= 0 {
		input.SlotNumber = 1
	}
	requestedPath := input.OutputPath
	if requestedPath != "" && !filepath.IsAbs(requestedPath) {
		requestedPath = filepath.Join(w.dataDir, requestedPath)
	}
	outputPath, err := safeArtifactPath(filepath.Join(w.dataDir, "evidences", "html"), requestedPath, job.ID+".html")
	if err != nil {
		return jobs.Result{ErrorCode: "invalid_output_path", ErrorMessage: err.Error()}
	}
	if err := reporter.Progress(ctx, "fetch", 10, "Descargando evidencia HTML"); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return jobs.Result{ErrorCode: "request_failed", ErrorMessage: err.Error()}
	}
	request.Header.Set("User-Agent", "Zajuna App/0.1 (+local)")
	response, err := w.client.Do(request)
	if err != nil {
		return jobs.Result{Retryable: true, ErrorCode: "evidence_fetch_failed", ErrorMessage: fmt.Sprintf("no se pudo descargar la evidencia: %v", err)}
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests {
		return jobs.Result{Retryable: true, ErrorCode: "evidence_http_retryable", ErrorMessage: fmt.Sprintf("la URL respondió HTTP %d", response.StatusCode)}
	}
	if response.StatusCode >= 400 {
		return jobs.Result{ErrorCode: "evidence_http_failed", ErrorMessage: fmt.Sprintf("la URL respondió HTTP %d", response.StatusCode)}
	}
	if response.ContentLength > maxEvidenceHTMLSize {
		return jobs.Result{ErrorCode: "evidence_too_large", ErrorMessage: "la evidencia HTML supera el límite local de 32 MB"}
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxEvidenceHTMLSize+1))
	if err != nil {
		return jobs.Result{Retryable: true, ErrorCode: "evidence_read_failed", ErrorMessage: err.Error()}
	}
	if len(contents) > maxEvidenceHTMLSize {
		return jobs.Result{ErrorCode: "evidence_too_large", ErrorMessage: "la evidencia HTML supera el límite local de 32 MB"}
	}
	if err := reporter.Progress(ctx, "persisting", 70, "Guardando HTML y calculando hash"); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
	}
	if err := os.WriteFile(outputPath, contents, 0o600); err != nil {
		return jobs.Result{ErrorCode: "evidence_write_failed", ErrorMessage: err.Error()}
	}
	hashBytes := sha256.Sum256(contents)
	hash := hex.EncodeToString(hashBytes[:])
	metadata, _ := json.Marshal(map[string]any{"url": security.RedactURL(input.URL), "status": response.StatusCode, "contentType": response.Header.Get("Content-Type"), "sizeBytes": len(contents), "jobId": job.ID})
	name := input.Name
	if name == "" {
		name = filepath.Base(outputPath)
	}
	evidenceID := artifactID("evidence", input.FichaID, input.ItemCode, hash)
	if err := w.store.CreateEvidence(ctx, evidence.Record{ID: evidenceID, FichaID: input.FichaID, ItemCode: input.ItemCode, SlotNumber: input.SlotNumber, Name: name, FilePath: outputPath, Format: "html", Source: "capture-evidence", SHA256: hash, Metadata: metadata, CapturedAt: time.Now().UTC()}); err != nil {
		return jobs.Result{ErrorCode: "evidence_persist_failed", ErrorMessage: err.Error()}
	}
	if err := reporter.Progress(ctx, "completed", 100, "Evidencia HTML guardada localmente"); err != nil {
		return jobs.Result{ErrorCode: "progress_failed", ErrorMessage: err.Error()}
	}
	return jobs.Result{Output: map[string]any{"url": security.RedactURL(input.URL), "path": outputPath, "format": "html", "sha256": hash, "evidenceId": evidenceID, "status": response.StatusCode}}
}
