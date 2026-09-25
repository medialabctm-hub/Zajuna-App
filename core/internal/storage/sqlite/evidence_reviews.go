package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/evidence"
)

// applyV14 stores the automatic/manual review of each evidence. ON UPDATE
// CASCADE is required because CreateEvidence rewrites evidences.id when a slot
// is recaptured.
func applyV14(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS evidence_reviews (
			evidence_id TEXT PRIMARY KEY REFERENCES evidences(id) ON DELETE CASCADE ON UPDATE CASCADE,
			ficha_id TEXT NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('approved', 'pending', 'rejected')),
			source TEXT NOT NULL CHECK(source IN ('auto', 'manual')),
			reasons_json TEXT NOT NULL DEFAULT '[]',
			note TEXT NOT NULL DEFAULT '',
			sha256 TEXT NOT NULL DEFAULT '',
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_evidence_reviews_ficha ON evidence_reviews(ficha_id)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply schema v14: %w", err)
		}
	}
	return nil
}

func (s *Store) ListEvidenceReviews(ctx context.Context, fichaID string) ([]evidence.Review, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT evidence_id, ficha_id, status, source, reasons_json, note, sha256, width, height, updated_at
		FROM evidence_reviews WHERE ficha_id = ?
	`, fichaID)
	if err != nil {
		return nil, fmt.Errorf("list evidence reviews: %w", err)
	}
	defer rows.Close()
	result := make([]evidence.Review, 0)
	for rows.Next() {
		review, err := scanEvidenceReview(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, review)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read evidence reviews: %w", err)
	}
	return result, nil
}

func (s *Store) GetEvidenceReview(ctx context.Context, evidenceID string) (evidence.Review, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT evidence_id, ficha_id, status, source, reasons_json, note, sha256, width, height, updated_at
		FROM evidence_reviews WHERE evidence_id = ?
	`, evidenceID)
	return scanEvidenceReview(row)
}

func scanEvidenceReview(scanner interface{ Scan(dest ...any) error }) (evidence.Review, error) {
	var review evidence.Review
	var reasons, updatedAt string
	if err := scanner.Scan(&review.EvidenceID, &review.FichaID, &review.Status, &review.Source, &reasons, &review.Note, &review.SHA256, &review.Width, &review.Height, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return evidence.Review{}, err
		}
		return evidence.Review{}, fmt.Errorf("scan evidence review: %w", err)
	}
	review.Reasons = []evidence.ReviewReason{}
	_ = json.Unmarshal([]byte(reasons), &review.Reasons)
	review.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return review, nil
}

