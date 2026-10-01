package sqlite

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/zajuna"
)

func writeReviewPNG(t *testing.T, path string, width, height int, dark bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	img := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value := uint8(255)
			if dark && x < width/2 {
				value = 0
			}
			img.SetGray(x, y, color.Gray{Y: value})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceReviewsPersistManualDecisionAndResetOnRecapture(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "100", Name: "Ficha", CourseID: "c1"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 1 {
		t.Fatalf("fichas: %#v (%v)", fichas, err)
	}
	fichaID := fichas[0].ID

	blankPath := filepath.Join(dataDir, "evidences", "blank.png")
	writeReviewPNG(t, blankPath, 800, 600, false)
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "ev-1", FichaID: fichaID, ItemCode: "1.1.1", SlotNumber: 1, Name: "Cronograma", FilePath: blankPath, Format: "png", Source: "capture-checklist", SHA256: "sha-1"}); err != nil {
		t.Fatal(err)
	}

	report, err := store.EvidenceReviewReport(ctx, fichaID)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Evidences) != 1 || report.Evidences[0].Status != evidence.ReviewPending || report.Evidences[0].Reasons[0].Code != evidence.ReasonMostlyBlank {
		t.Fatalf("unexpected report %#v", report.Evidences)
	}
	if report.Evidences[0].ItemDescription == "" || report.Evidences[0].Width != 800 {
		t.Fatalf("missing description/dimensions %#v", report.Evidences[0])
	}

	entry, err := store.SetEvidenceReview(ctx, "ev-1", evidence.ReviewApproved, "Revisada")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Status != evidence.ReviewApproved || entry.Source != evidence.ReviewSourceManual || entry.Note != "Revisada" {
		t.Fatalf("unexpected manual entry %#v", entry)
	}
	report, err = store.VerifyEvidenceReviews(ctx, fichaID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Evidences[0].Status != evidence.ReviewApproved || report.Evidences[0].Source != evidence.ReviewSourceManual {
		t.Fatalf("verify must keep manual decision %#v", report.Evidences[0])
	}

	// Recapture of the same slot rewrites evidences.id and sha256.
	goodPath := filepath.Join(dataDir, "evidences", "good.png")
	writeReviewPNG(t, goodPath, 800, 600, true)
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "ev-2", FichaID: fichaID, ItemCode: "1.1.1", SlotNumber: 1, Name: "Cronograma", FilePath: goodPath, Format: "png", Source: "capture-checklist", SHA256: "sha-2"}); err != nil {
		t.Fatalf("recapture must not be blocked by evidence_reviews: %v", err)
	}
	report, err = store.VerifyEvidenceReviews(ctx, fichaID)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Evidences) != 1 || report.Evidences[0].EvidenceID != "ev-2" || report.Evidences[0].Status != evidence.ReviewApproved || report.Evidences[0].Source != evidence.ReviewSourceAuto {
		t.Fatalf("recaptured evidence must be re-verified automatically %#v", report.Evidences)
	}

	// pending clears a manual decision.
	if _, err := store.SetEvidenceReview(ctx, "ev-2", evidence.ReviewRejected, ""); err != nil {
		t.Fatal(err)
	}
	entry, err = store.SetEvidenceReview(ctx, "ev-2", evidence.ReviewPending, "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Source != evidence.ReviewSourceAuto || entry.Status != evidence.ReviewApproved {
		t.Fatalf("pending must return to auto verification %#v", entry)
	}

	// Deleting the evidence cascades to its review.
	if _, err := store.DeleteEvidence(ctx, "ev-2"); err != nil {
		t.Fatal(err)
	}
	reviews, err := store.ListEvidenceReviews(ctx, fichaID)
	if err != nil || len(reviews) != 0 {
		t.Fatalf("expected cascade delete, got %#v (%v)", reviews, err)
	}
}

