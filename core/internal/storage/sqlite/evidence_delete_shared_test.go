package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/zajuna"
)

func TestDeleteEvidenceKeepsFileSharedByAnotherItem(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "700", Name: "Ficha", CourseID: "9"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 1 {
		t.Fatalf("fichas: %#v (%v)", fichas, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "evidences"), 0o755); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(dir, "evidences", "slot-1.png")
	if err := os.WriteFile(shared, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"10.1.1", "10.1.2"} {
		if err := store.CreateEvidence(ctx, evidence.Record{
			ID: "e-" + item, FichaID: fichas[0].ID, ItemCode: item, SlotNumber: 1, Name: item, FilePath: shared,
			Format: "png", Source: "capture-checklist", SHA256: "same", CapturedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.DeleteEvidence(ctx, "e-10.1.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shared); err != nil {
		t.Fatal("the image is still used by 10.1.2 and must not be deleted")
	}
	if _, err := store.DeleteEvidence(ctx, "e-10.1.2"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shared); !os.IsNotExist(err) {
		t.Fatal("the image must be removed once no evidence references it")
	}
}

func TestReplacingEvidenceKeepsPreviousFileSharedByAnotherItem(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "701", Name: "Ficha", CourseID: "9"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 1 {
		t.Fatalf("fichas: %#v (%v)", fichas, err)
	}
	evidencesDir := filepath.Join(dir, "evidences")
	if err := os.MkdirAll(evidencesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(evidencesDir, "shared.png")
	replacement := filepath.Join(evidencesDir, "replacement.png")
	for _, path := range []string{shared, replacement} {
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	create := func(id, item, path string) {
		if err := store.CreateEvidence(ctx, evidence.Record{
			ID: id, FichaID: fichas[0].ID, ItemCode: item, SlotNumber: 1, Name: id, FilePath: path,
			Format: "png", Source: "capture-checklist", SHA256: id, CapturedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	create("first", "10.1.1", shared)
	create("other", "10.1.2", shared)
	create("replacement", "10.1.1", replacement)
	if _, err := os.Stat(shared); err != nil {
		t.Fatalf("shared image removed while another evidence references it: %v", err)
	}
}

func TestEvidenceRemovalNeverDeletesFilesOutsideEvidences(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "700", Name: "Ficha", CourseID: "9"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 1 {
		t.Fatalf("fichas: %#v (%v)", fichas, err)
	}
	// A restored or tampered database points two rows at user files.
	outside := t.TempDir()
	for index, item := range []string{"10.1.1", "10.1.2"} {
		path := filepath.Join(outside, item+".png")
		if err := os.WriteFile(path, []byte("user file"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateEvidence(ctx, evidence.Record{
			ID: "e-" + item, FichaID: fichas[0].ID, ItemCode: item, SlotNumber: 1, Name: item, FilePath: path,
			Format: "png", Source: "capture-checklist", SHA256: "h" + item, CapturedAt: time.Now().UTC().Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.DeleteEvidence(ctx, "e-10.1.1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ClearEvidences(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"10.1.1", "10.1.2"} {
		if _, err := os.Stat(filepath.Join(outside, item+".png")); err != nil {
			t.Fatalf("a file outside evidences/ must never be deleted (%s): %v", item, err)
		}
	}
}

func TestReconcileAfterRestoreKeepsEvidencesFromAnotherDataFolder(t *testing.T) {
	ctx := context.Background()
	dirA, dirB := t.TempDir(), t.TempDir()
	store, err := Open(dirA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "700", Name: "Ficha", CourseID: "9"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 1 {
		t.Fatalf("fichas: %#v (%v)", fichas, err)
	}
	if err := store.CreateEvidence(ctx, evidence.Record{
		ID: "e-1", FichaID: fichas[0].ID, ItemCode: "10.1.1", SlotNumber: 1, Name: "x", FilePath: filepath.Join(dirA, "evidences", "manual", "x.png"),
		Format: "png", Source: "manual", SHA256: "h", CapturedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// The restored backup brings the database of dirA and its files into dirB.
	database, err := os.ReadFile(filepath.Join(dirA, "zajuna.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "zajuna.db"), database, 0o600); err != nil {
		t.Fatal(err)
	}
	restoredFile := filepath.Join(dirB, "evidences", "manual", "x.png")
	if err := os.MkdirAll(filepath.Dir(restoredFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(restoredFile, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(dirB)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.ReconcileEvidenceFilesAfterRestore(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(restoredFile); err != nil {
		t.Fatalf("a restored evidence must survive the reconciliation: %v", err)
	}
	var stored string
	if err := restored.DB().QueryRowContext(ctx, `SELECT file_path FROM evidences WHERE id = 'e-1'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != restoredFile {
		t.Fatalf("the evidence must point at the restored file, got %s", stored)
	}
}
