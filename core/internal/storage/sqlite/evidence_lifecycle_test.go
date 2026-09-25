package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/zajuna"
	_ "modernc.org/sqlite"
)

func TestSchemaMigratesToV13(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var version int
	if err := store.DB().QueryRowContext(context.Background(), `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != currentSchemaVersion || currentSchemaVersion < 13 {
		t.Fatalf("expected schema v13, got %d (currentSchemaVersion=%d)", version, currentSchemaVersion)
	}
	if CurrentSchemaVersion() != currentSchemaVersion {
		t.Fatalf("CurrentSchemaVersion() = %d, want %d", CurrentSchemaVersion(), currentSchemaVersion)
	}

	var sqlText string
	if err := store.DB().QueryRowContext(context.Background(), `SELECT sql FROM sqlite_master WHERE type='table' AND name='evidences'`).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	if !containsAll(sqlText, "UNIQUE(ficha_id, item_code, slot_number, source)") {
		t.Fatalf("evidences table missing v13 unique constraint; sql=%s", sqlText)
	}
}

func TestApplyV13DedupesDuplicateSlotsAndKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "zajuna.db")

	// Build a v12-shaped DB: UNIQUE includes sha256 so duplicates by content are allowed.
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, stmt := range []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`INSERT INTO schema_migrations(version, applied_at) VALUES (12, '2026-01-01T00:00:00Z')`,
		`CREATE TABLE fichas (
			id TEXT PRIMARY KEY,
			external_id TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			course_id TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO fichas(id, external_id, name, course_id, updated_at) VALUES ('ficha-1', '100', 'Ficha', 'c1', '2026-01-01T00:00:00Z')`,
		`CREATE TABLE evidences (
			id TEXT PRIMARY KEY,
			ficha_id TEXT REFERENCES fichas(id) ON DELETE CASCADE,
			item_code TEXT NOT NULL DEFAULT '',
			slot_number INTEGER NOT NULL DEFAULT 1,
			name TEXT NOT NULL,
			file_path TEXT NOT NULL,
			format TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'manual',
			sha256 TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			captured_at TEXT NOT NULL,
			UNIQUE(ficha_id, item_code, slot_number, sha256)
		)`,
		`INSERT INTO evidences(id, ficha_id, item_code, slot_number, name, file_path, format, source, sha256, metadata_json, captured_at) VALUES
			('old', 'ficha-1', '6.1', 1, 'vieja', 'evidences/old.png', 'png', 'capture-checklist', 'hash-old', '{}', '2026-01-01T00:00:00Z'),
			('new', 'ficha-1', '6.1', 1, 'nueva', 'evidences/new.png', 'png', 'capture-checklist', 'hash-new', '{}', '2026-01-02T00:00:00Z')`,
	} {
		if _, err := raw.ExecContext(ctx, stmt); err != nil {
			raw.Close()
			t.Fatalf("seed v12 db: %v\nstmt: %s", err, stmt)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open should migrate v12→v13: %v", err)
	}
	defer store.Close()

	var version int
	if err := store.DB().QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("expected migrated schema v13, got %d", version)
	}

	items, err := store.ListEvidences(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("v13 should keep one evidence per slot/source, got %#v", items)
	}
	if items[0].ID != "new" || items[0].Name != "nueva" {
		t.Fatalf("expected newest row kept, got %#v", items[0])
	}
}

func TestGarbageCollectEvidenceFilesRemovesOrphans(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "200", Name: "Ficha GC", CourseID: "41080"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 1 {
		t.Fatalf("ficha: %#v (%v)", fichas, err)
	}

	evidencesDir := filepath.Join(dir, "evidences")
	if err := os.MkdirAll(evidencesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keptPath := filepath.Join(evidencesDir, "kept.png")
	orphanPath := filepath.Join(evidencesDir, "orphan.png")
	if err := os.WriteFile(keptPath, []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if err := store.CreateEvidence(ctx, evidence.Record{
		ID: "kept-id", FichaID: fichas[0].ID, ItemCode: "6.1", SlotNumber: 1,
		Name: "kept", FilePath: keptPath, Format: "png", Source: "capture-checklist",
		SHA256: "kept-hash", CapturedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.GarbageCollectEvidenceFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keptPath); err != nil {
		t.Fatalf("referenced file should remain: %v", err)
	}
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("orphan file should be removed, stat err=%v", err)
	}
}

func TestClearEvidencesRemovesRowsAndFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{
		{ExternalID: "301", Name: "Ficha A", CourseID: "1"},
		{ExternalID: "302", Name: "Ficha B", CourseID: "2"},
	}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 2 {
		t.Fatalf("fichas: %#v (%v)", fichas, err)
	}

	evidencesDir := filepath.Join(dir, "evidences")
	if err := os.MkdirAll(evidencesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pathA := filepath.Join(evidencesDir, "a.png")
	pathB := filepath.Join(evidencesDir, "b.png")
	pathExtra := filepath.Join(evidencesDir, "extra-orphan.png")
	for _, p := range []string{pathA, pathB, pathExtra} {
		if err := os.WriteFile(p, []byte(p), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC()
	if err := store.CreateEvidence(ctx, evidence.Record{
		ID: "ev-a", FichaID: fichas[0].ID, ItemCode: "6.1", SlotNumber: 1,
		Name: "A", FilePath: pathA, Format: "png", Source: "capture-checklist",
		SHA256: "hash-a", CapturedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEvidence(ctx, evidence.Record{
		ID: "ev-b", FichaID: fichas[1].ID, ItemCode: "6.1", SlotNumber: 1,
		Name: "B", FilePath: pathB, Format: "png", Source: "capture-checklist",
		SHA256: "hash-b", CapturedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	deletedRows, deletedFiles, err := store.ClearEvidences(ctx, fichas[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if deletedRows != 1 || deletedFiles != 1 {
		t.Fatalf("scoped clear: deletedRows=%d deletedFiles=%d", deletedRows, deletedFiles)
	}
	if _, err := os.Stat(pathA); !os.IsNotExist(err) {
		t.Fatal("ficha A file should be gone")
	}
	if _, err := os.Stat(pathB); err != nil {
		t.Fatalf("ficha B file should remain: %v", err)
	}

	deletedRows, deletedFiles, err = store.ClearEvidences(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if deletedRows != 1 {
		t.Fatalf("global clear should delete remaining row, got rows=%d", deletedRows)
	}
	if _, err := os.Stat(pathB); !os.IsNotExist(err) {
		t.Fatal("ficha B file should be gone after global clear")
	}
	if _, err := os.Stat(pathExtra); !os.IsNotExist(err) {
		t.Fatal("global clear should GC orphan files")
	}

	left, err := store.ListEvidences(ctx, 10)
	if err != nil || len(left) != 0 {
		t.Fatalf("expected empty evidences, got %#v (%v)", left, err)
	}
}

func TestCreateEvidenceUpsertReplacesFileForSameSlot(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "400", Name: "Ficha", CourseID: "9"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 1 {
		t.Fatalf("ficha: %#v (%v)", fichas, err)
	}

	evidencesDir := filepath.Join(dir, "evidences")
	if err := os.MkdirAll(evidencesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(evidencesDir, "old.png")
	newPath := filepath.Join(evidencesDir, "new.png")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	base := evidence.Record{
		ID: "id-old", FichaID: fichas[0].ID, ItemCode: "6.1", SlotNumber: 1,
		Name: "old", FilePath: oldPath, Format: "png", Source: "manual",
		SHA256: "sha-old", CapturedAt: now,
	}
	if err := store.CreateEvidence(ctx, base); err != nil {
		t.Fatal(err)
	}
	base.ID = "id-new"
	base.Name = "new"
	base.FilePath = newPath
	base.SHA256 = "sha-new"
	base.CapturedAt = now.Add(time.Minute)
	if err := store.CreateEvidence(ctx, base); err != nil {
		t.Fatal(err)
	}

	items, err := store.ListEvidences(ctx, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("upsert should keep one row: %#v (%v)", items, err)
	}
	if items[0].ID != "id-new" || items[0].FilePath != newPath {
		t.Fatalf("unexpected current evidence: %#v", items[0])
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("previous file should be removed on path change")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new file should remain: %v", err)
	}
}

func TestReconcileEvidenceFilesAfterRestore(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	evidencesDir := filepath.Join(dir, "evidences")
	if err := os.MkdirAll(evidencesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(evidencesDir, "leftover-from-backup.png")
	if err := os.WriteFile(leftover, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileEvidenceFilesAfterRestore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Fatal("reconcile after restore should remove unreferenced files")
	}
}

func containsAll(s, substr string) bool {
	return strings.Contains(s, substr)
}