func TestAutomaticVerificationNeverOverwritesAConcurrentManualDecision(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "100", Name: "Ficha", CourseID: "c1"}}); err != nil {
		t.Fatal(err)
	}
	fichas, _ := store.ListFichas(ctx, 10)
	fichaID := fichas[0].ID
	path := filepath.Join(dataDir, "evidences", "blank.png")
	writeReviewPNG(t, path, 800, 600, false)
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "ev-1", FichaID: fichaID, ItemCode: "1.1.1", SlotNumber: 1, Name: "Cronograma", FilePath: path, Format: "png", Source: "capture-checklist", SHA256: "sha-1"}); err != nil {
		t.Fatal(err)
	}
	// A verification computed its automatic result before the person's
	// decision was saved, and writes it afterwards.
	stale := evidence.Review{EvidenceID: "ev-1", FichaID: fichaID, Status: evidence.ReviewPending, Source: evidence.ReviewSourceAuto, SHA256: "sha-1", Reasons: []evidence.ReviewReason{{Code: evidence.ReasonMostlyBlank}}}
	if _, err := store.SetEvidenceReview(ctx, "ev-1", evidence.ReviewApproved, "Revisada"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEvidenceReviews(ctx, []evidence.Review{stale}); err != nil {
		t.Fatal(err)
	}
	review, err := store.GetEvidenceReview(ctx, "ev-1")
	if err != nil {
		t.Fatal(err)
	}
	if review.Source != evidence.ReviewSourceManual || review.Status != evidence.ReviewApproved {
		t.Fatalf("the manual decision must survive a late automatic write, got %#v", review)
	}
	// A recapture (new sha256) is verified again automatically.
	stale.SHA256 = "sha-2"
	if err := store.UpsertEvidenceReviews(ctx, []evidence.Review{stale}); err != nil {
		t.Fatal(err)
	}
	if review, _ := store.GetEvidenceReview(ctx, "ev-1"); review.Source != evidence.ReviewSourceAuto {
		t.Fatalf("a new file must be reviewed automatically, got %#v", review)
	}
}

func TestCaptureAbsenceReasonsExplainMissingItems(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, row := range []struct{ id, created, result string }{
		{"job-old", "2026-01-01T00:00:00Z", `{"absences":["7.3.2: vieja"]}`},
		{"job-new", "2026-01-02T00:00:00Z", `{"absences":["7.3.2: el selector requerido no apareció en la página destino (sin contenido en Zajuna): la sección no tiene actividades ni archivos","9.1.3: sin contenido válido en Zajuna: el foro no muestra fechas de apertura y cierre"]}`},
		{"job-other", "2026-01-03T00:00:00Z", `{"absences":["9.1.6: otra ficha"]}`},
	} {
		ficha := "ficha-1"
		if row.id == "job-other" {
			ficha = "ficha-2"
		}
		if _, err := store.DB().ExecContext(ctx, `INSERT INTO jobs(id, type, status, input_json, result_json, progress, stage, message, attempt, max_attempts, created_at, updated_at)
			VALUES (?, 'capture-checklist', 'completed', ?, ?, 100, '', '', 1, 3, ?, ?)`, row.id, `{"fichaId":"`+ficha+`"}`, row.result, row.created, row.created); err != nil {
			t.Fatal(err)
		}
	}
	reasons, err := store.CaptureAbsenceReasons(ctx, "ficha-1")
	if err != nil {
		t.Fatal(err)
	}
	if reasons["7.3.2"] != "la sección no tiene actividades ni archivos" || reasons["9.1.3"] != "el foro no muestra fechas de apertura y cierre" {
		t.Fatalf("unexpected reasons: %#v", reasons)
	}
	if _, leaked := reasons["9.1.6"]; leaked {
		t.Fatal("absences of another ficha must not leak")
	}
}

