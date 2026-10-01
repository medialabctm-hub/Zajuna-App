package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrItemNotFulfillable: an item with a gap or with evidence showing known
// errors cannot be marked fulfilled.
var ErrItemNotFulfillable = errors.New("este ítem no se puede marcar como cumplido")

// CaptureGap is an element of a checklist item that the last capture of that
// item could not verify: a slot without the content the item asks for in
// Zajuna ("absent", the instructor's to fix) or a slot that failed ("failed",
// the app's to retry). While an item has a gap it is not fulfilled, even if
// the evidence it does have is approved: the checklist is correct or not.
type CaptureGap struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	At     string `json:"at"`
}

const (
	CaptureGapAbsent = "absent"
	CaptureGapFailed = "failed"
)

func captureGapsKey(fichaID string) string {
	return "capture_gaps:" + strings.TrimSpace(fichaID)
}

// CaptureGaps returns the gaps of the last capture of each item of a ficha.
func (s *Store) CaptureGaps(ctx context.Context, fichaID string) (map[string]CaptureGap, error) {
	raw, err := s.GetAppSetting(ctx, captureGapsKey(fichaID))
	if err != nil {
		return nil, err
	}
	gaps := map[string]CaptureGap{}
	if strings.TrimSpace(raw) == "" {
		return gaps, nil
	}
	if err := json.Unmarshal([]byte(raw), &gaps); err != nil {
		return map[string]CaptureGap{}, nil
	}
	return gaps, nil
}

// RecordCaptureGaps replaces the gaps of the items a capture verified
// (scope) with what it found: absences and failures as "<itemCode>: detail"
// lines. Items outside the scope keep the gaps of their own last capture.
func (s *Store) RecordCaptureGaps(ctx context.Context, fichaID string, scope []string, absences, failures []string) error {
	gaps, err := s.CaptureGaps(ctx, fichaID)
	if err != nil {
		return err
	}
	for _, code := range scope {
		delete(gaps, strings.TrimSpace(code))
	}
	now := time.Now().UTC().Format(time.RFC3339)
	add := func(kind string, lines []string) {
		for _, line := range lines {
			code, detail, ok := strings.Cut(line, ": ")
			code = strings.TrimSpace(code)
			if !ok || code == "" {
				continue
			}
			// An absence wins over a failure: it tells the instructor what to do.
			if existing, seen := gaps[code]; seen && (existing.Kind == CaptureGapAbsent || kind == CaptureGapFailed) {
				continue
			}
			gaps[code] = CaptureGap{Kind: kind, Detail: plainAbsenceDetail(detail), At: now}
		}
	}
	add(CaptureGapAbsent, absences)
	add(CaptureGapFailed, failures)
	encoded, err := json.Marshal(gaps)
	if err != nil {
		return fmt.Errorf("encode capture gaps: %w", err)
	}
	return s.SetAppSetting(ctx, captureGapsKey(fichaID), string(encoded))
}

// itemBlocker explains why an item cannot be fulfilled: a gap of its last
// capture or an evidence whose review reports content errors (a schedule
// with #REF!).
func (s *Store) itemBlocker(ctx context.Context, fichaID, itemCode string) (string, bool, error) {
	gaps, err := s.CaptureGaps(ctx, fichaID)
	if err != nil {
		return "", false, err
	}
	if gap, ok := gaps[strings.TrimSpace(itemCode)]; ok {
		return "le falta un elemento en Zajuna: " + gap.Detail, true, nil
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM evidences e JOIN evidence_reviews r ON r.evidence_id = e.id
		WHERE e.ficha_id = ? AND e.item_code = ? AND r.reasons_json LIKE '%"sheet_errors"%'`, fichaID, itemCode).Scan(&count); err != nil {
		return "", false, fmt.Errorf("check item content errors: %w", err)
	}
	if count > 0 {
		return "su evidencia muestra errores en el contenido de Zajuna (celdas con error en el cronograma)", true, nil
	}
	return "", false, nil
}
