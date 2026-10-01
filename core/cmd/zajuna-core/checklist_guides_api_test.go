package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/zajuna"
)

func TestChecklistGuidesAPIGuidesItemsOutOfTheAppsReach(t *testing.T) {
	dataDir := t.TempDir()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "3135429", Name: "Ficha", CourseID: "41080"}}); err != nil {
		t.Fatal(err)
	}
	fichas, _ := store.ListFichas(ctx, 10)
	fichaID := fichas[0].ID
	// 7.3.2: the subsection exists but is empty.
	path := filepath.Join(dataDir, "evidences", "empty.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "ev-1", FichaID: fichaID, ItemCode: "7.3.2", SlotNumber: 1, Name: "Documentos de retención", FilePath: path, Format: "png", Source: "capture-checklist", SHA256: "sha-1", Metadata: []byte(`{"contentItems":0}`)}); err != nil {
		t.Fatal(err)
	}
	// 14.1.1: the last capture found the forum without a conclusion.
	if _, err := store.DB().ExecContext(ctx, `INSERT INTO jobs(id, type, status, input_json, result_json, created_at, updated_at)
		VALUES('job-1', 'capture-checklist', 'completed', ?, ?, '2026-09-24T10:00:00Z', '2026-09-24T10:00:00Z')`,
		`{"fichaId":"`+fichaID+`"}`, `{"absences":["14.1.1: selector not found (sin contenido en Zajuna): la lista no tiene publicaciones del instructor autenticado sobre «conclusión»"]}`); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(newRouterWithServices(dataDir, &memoryCredentialStore{}, nil, store, nil))
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/api/checklist/guides?fichaId=" + fichaID)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	var view checklistGuidesView
	if err := json.NewDecoder(response.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]checklist.Guide{}
	for _, guide := range view.Guides {
		kinds[guide.ItemCode] = guide
	}
	if kinds["7.3.2"].Kind != checklist.GuideEmptySection || kinds["14.1.1"].Kind != checklist.GuideContentAbsent {
		t.Fatalf("guides = %#v", view.Guides)
	}
	if kinds["14.1.1"].Template == nil || kinds["14.1.1"].Detected == "" {
		t.Fatalf("14.1.1 guide = %#v", kinds["14.1.1"])
	}
	if view.MapReady || len(view.Guides) != 2 {
		t.Fatalf("without a course map only content guides apply: %#v", view)
	}

	missing, err := server.Client().Get(server.URL + "/api/checklist/guides")
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing fichaId status %d", missing.StatusCode)
	}
}
