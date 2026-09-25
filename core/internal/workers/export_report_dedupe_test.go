package workers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/evidence"
)

func TestGroupedReportEmbedsEachImageOnce(t *testing.T) {
	dataDir := t.TempDir()
	imageDir := filepath.Join(dataDir, "evidences", "png")
	if err := os.MkdirAll(imageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(imageDir, "shared.png")
	copyPath := filepath.Join(imageDir, "copy.png")
	nohash := filepath.Join(imageDir, "nohash.png")
	for _, path := range []string{shared, copyPath, nohash} {
		if err := os.WriteFile(path, []byte("\x89PNG-fixture-"+filepath.Base(path)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	groups := []evidence.Group{
		// Same bytes captured under two unrelated groups (e.g. 6.1 and 10.1.1).
		{Title: "Grupo A", ItemCodes: []string{"6.1"}, Evidences: []evidence.Record{{ID: "a", ItemCode: "6.1", Name: "a.png", FilePath: shared, Format: "png", SHA256: "same"}}},
		{Title: "Grupo B", ItemCodes: []string{"10.1.1"}, Evidences: []evidence.Record{{ID: "b", ItemCode: "10.1.1", Name: "b.png", FilePath: copyPath, Format: "png", SHA256: "SAME"}}},
		// Same physical file without hash, reached from two groups.
		{Title: "Grupo C", ItemCodes: []string{"1.2.1"}, Evidences: []evidence.Record{{ID: "c", ItemCode: "1.2.1", Name: "c.png", FilePath: nohash, Format: "png"}}},
		{Title: "Grupo D", ItemCodes: []string{"1.2.2"}, Evidences: []evidence.Record{{ID: "d", ItemCode: "1.2.2", Name: "d.png", FilePath: nohash, Format: "png"}}},
	}
	text := buildGroupedReportHTML(dataDir, "Reporte", "ficha", groups, 0)
	if count := strings.Count(text, "data:image/png;base64,"); count != 2 {
		t.Fatalf("expected 2 embedded images, got %d", count)
	}
	if !strings.Contains(text, "Imagen usada como evidencia en los ítems: 10.1.1, 6.1") {
		t.Fatalf("shared hash caption missing: %s", text)
	}
	if !strings.Contains(text, "Imagen usada como evidencia en los ítems: 1.2.1, 1.2.2") {
		t.Fatalf("shared file caption missing: %s", text)
	}
	if strings.Contains(text, "Grupo B") || strings.Contains(text, "Grupo D") {
		t.Fatalf("duplicated groups should be merged into the first occurrence: %s", text)
	}
}
