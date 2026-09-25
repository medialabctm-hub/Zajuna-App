package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// applyV13 keeps one current evidence per (ficha, item, slot, source), then
// replaces UNIQUE(ficha, item, slot, sha256) with UNIQUE(..., source).
func applyV13(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM evidences
		WHERE id IN (
			SELECT older.id
			FROM evidences older
			WHERE EXISTS (
				SELECT 1
				FROM evidences newer
				WHERE COALESCE(newer.ficha_id, '') = COALESCE(older.ficha_id, '')
				  AND newer.item_code = older.item_code
				  AND newer.slot_number = older.slot_number
				  AND newer.source = older.source
				  AND (
					newer.captured_at > older.captured_at
					OR (newer.captured_at = older.captured_at AND newer.id > older.id)
				  )
			)
		)
	`); err != nil {
		return fmt.Errorf("apply schema v13 dedupe: %w", err)
	}

	statements := []string{
		`CREATE TABLE evidences_v13 (
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
			UNIQUE(ficha_id, item_code, slot_number, source)
		)`,
		`INSERT INTO evidences_v13(id, ficha_id, item_code, slot_number, name, file_path, format, source, sha256, metadata_json, captured_at)
		 SELECT id, ficha_id, item_code, slot_number, name, file_path, format, source, sha256, metadata_json, captured_at FROM evidences`,
		`DROP TABLE evidences`,
		`ALTER TABLE evidences_v13 RENAME TO evidences`,
		`CREATE INDEX IF NOT EXISTS idx_evidences_captured_at ON evidences(captured_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_evidences_ficha_item ON evidences(ficha_id, item_code, slot_number)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply schema v13: %w", err)
		}
	}
	return nil
}

func (s *Store) enforceMaxEvidences(ctx context.Context, fichaID, itemCode string) error {
	var max int
	err := s.db.QueryRowContext(ctx, `SELECT max_evidences FROM checklist_catalog_items WHERE item_code = ?`, itemCode).Scan(&max)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read max evidences: %w", err)
	}
	if max <= 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, file_path FROM evidences
		WHERE ficha_id = ? AND item_code = ?
		ORDER BY captured_at DESC, id DESC
	`, fichaID, itemCode)
	if err != nil {
		return fmt.Errorf("list evidences for max enforcement: %w", err)
	}
	defer rows.Close()
	type row struct {
		id, path string
	}
	var all []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.path); err != nil {
			return fmt.Errorf("scan evidence for max enforcement: %w", err)
		}
		all = append(all, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(all) <= max {
		return nil
	}
	for _, item := range all[max:] {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM evidences WHERE id = ?`, item.id); err != nil {
			return fmt.Errorf("delete excess evidence: %w", err)
		}
		if item.path != "" {
			s.removeEvidenceFileIfUnreferenced(ctx, item.path)
		}
	}
	return nil
}

// GarbageCollectEvidenceFiles deletes files under evidences/ that are not
// referenced by the database. Safe to call after migrations and restores.
func (s *Store) GarbageCollectEvidenceFiles(ctx context.Context) error {
	if s == nil || s.dataDir == "" {
		return nil
	}
	root := filepath.Join(s.dataDir, "evidences")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect evidences directory: %w", err)
	}
	if !info.IsDir() {
		return nil
	}

	referenced := map[string]struct{}{}
	rows, err := s.db.QueryContext(ctx, `SELECT file_path FROM evidences WHERE file_path != ''`)
	if err != nil {
		return fmt.Errorf("list evidence file paths: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return fmt.Errorf("scan evidence file path: %w", err)
		}
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			continue
		}
		referenced[abs] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	var orphaned []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return nil
		}
		if _, ok := referenced[abs]; ok {
			return nil
		}
		orphaned = append(orphaned, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk evidences directory: %w", err)
	}
	for _, path := range orphaned {
		_ = os.Remove(path)
	}
	return nil
}