func TestApprovedItemsAreMarkedInTheChecklistAutomatically(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "200", Name: "Ficha", CourseID: "c2"}}); err != nil {
		t.Fatal(err)
	}
	fichas, _ := store.ListFichas(ctx, 10)
	fichaID := fichas[0].ID
	add := func(id, item string) {
		path := filepath.Join(dataDir, "evidences", id+".png")
		writeReviewPNG(t, path, 800, 600, true)
		if err := store.CreateEvidence(ctx, evidence.Record{ID: id, FichaID: fichaID, ItemCode: item, SlotNumber: 1, Name: id, FilePath: path, Format: "png", Source: "capture-checklist", SHA256: "sha-" + id}); err != nil {
			t.Fatal(err)
		}
	}
	add("a", "2.1.1") // approved automatically
	add("b", "4.1")   // approved, but the person set "No" by hand
	add("c", "3.1")   // left pending by hand
	if err := store.SetChecklistItemStatus(ctx, fichaID, "4.1", "NO"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.VerifyEvidenceReviews(ctx, fichaID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetEvidenceReview(ctx, "c", evidence.ReviewRejected, "falta"); err != nil {
		t.Fatal(err)
	}
	status := func(code string) string {
		var value string
		if err := store.DB().QueryRowContext(ctx, `SELECT status FROM checklist_items WHERE ficha_id = ? AND item_code = ?`, fichaID, code).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if status("2.1.1") != "SI" || status("4.1") != "NO" || status("3.1") != "PENDIENTE" {
		t.Fatalf("unexpected statuses 2.1.1=%s 4.1=%s 3.1=%s", status("2.1.1"), status("4.1"), status("3.1"))
	}
	// An automatic "Sí" goes back to pending when its evidence stops being approved.
	if _, err := store.SetEvidenceReview(ctx, "a", evidence.ReviewRejected, "no corresponde"); err != nil {
		t.Fatal(err)
	}
	if status("2.1.1") != "PENDIENTE" {
		t.Fatalf("an automatic Sí must be withdrawn, got %s", status("2.1.1"))
	}
	// A "Sí" set by hand is never withdrawn.
	if err := store.SetChecklistItemStatus(ctx, fichaID, "2.1.1", "SI"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncApprovedChecklistItems(ctx, fichaID); err != nil {
		t.Fatal(err)
	}
	if status("2.1.1") != "SI" {
		t.Fatalf("a manual Sí must be kept, got %s", status("2.1.1"))
	}
}

func TestInstructorUploadCompletesAnItemWithAnEmptySectionCapture(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "300", Name: "Ficha", CourseID: "c3"}}); err != nil {
		t.Fatal(err)
	}
	fichas, _ := store.ListFichas(ctx, 10)
	fichaID := fichas[0].ID
	capturePath := filepath.Join(dataDir, "evidences", "empty.png")
	writeReviewPNG(t, capturePath, 800, 600, true)
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "cap", FichaID: fichaID, ItemCode: "7.3.2", SlotNumber: 1, Name: "Documentos de retención", FilePath: capturePath, Format: "png", Source: "capture-checklist", SHA256: "sha-cap", Metadata: []byte(`{"contentItems":0}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.VerifyEvidenceReviews(ctx, fichaID); err != nil {
		t.Fatal(err)
	}
	status := func() string {
		var value string
		if err := store.DB().QueryRowContext(ctx, `SELECT status FROM checklist_items WHERE ficha_id = ? AND item_code = '7.3.2'`, fichaID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if status() != "PENDIENTE" {
		t.Fatalf("an empty section must keep the item pending, got %s", status())
	}

	manualPath := filepath.Join(dataDir, "evidences", "manual", "acta.pdf")
	if err := os.MkdirAll(filepath.Dir(manualPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manualPath, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "man", FichaID: fichaID, ItemCode: "7.3.2", SlotNumber: 1, Name: "Documento", FilePath: manualPath, Format: "pdf", Source: evidence.EvidenceSourceManual, SHA256: "sha-man"}); err != nil {
		t.Fatal(err)
	}
	report, err := store.ReviewNewEvidenceAndSync(ctx, fichaID)
	if err != nil {
		t.Fatal(err)
	}
	if status() != "SI" {
		t.Fatalf("the instructor's upload must complete the item, got %s", status())
	}
	if report.Summary.ItemsApproved != 1 || report.Summary.ItemsPending != 0 {
		t.Fatalf("summary=%#v", report.Summary)
	}

	// A later automatic recapture (the section is still empty in Zajuna) must
	// not push the instructor's upload out of the 1-evidence item.
	recapturePath := filepath.Join(dataDir, "evidences", "empty-2.png")
	writeReviewPNG(t, recapturePath, 800, 600, true)
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "cap-2", FichaID: fichaID, ItemCode: "7.3.2", SlotNumber: 1, Name: "Documentos de retención", FilePath: recapturePath, Format: "png", Source: "capture-checklist", SHA256: "sha-cap-2", Metadata: []byte(`{"contentItems":0}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetEvidence(ctx, "man"); err != nil {
		t.Fatalf("the instructor's upload was removed by a recapture: %v", err)
	}
	if _, err := store.VerifyEvidenceReviews(ctx, fichaID); err != nil {
		t.Fatal(err)
	}
	if status() != "SI" {
		t.Fatalf("the item must stay completed after a recapture, got %s", status())
	}
}

func TestInstructorUploadSupersedesTheEmptySlotOfAMultiEvidenceItem(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "301", Name: "Ficha", CourseID: "c4"}}); err != nil {
		t.Fatal(err)
	}
	fichas, _ := store.ListFichas(ctx, 10)
	fichaID := fichas[0].ID
	add := func(id string, slot int, source, metadata string) {
		path := filepath.Join(dataDir, "evidences", id+".png")
		writeReviewPNG(t, path, 800, 600, true)
		if err := store.CreateEvidence(ctx, evidence.Record{ID: id, FichaID: fichaID, ItemCode: "12.1.1", SlotNumber: slot, Name: id, FilePath: path, Format: "png", Source: source, SHA256: "sha-" + id, Metadata: []byte(metadata)}); err != nil {
			t.Fatal(err)
		}
	}
	add("week-1", 1, "capture-checklist", `{}`)
	add("week-4", 4, "capture-checklist", `{"contentItems":0}`)
	if _, err := store.VerifyEvidenceReviews(ctx, fichaID); err != nil {
		t.Fatal(err)
	}
	add("week-4-manual", 4, evidence.EvidenceSourceManual, `{}`)
	report, err := store.ReviewNewEvidenceAndSync(ctx, fichaID)
	if err != nil {
		t.Fatal(err)
	}
	superseded := map[string]bool{}
	for _, entry := range report.Evidences {
		superseded[entry.EvidenceID] = entry.Superseded
	}
	if !superseded["week-4"] || superseded["week-1"] || superseded["week-4-manual"] {
		t.Fatalf("superseded = %#v", superseded)
	}
	var status string
	if err := store.DB().QueryRowContext(ctx, `SELECT status FROM checklist_items WHERE ficha_id = ? AND item_code = '12.1.1'`, fichaID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "SI" {
		t.Fatalf("12.1.1 must be completed, got %s", status)
	}
}

