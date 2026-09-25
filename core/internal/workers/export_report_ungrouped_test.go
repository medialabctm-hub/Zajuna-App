package workers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/storage/sqlite"
)

func TestUngroupedReportEmbedsEachImageOnce(t *testing.T) {
	dataDir := t.TempDir()
	imageDir := filepath.Join(dataDir, "evidences", "browser")
	if err := os.MkdirAll(imageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.png", "two.png"} {
		if err := os.WriteFile(filepath.Join(imageDir, name), []byte("\x89PNG-fixture-"+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(dataDir, "outside.png")
	if err := os.WriteFile(outside, []byte("\x89PNG-outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	evidences := []evidence.Record{
		{ID: "one", Name: "Captura uno", FilePath: filepath.Join(imageDir, "one.png"), Format: "png", SHA256: "hash-one"},
		// Same content under another record: shown once.
		{ID: "one-alias", Name: "Alias de uno", FilePath: filepath.Join(imageDir, "one.png"), Format: "png", SHA256: "HASH-ONE"},
		// Stored relative to the data directory, not to the working directory.
		{ID: "two", Name: "Captura dos", FilePath: filepath.Join("evidences", "browser", "two.png"), Format: "png", SHA256: "hash-two"},
		// Outside evidences/: listed but never read.
		{ID: "outside", Name: "Fuera", FilePath: outside, Format: "png", SHA256: "hash-outside"},
		{ID: "html", Name: "fixture.html", FilePath: filepath.Join(imageDir, "fixture.html"), Format: "html", SHA256: "hash-html"},
	}
	text := buildReportHTML(dataDir, "Reporte", evidences)
	if count := strings.Count(text, "data:image/png;base64,"); count != 2 {
		t.Fatalf("expected 2 embedded images, got %d: %s", count, text)
	}
	for _, name := range []string{"Captura uno", "Alias de uno", "Captura dos", "Fuera", "fixture.html"} {
		if !strings.Contains(text, name) {
			t.Fatalf("summary table lost %q", name)
		}
	}
	if strings.Contains(text, `alt="Fuera"`) || strings.Contains(text, `alt="Alias de uno"`) {
		t.Fatalf("outside or duplicated image was embedded: %s", text)
	}
}

func TestExportReportWorkerEmbedsImagesWithoutFicha(t *testing.T) {
	dataDir := t.TempDir()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	imagePath := filepath.Join(dataDir, "evidences", "browser", "shot.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("\x89PNG-fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEvidence(context.Background(), evidence.Record{ID: "evidence-shot", Name: "shot.png", FilePath: imagePath, Format: "png", Source: "capture-browser", SHA256: "shot-hash"}); err != nil {
		t.Fatal(err)
	}
	worker, err := NewExportReportWorker(dataDir, store, store, capture.Resolve(""))
	if err != nil {
		t.Fatal(err)
	}
	result := worker.Execute(context.Background(), jobs.Job{ID: "job-ungrouped", Input: []byte(`{"title":"Reporte sin ficha","format":"html"}`)}, captureReporter{})
	if result.ErrorMessage != "" {
		t.Fatalf("report failed: %#v", result)
	}
	contents, err := os.ReadFile(result.Output.(map[string]any)["path"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "data:image/png;base64,") {
		t.Fatal("report without ficha does not embed the evidence image")
	}
}