// ClearEvidences removes evidence rows (optionally scoped to a ficha) and their
// local files. Updating the app never does this automatically: data under the
// user data directory survives package updates by design.
func (s *Store) ClearEvidences(ctx context.Context, fichaID string) (deletedRows int, deletedFiles int, err error) {
	fichaID = strings.TrimSpace(fichaID)
	var rows *sql.Rows
	if fichaID == "" {
		rows, err = s.db.QueryContext(ctx, `SELECT id, file_path FROM evidences`)
	} else {
		rows, err = s.db.QueryContext(ctx, `SELECT id, file_path FROM evidences WHERE ficha_id = ?`, fichaID)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("list evidences to clear: %w", err)
	}
	defer rows.Close()

	type row struct {
		id, path string
	}
	var items []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.path); err != nil {
			return 0, 0, fmt.Errorf("scan evidence to clear: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	for _, item := range items {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM evidences WHERE id = ?`, item.id); err != nil {
			return deletedRows, deletedFiles, fmt.Errorf("delete evidence row: %w", err)
		}
		deletedRows++
		if item.path != "" {
			var remaining int
			if countErr := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM evidences WHERE file_path = ?`, item.path).Scan(&remaining); countErr == nil && remaining == 0 {
				if removeErr := os.Remove(item.path); removeErr == nil {
					deletedFiles++
				}
			}
		}
	}
	if fichaID == "" {
		_ = s.GarbageCollectEvidenceFiles(ctx)
	}
	return deletedRows, deletedFiles, nil
}

// PruneCaptureChecklistEvidence deletes "capture-checklist" evidence of a
// ficha whose item is in itemCodes but whose (item, slot) is not in keep:
// slots dropped from the capture plan or skipped as empty row batches. Files
// are removed only once no remaining evidence row references them (covered
// items share one screenshot). Other sources and other items are untouched.
func (s *Store) PruneCaptureChecklistEvidence(ctx context.Context, fichaID string, itemCodes []string, keep map[string]map[int]bool) (int, error) {
	fichaID = strings.TrimSpace(fichaID)
	if fichaID == "" || len(itemCodes) == 0 {
		return 0, nil
	}
	covered := make(map[string]bool, len(itemCodes))
	for _, itemCode := range itemCodes {
		if itemCode = strings.TrimSpace(itemCode); itemCode != "" {
			covered[itemCode] = true
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, item_code, slot_number, file_path FROM evidences
		WHERE ficha_id = ? AND source = 'capture-checklist'
	`, fichaID)
	if err != nil {
		return 0, fmt.Errorf("list checklist evidences to prune: %w", err)
	}
	type row struct {
		id, itemCode, path string
		slot               int
	}
	var stale []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.itemCode, &item.slot, &item.path); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan checklist evidence to prune: %w", err)
		}
		if covered[item.itemCode] && !keep[item.itemCode][item.slot] {
			stale = append(stale, item)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	if len(stale) == 0 {
		return 0, nil
	}
	// All rows go in one transaction so a cancellation or error never leaves
	// half of a slot plan pruned; files are removed only after the commit.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin checklist evidence prune: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range stale {
		if _, err := tx.ExecContext(ctx, `DELETE FROM evidences WHERE id = ?`, item.id); err != nil {
			return 0, fmt.Errorf("delete stale checklist evidence: %w", err)
		}
	}
	unreferenced := make([]string, 0, len(stale))
	for _, item := range stale {
		if item.path == "" {
			continue
		}
		var references int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM evidences WHERE file_path = ?`, item.path).Scan(&references); err != nil {
			return 0, fmt.Errorf("count evidence file references: %w", err)
		}
		if references == 0 {
			unreferenced = append(unreferenced, item.path)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit checklist evidence prune: %w", err)
	}
	for _, path := range unreferenced {
		_ = os.Remove(path)
	}
	return len(stale), nil
}

// ReconcileEvidenceFilesAfterRestore removes leftover files that were not part
// of the restored backup tree / database references.
func (s *Store) ReconcileEvidenceFilesAfterRestore(ctx context.Context) error {
	return s.GarbageCollectEvidenceFiles(ctx)
}