func TestAnItemWithAMissingElementIsNeverFulfilled(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "400", Name: "Ficha", CourseID: "c5"}}); err != nil {
		t.Fatal(err)
	}
	fichas, _ := store.ListFichas(ctx, 10)
	fichaID := fichas[0].ID
	path := filepath.Join(dataDir, "evidences", "forum.png")
	writeReviewPNG(t, path, 800, 600, true)
	// 9.1.3: one forum captured with its dates, the other forum has none.
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "f1", FichaID: fichaID, ItemCode: "9.1.3", SlotNumber: 1, Name: "Foro con fechas", FilePath: path, Format: "png", Source: "capture-checklist", SHA256: "sha-f1", Metadata: []byte(`{"semanticCheck":"forum-dates"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCaptureGaps(ctx, fichaID, []string{"9.1.3"}, []string{"9.1.3: sin contenido en Zajuna: el foro no tiene fechas de apertura ni de cierre configuradas"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.VerifyEvidenceReviews(ctx, fichaID); err != nil {
		t.Fatal(err)
	}
	status := func() string {
		var value string
		if err := store.DB().QueryRowContext(ctx, `SELECT status FROM checklist_items WHERE ficha_id = ? AND item_code = '9.1.3'`, fichaID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if status() != "PENDIENTE" {
		t.Fatalf("approved evidence plus a missing forum must stay pending, got %s", status())
	}
	// A later capture of the item finds both forums: fulfilled.
	if err := store.RecordCaptureGaps(ctx, fichaID, []string{"9.1.3"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncApprovedChecklistItems(ctx, fichaID); err != nil {
		t.Fatal(err)
	}
	if status() != "SI" {
		t.Fatalf("without gaps the item is fulfilled, got %s", status())
	}
	// And an automatic «Sí» goes back to pending when a new gap appears.
	if err := store.RecordCaptureGaps(ctx, fichaID, []string{"9.1.3"}, nil, []string{"9.1.3: navegación fallida"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncApprovedChecklistItems(ctx, fichaID); err != nil {
		t.Fatal(err)
	}
	if status() != "PENDIENTE" {
		t.Fatalf("a failed slot leaves the item unverified, got %s", status())
	}
	gaps, _ := store.CaptureGaps(ctx, fichaID)
	if gaps["9.1.3"].Kind != CaptureGapFailed {
		t.Fatalf("gaps = %#v", gaps)
	}
}

func TestItemsWithGapsOrErrorsCannotBeMarkedByHand(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "500", Name: "Ficha", CourseID: "c6"}}); err != nil {
		t.Fatal(err)
	}
	fichas, _ := store.ListFichas(ctx, 10)
	fichaID := fichas[0].ID
	if err := store.RecordCaptureGaps(ctx, fichaID, []string{"9.1.3"}, []string{"9.1.3: sin contenido en Zajuna: el foro no tiene fechas"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SetChecklistItemStatus(ctx, fichaID, "9.1.3", "SI"); !errors.Is(err, ErrItemNotFulfillable) {
		t.Fatalf("a manual «SI» with a gap must be refused, got %v", err)
	}
	if err := store.SetChecklistItemStatus(ctx, fichaID, "9.1.3", "NO"); err != nil {
		t.Fatalf("«No» stays free: %v", err)
	}
	// An evidence that shows #REF! cannot be approved by hand.
	path := filepath.Join(dataDir, "evidences", "sheet.png")
	writeReviewPNG(t, path, 800, 600, true)
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "s1", FichaID: fichaID, ItemCode: "1.2.1", SlotNumber: 1, Name: "Cronograma", FilePath: path, Format: "png", Source: "capture-checklist", SHA256: "sha-s1", Metadata: []byte(`{"sheetIssues":["la columna «Fecha fin fase» del cronograma tiene 1 celda con el error #REF!"]}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.VerifyEvidenceReviews(ctx, fichaID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetEvidenceReview(ctx, "s1", evidence.ReviewApproved, ""); !errors.Is(err, ErrItemNotFulfillable) {
		t.Fatalf("approving an evidence with known errors must be refused, got %v", err)
	}
	if err := store.SetChecklistItemStatus(ctx, fichaID, "1.2.1", "SI"); !errors.Is(err, ErrItemNotFulfillable) {
		t.Fatalf("a manual «SI» over known errors must be refused, got %v", err)
	}
	if err := store.SetChecklistItemStatus(ctx, fichaID, "4.1", "SI"); err != nil {
		t.Fatalf("an item without gaps or errors can still be marked: %v", err)
	}
}
