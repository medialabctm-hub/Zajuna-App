package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/zajuna"
)

func TestPruneCaptureChecklistEvidenceDropsUnplannedAndSkippedSlots(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "500", Name: "Ficha", CourseID: "9"}, {ExternalID: "501", Name: "Otra", CourseID: "9"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 2 {
		t.Fatalf("fichas: %#v (%v)", fichas, err)
	}
	fichaID, otherFicha := fichas[0].ID, fichas[1].ID

	evidencesDir := filepath.Join(dir, "evidences")
	if err := os.MkdirAll(evidencesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile := func(name string) string {
		path := filepath.Join(evidencesDir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	shared := writeFile("shared-slot-3.png") // referenced by X.1#3 (stale) and X.9#1 (kept)
	paths := map[string]string{
		"X.1#1": writeFile("x1-1.png"), "X.1#2": writeFile("x1-2.png"), "X.1#3": shared,
		"X.2#1": writeFile("x2-1.png"), "X.9#1": shared, "X.1#1@manual": writeFile("manual.png"),
		"X.1#2@other": writeFile("other.png"),
	}
	now := time.Now().UTC()
	create := func(id, ficha, item string, slot int, source, path string) {
		if err := store.CreateEvidence(ctx, evidence.Record{
			ID: id, FichaID: ficha, ItemCode: item, SlotNumber: slot, Name: id, FilePath: path,
			Format: "png", Source: source, SHA256: "sha-" + id, CapturedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	create("x1-1", fichaID, "X.1", 1, "capture-checklist", paths["X.1#1"])
	create("x1-2", fichaID, "X.1", 2, "capture-checklist", paths["X.1#2"])
	create("x1-3", fichaID, "X.1", 3, "capture-checklist", paths["X.1#3"])
	create("x2-1", fichaID, "X.2", 1, "capture-checklist", paths["X.2#1"])
	create("x9-1", fichaID, "X.9", 1, "capture-checklist", paths["X.9#1"]) // item not covered by run
	create("manual", fichaID, "X.1", 1, "manual", paths["X.1#1@manual"])
	create("other", otherFicha, "X.1", 2, "capture-checklist", paths["X.1#2@other"])

	// Run covered X.1 (slot 1 captured, slot 2 failed -> keep, slot 3 skipped)
	// and X.2 whose only slot is no longer planned.
	pruned, err := store.PruneCaptureChecklistEvidence(ctx, fichaID, []string{"X.1", "X.2"}, map[string]map[int]bool{
		"X.1": {1: true, 2: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 2 {
		t.Fatalf("pruned = %d, want 2", pruned)
	}
	items, err := store.ListEvidences(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	want := []string{"manual", "other", "x1-1", "x1-2", "x9-1"}
	if len(ids) != len(want) {
		t.Fatalf("remaining ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("remaining ids = %v, want %v", ids, want)
		}
	}
	if _, err := os.Stat(paths["X.2#1"]); !os.IsNotExist(err) {
		t.Fatalf("unreferenced stale file should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(shared); err != nil {
		t.Fatalf("file still referenced by another evidence must survive: %v", err)
	}
	for _, key := range []string{"X.1#1", "X.1#2", "X.1#1@manual", "X.1#2@other"} {
		if _, err := os.Stat(paths[key]); err != nil {
			t.Fatalf("kept file %s missing: %v", key, err)
		}
	}

	again, err := store.PruneCaptureChecklistEvidence(ctx, fichaID, []string{"X.1", "X.2"}, map[string]map[int]bool{"X.1": {1: true, 2: true}})
	if err != nil || again != 0 {
		t.Fatalf("second prune = %d (%v), want 0", again, err)
	}
	if n, err := store.PruneCaptureChecklistEvidence(ctx, fichaID, nil, nil); err != nil || n != 0 {
		t.Fatalf("empty item list must be a no-op, got %d (%v)", n, err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if n, err := store.PruneCaptureChecklistEvidence(cancelled, fichaID, []string{"X.1"}, nil); err == nil || n != 0 {
		t.Fatalf("a cancelled prune must fail without deleting, got %d (%v)", n, err)
	}
	if remaining, _ := store.ListEvidences(ctx, 50); len(remaining) != len(want) {
		t.Fatalf("a cancelled prune deleted evidence: %d rows left", len(remaining))
	}
	if _, err := os.Stat(paths["X.1#1"]); err != nil {
		t.Fatalf("a cancelled prune removed a file: %v", err)
	}
}