func (s *Store) UpsertEvidenceReviews(ctx context.Context, reviews []evidence.Review) error {
	if len(reviews) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin evidence review upsert: %w", err)
	}
	defer tx.Rollback()
	for _, review := range reviews {
		reasons := review.Reasons
		if reasons == nil {
			reasons = []evidence.ReviewReason{}
		}
		encoded, err := json.Marshal(reasons)
		if err != nil {
			return fmt.Errorf("encode review reasons: %w", err)
		}
		updatedAt := review.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = time.Now().UTC()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO evidence_reviews(evidence_id, ficha_id, status, source, reasons_json, note, sha256, width, height, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(evidence_id) DO UPDATE SET
				ficha_id = excluded.ficha_id, status = excluded.status, source = excluded.source,
				reasons_json = excluded.reasons_json, note = excluded.note, sha256 = excluded.sha256,
				width = excluded.width, height = excluded.height, updated_at = excluded.updated_at
			-- An automatic verification read the reviews before analysing the
			-- images; a manual decision saved meanwhile for the same file wins.
			WHERE NOT (evidence_reviews.source = 'manual' AND excluded.source = 'auto' AND evidence_reviews.sha256 = excluded.sha256)
		`, review.EvidenceID, review.FichaID, review.Status, review.Source, string(encoded), review.Note, review.SHA256, review.Width, review.Height, updatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("upsert evidence review: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit evidence reviews: %w", err)
	}
	return nil
}

// VerifyEvidenceReviews re-runs the automatic verification for the whole ficha,
// keeping manual decisions whose sha256 is unchanged.
func (s *Store) VerifyEvidenceReviews(ctx context.Context, fichaID string) (evidence.ReviewReport, error) {
	report, err := evidence.VerifyFicha(ctx, s, s.dataDir, fichaID, false, time.Now().UTC())
	if err != nil {
		return report, err
	}
	if _, err := s.SyncApprovedChecklistItems(ctx, fichaID); err != nil {
		return report, err
	}
	return report, nil
}

// AutoReviewSource marks checklist changes made by SyncApprovedChecklistItems.
const AutoReviewSource = "revision-automatica"

// SyncApprovedChecklistItems marks as fulfilled ("SI") every pending item of
// the ficha whose evidences are all approved, and returns to pending an item
// it marked earlier whose evidence is no longer all approved. A status the
// person set by hand is never changed. It returns how many items changed.
func (s *Store) SyncApprovedChecklistItems(ctx context.Context, fichaID string) (int, error) {
	fichaID = strings.TrimSpace(fichaID)
	if fichaID == "" {
		return 0, nil
	}
	if err := s.EnsureChecklistItems(ctx, fichaID); err != nil {
		return 0, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.item_code, MIN(CASE WHEN r.status = 'approved' THEN 1 ELSE 0 END)
		FROM evidences e LEFT JOIN evidence_reviews r ON r.evidence_id = e.id
		WHERE e.ficha_id = ? AND e.item_code <> ''
		GROUP BY e.item_code`, fichaID)
	if err != nil {
		return 0, fmt.Errorf("list reviewed checklist items: %w", err)
	}
	approved := map[string]bool{}
	for rows.Next() {
		var code string
		var allApproved int
		if err := rows.Scan(&code, &allApproved); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan reviewed checklist item: %w", err)
		}
		approved[code] = allApproved == 1
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin checklist auto review: %w", err)
	}
	defer transaction.Rollback()
	items, err := transaction.QueryContext(ctx, `
		SELECT c.item_code, c.status, COALESCE((SELECT source FROM checklist_item_events ev
			WHERE ev.ficha_id = c.ficha_id AND ev.item_code = c.item_code ORDER BY ev.id DESC LIMIT 1), '')
		FROM checklist_items c WHERE c.ficha_id = ?`, fichaID)
	if err != nil {
		return 0, fmt.Errorf("list checklist items for auto review: %w", err)
	}
	type change struct{ code, from, to, note string }
	changes := []change{}
	for items.Next() {
		var code, status, lastSource string
		if err := items.Scan(&code, &status, &lastSource); err != nil {
			items.Close()
			return 0, fmt.Errorf("scan checklist item for auto review: %w", err)
		}
		switch {
		case status == string(checklist.StatusPending) && approved[code]:
			changes = append(changes, change{code, status, string(checklist.StatusYes), "Todas sus evidencias quedaron aprobadas en Revisión."})
		case status == string(checklist.StatusYes) && lastSource == AutoReviewSource && !approved[code]:
			changes = append(changes, change{code, status, string(checklist.StatusPending), "Una de sus evidencias ya no está aprobada."})
		}
	}
	items.Close()
	if err := items.Err(); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range changes {
		if _, err := transaction.ExecContext(ctx, `UPDATE checklist_items SET status = ?, updated_at = ? WHERE ficha_id = ? AND item_code = ?`, item.to, now, fichaID, item.code); err != nil {
			return 0, fmt.Errorf("update checklist item from review: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO checklist_item_events(ficha_id, item_code, from_status, to_status, source, note, created_at)
			VALUES(?, ?, ?, ?, ?, ?, ?)`, fichaID, item.code, item.from, item.to, AutoReviewSource, item.note, now); err != nil {
			return 0, fmt.Errorf("record checklist auto review event: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return 0, fmt.Errorf("commit checklist auto review: %w", err)
	}
	return len(changes), nil
}

// EvidenceReviewReport returns the saved review state, verifying only the
// evidences that have no review yet (or whose content changed).
func (s *Store) EvidenceReviewReport(ctx context.Context, fichaID string) (evidence.ReviewReport, error) {
	return evidence.VerifyFicha(ctx, s, s.dataDir, fichaID, true, time.Now().UTC())
}

// SetEvidenceReview stores a manual decision. status "pending" clears the
// manual decision and re-verifies the evidence automatically.
func (s *Store) SetEvidenceReview(ctx context.Context, evidenceID, status, note string) (evidence.ReviewEntry, error) {
	status = strings.TrimSpace(status)
	if status != evidence.ReviewApproved && status != evidence.ReviewRejected && status != evidence.ReviewPending {
		return evidence.ReviewEntry{}, errors.New("status debe ser approved, rejected o pending")
	}
	record, err := s.GetEvidence(ctx, evidenceID)
	if err != nil {
		return evidence.ReviewEntry{}, err
	}
	records := []evidence.Record{record}
	if record.FichaID != "" {
		if all, err := s.ListEvidencesByFicha(ctx, record.FichaID, 10000); err == nil {
			records = all
		}
	}
	now := time.Now().UTC()
	// The automatic reasons and dimensions are kept even on a manual decision
	// so the person still sees what the verifier detected. A manual approve or
	// reject reuses the stored analysis when the image is unchanged instead of
	// decoding it again (bulk decisions used to decode every image).
	var review evidence.Review
	if stored, storedErr := s.GetEvidenceReview(ctx, evidenceID); storedErr == nil && status != evidence.ReviewPending && stored.SHA256 == record.SHA256 {
		review = stored
		review.UpdatedAt = now
	} else {
		review = evidence.VerifyRecord(s.dataDir, record, records, now)
	}
	if status != evidence.ReviewPending {
		review.Status = status
		review.Source = evidence.ReviewSourceManual
		review.Note = strings.TrimSpace(note)
	} else if _, err := s.db.ExecContext(ctx, `DELETE FROM evidence_reviews WHERE evidence_id = ? AND source = 'manual'`, evidenceID); err != nil {
		// "pending" is the person withdrawing their decision; the upsert
		// never lets an automatic review replace a manual one by itself.
		return evidence.ReviewEntry{}, fmt.Errorf("clear manual evidence review: %w", err)
	}
	if err := s.UpsertEvidenceReviews(ctx, []evidence.Review{review}); err != nil {
		return evidence.ReviewEntry{}, err
	}
	if record.FichaID != "" {
		if _, err := s.SyncApprovedChecklistItems(ctx, record.FichaID); err != nil {
			return evidence.ReviewEntry{}, err
		}
	}
	return evidence.BuildReviewEntry(record, review, records), nil
}

// CaptureAbsenceReasons reads the absences reported by the recent checklist
// captures of a ficha ("<itemCode>: <detalle>") and keeps, per item, the
// newest plain-words reason. It implements evidence.AbsenceReasonStore.
func (s *Store) CaptureAbsenceReasons(ctx context.Context, fichaID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT input_json, COALESCE(result_json, '') FROM jobs
		WHERE type = 'capture-checklist' AND status IN ('completed', 'failed') ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("list capture jobs for absences: %w", err)
	}
	defer rows.Close()
	reasons := map[string]string{}
	for rows.Next() {
		var inputJSON, resultJSON string
		if err := rows.Scan(&inputJSON, &resultJSON); err != nil {
			return nil, fmt.Errorf("scan capture job: %w", err)
		}
		var input struct {
			FichaID string `json:"fichaId"`
		}
		var result struct {
			Absences []string `json:"absences"`
		}
		if json.Unmarshal([]byte(inputJSON), &input) != nil || input.FichaID != fichaID || resultJSON == "" {
			continue
		}
		if json.Unmarshal([]byte(resultJSON), &result) != nil {
			continue
		}
		for _, absence := range result.Absences {
			code, detail, ok := strings.Cut(absence, ": ")
			code = strings.TrimSpace(code)
			if !ok || code == "" {
				continue
			}
			if _, seen := reasons[code]; seen {
				continue
			}
			reasons[code] = plainAbsenceDetail(detail)
		}
	}
	return reasons, rows.Err()
}

// plainAbsenceDetail drops the technical prefix of an absence and keeps the
// part that says what is missing in Zajuna.
func plainAbsenceDetail(detail string) string {
	for _, marker := range []string{"Zajuna): ", "Zajuna: "} {
		if index := strings.LastIndex(detail, marker); index >= 0 {
			detail = detail[index+len(marker):]
			break
		}
	}
	return strings.TrimSpace(detail)
}
