package evidence

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestPNG writes a width x height PNG; the left `darkFraction` of columns is
// black and the rest white.
func writeTestPNG(t *testing.T, dataDir, name string, width, height int, darkFraction float64) string {
	t.Helper()
	dir := filepath.Join(dataDir, "evidences")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	img := image.NewGray(image.Rect(0, 0, width, height))
	darkCols := int(float64(width) * darkFraction)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if x < darkCols {
				img.SetGray(x, y, color.Gray{Y: 0})
			} else {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func testRecord(id, itemCode, path, sha string, metadata map[string]any) Record {
	raw, _ := json.Marshal(metadata)
	return Record{ID: id, FichaID: "ficha-1", ItemCode: itemCode, SlotNumber: 1, Name: id, FilePath: path, Format: "png", Source: "capture-checklist", SHA256: sha, Metadata: raw}
}

func reasonCodes(review Review) map[string]bool {
	codes := map[string]bool{}
	for _, reason := range review.Reasons {
		codes[reason.Code] = true
	}
	return codes
}

func TestVerifyRecordRules(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Now()
	good := writeTestPNG(t, dataDir, "good.png", 800, 600, 0.5)
	wide := writeTestPNG(t, dataDir, "wide.png", 4100, 300, 0.5)
	tall := writeTestPNG(t, dataDir, "tall.png", 300, 9100, 0.5)
	small := writeTestPNG(t, dataDir, "small.png", 800, 60, 0.5)
	blank := writeTestPNG(t, dataDir, "blank.png", 800, 600, 0.002)
	outside := filepath.Join(t.TempDir(), "outside.png")
	_ = os.WriteFile(outside, []byte("x"), 0o600)

	cases := []struct {
		name       string
		record     Record
		wantCode   string
		wantStatus string
	}{
		{"approved", testRecord("e1", "1.1.1", good, "a", nil), "", ReviewApproved},
		{"file_missing", testRecord("e2", "1.1.1", filepath.Join(dataDir, "evidences", "nope.png"), "b", nil), ReasonFileMissing, ReviewRejected},
		{"outside_data_dir", testRecord("e3", "1.1.1", outside, "c", nil), ReasonFileMissing, ReviewRejected},
		{"login_page", testRecord("e4", "1.1.1", good, "d", map[string]any{"finalUrl": "https://zajuna.sena.edu.co/zajuna/login/index.php"}), ReasonLoginPage, ReviewRejected},
		{"too_wide", testRecord("e5", "1.1.1", wide, "e", nil), ReasonTooWide, ReviewPending},
		{"too_tall", testRecord("e6", "1.1.1", tall, "f", nil), ReasonTooTall, ReviewPending},
		{"too_small", testRecord("e7", "7.3.2", small, "g", map[string]any{"selector": "#region-main .course-content li.section"}), ReasonTooSmall, ReviewPending},
		{"short_card_is_fine", testRecord("e7b", "6.1", small, "g2", map[string]any{"selector": "#region-main .course-content #module-1"}), "", ReviewApproved},
		{"mostly_blank", testRecord("e8", "1.1.1", blank, "h", nil), ReasonMostlyBlank, ReviewPending},
		{"generic_selector", testRecord("e9", "1.1.1", good, "i", map[string]any{"selector": "#region-main", "selectorFallbacks": []string{".generaltable", "#region-main"}}), ReasonGenericSelector, ReviewPending},
		{"generic_selector_expected", testRecord("e10", "1.1.1", good, "j", map[string]any{"selector": "#region-main", "selectorFallbacks": []string{"#region-main"}}), "", ReviewApproved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			review := VerifyRecord(dataDir, tc.record, []Record{tc.record}, now)
			if review.Status != tc.wantStatus {
				t.Fatalf("status = %s, want %s (reasons %#v)", review.Status, tc.wantStatus, review.Reasons)
			}
			codes := reasonCodes(review)
			if tc.wantCode == "" && len(codes) != 0 {
				t.Fatalf("unexpected reasons %#v", review.Reasons)
			}
			if tc.wantCode != "" && !codes[tc.wantCode] {
				t.Fatalf("missing reason %s in %#v", tc.wantCode, review.Reasons)
			}
			if review.Source != ReviewSourceAuto || review.SHA256 != tc.record.SHA256 {
				t.Fatalf("unexpected review metadata %#v", review)
			}
		})
	}
}

func TestVerifyRecordDuplicateContent(t *testing.T) {
	dataDir := t.TempDir()
	path := writeTestPNG(t, dataDir, "shared.png", 800, 600, 0.5)
	covered := map[string]any{"coveredItemCodes": []string{"7.4.2", "7.4.3"}}
	a := testRecord("a", "7.4.2", path, "same", covered)
	b := testRecord("b", "7.4.3", path, "same", covered)
	c := testRecord("c", "9.1.1", path, "same", nil)
	all := []Record{a, b, c}

	reviewA := VerifyRecord(dataDir, a, all, time.Now())
	if !reasonCodes(reviewA)[ReasonDuplicateContent] || reviewA.Reasons[0].Message != "Es idéntica a la evidencia del ítem 9.1.1." {
		t.Fatalf("expected duplicate with 9.1.1, got %#v", reviewA.Reasons)
	}
	entry := BuildReviewEntry(a, reviewA, all)
	if len(entry.SharedWith) != 2 || entry.SharedWith[0] != "7.4.3" || entry.SharedWith[1] != "9.1.1" {
		t.Fatalf("unexpected sharedWith %#v", entry.SharedWith)
	}
	// Without the unrelated item, covered siblings are not duplicates.
	reviewB := VerifyRecord(dataDir, b, []Record{a, b}, time.Now())
	if reviewB.Status != ReviewApproved {
		t.Fatalf("covered siblings must not be duplicates: %#v", reviewB.Reasons)
	}
}

func TestAnalyzeImageSkipsBlankForHugeImages(t *testing.T) {
	// DecodeConfig only reads the header; a huge declared size must not decode.
	dataDir := t.TempDir()
	path := writeTestPNG(t, dataDir, "ok.png", 300, 200, 0)
	stats, err := AnalyzeImage(path)
	if err != nil || stats.Width != 300 || stats.Height != 200 || stats.BlankRatio < 0.99 {
		t.Fatalf("unexpected stats %#v (%v)", stats, err)
	}
	if reasons := imageReasons(ImageStats{Width: 3000, Height: 28000, BlankRatio: -1}); len(reasons) != 1 || reasons[0].Code != ReasonTooTall {
		t.Fatalf("unexpected reasons for skipped analysis %#v", reasons)
	}
}

type memoryReviewStore struct {
	records []Record
	reviews map[string]Review
	upserts int
}

func (m *memoryReviewStore) ListEvidencesByFicha(context.Context, string, int) ([]Record, error) {
	return m.records, nil
}

func (m *memoryReviewStore) ListEvidenceReviews(context.Context, string) ([]Review, error) {
	result := make([]Review, 0, len(m.reviews))
	for _, review := range m.reviews {
		result = append(result, review)
	}
	return result, nil
}

func (m *memoryReviewStore) UpsertEvidenceReviews(_ context.Context, reviews []Review) error {
	for _, review := range reviews {
		m.reviews[review.EvidenceID] = review
		m.upserts++
	}
	return nil
}

func TestVerifyFichaRespectsManualDecisionUntilShaChanges(t *testing.T) {
	dataDir := t.TempDir()
	blank := writeTestPNG(t, dataDir, "blank.png", 800, 600, 0)
	store := &memoryReviewStore{records: []Record{testRecord("e1", "1.1.1", blank, "sha-1", nil)}, reviews: map[string]Review{}}
	ctx := context.Background()

	report, err := VerifyFicha(ctx, store, dataDir, "ficha-1", false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Pending != 1 || report.Evidences[0].Status != ReviewPending {
		t.Fatalf("expected pending blank evidence: %#v", report.Summary)
	}
	if report.Summary.ItemsPending != 1 || report.Summary.ItemsMissing != len(report.MissingItems) || report.Summary.ItemsMissing == 0 {
		t.Fatalf("unexpected item summary %#v", report.Summary)
	}

	manual := store.reviews["e1"]
	manual.Status, manual.Source, manual.Note = ReviewApproved, ReviewSourceManual, "ok"
	store.reviews["e1"] = manual
	report, err = VerifyFicha(ctx, store, dataDir, "ficha-1", false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Evidences[0].Status != ReviewApproved || report.Evidences[0].Source != ReviewSourceManual || report.Summary.ItemsApproved != 1 {
		t.Fatalf("manual decision must be kept: %#v", report.Evidences[0])
	}

	store.records[0].SHA256 = "sha-2"
	report, err = VerifyFicha(ctx, store, dataDir, "ficha-1", false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Evidences[0].Status != ReviewPending || report.Evidences[0].Source != ReviewSourceAuto {
		t.Fatalf("recaptured evidence must be re-verified: %#v", report.Evidences[0])
	}

	// onlyUnreviewed reuses saved rows without analysing again.
	before := store.upserts
	if _, err := VerifyFicha(ctx, store, dataDir, "ficha-1", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if store.upserts != before {
		t.Fatalf("GET-style verification must not rewrite reviewed rows")
	}
}

func TestVerifyFlagsSectionsWithoutContent(t *testing.T) {
	dataDir := t.TempDir()
	good := writeTestPNG(t, dataDir, "section.png", 900, 600, 0.3)
	empty := VerifyRecord(dataDir, testRecord("s1", "13.1.2", good, "a", map[string]any{"selector": "#region-main .course-content li.section", "contentItems": 0}), nil, time.Now())
	if empty.Status != ReviewPending || empty.Reasons[0].Code != ReasonEmptySection {
		t.Fatalf("a section without activities must be pending: %#v", empty)
	}
	full := VerifyRecord(dataDir, testRecord("s2", "7.4.2", good, "b", map[string]any{"selector": "#region-main .course-content li.section", "contentItems": 2}), nil, time.Now())
	if full.Status != ReviewApproved {
		t.Fatalf("a section with content must be approved: %#v", full)
	}
}

func TestVerifyRecordSameForumWithForceviewIsNotADuplicate(t *testing.T) {
	dataDir := t.TempDir()
	path := writeTestPNG(t, dataDir, "announcements.png", 800, 600, 0.5)
	selector := "#region-main table.discussion-list, #region-main table.forumheaderlist"
	// 11.4 reached the announcements forum with forceview=1 and 15.1 without
	// it: the same page, so the same rows are expected, not a capture error.
	format := testRecord("format", "11.4", path, "same", map[string]any{"selector": selector, "url": "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?forceview=1&id=3010173"})
	netiqueta := testRecord("netiqueta", "15.1", path, "same", map[string]any{"selector": selector, "url": "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=3010173"})
	all := []Record{format, netiqueta}
	for _, record := range all {
		if review := VerifyRecord(dataDir, record, all, time.Now()); review.Status != ReviewApproved {
			t.Fatalf("%s must be approved, got %#v", record.ItemCode, review.Reasons)
		}
	}
	// A different forum with identical bytes is still reported.
	other := testRecord("other", "9.1.1", path, "same", map[string]any{"selector": selector, "url": "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=99"})
	if review := VerifyRecord(dataDir, other, append(all, other), time.Now()); !reasonCodes(review)[ReasonDuplicateContent] {
		t.Fatalf("a different page with the same image must be a duplicate: %#v", review.Reasons)
	}
}

func TestVerifyRecordFlagsEvidenceFromAnOlderSemanticRule(t *testing.T) {
	dataDir := t.TempDir()
	path := writeTestPNG(t, dataDir, "forum.png", 800, 600, 0.5)
	// Captured before replies were required: an instructor discussion with
	// no replies was approved as proof that the instructor answers.
	old := testRecord("old", "9.1.6", path, "old", map[string]any{"selector": "#region-main table.discussion-list"})
	review := VerifyRecord(dataDir, old, []Record{old}, time.Now())
	if review.Status != ReviewPending || !reasonCodes(review)[ReasonOutdatedRule] {
		t.Fatalf("evidence from an older rule must be pending: %#v", review)
	}
	current := testRecord("current", "9.1.6", path, "current", map[string]any{"selector": "#region-main table.discussion-list", "semanticCheck": "forum-replies"})
	if review := VerifyRecord(dataDir, current, []Record{current}, time.Now()); review.Status != ReviewApproved {
		t.Fatalf("evidence captured with the current rule must be approved: %#v", review.Reasons)
	}
	// Items without a semantic rule and manual uploads are unaffected.
	plain := testRecord("plain", "6.1", path, "plain", nil)
	manual := testRecord("manual", "14.1.1", path, "manual", nil)
	manual.Source = "manual-upload"
	for _, record := range []Record{plain, manual} {
		if review := VerifyRecord(dataDir, record, []Record{record}, time.Now()); reasonCodes(review)[ReasonOutdatedRule] {
			t.Fatalf("%s must not be flagged as outdated", record.ID)
		}
	}
}

// An unrendered Google Sheet: a header on top and a tall empty band below.
// Text with many short gaps must not be flagged.
func TestTallShotWithOneEmptyBandIsMostlyBlank(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, paint func(y int) bool) string {
		img := image.NewNRGBA(image.Rect(0, 0, 800, 3000))
		for y := 0; y < 3000; y++ {
			for x := 0; x < 800; x++ {
				if paint(y) && x < 400 {
					img.Set(x, y, color.Black)
				} else {
					img.Set(x, y, color.White)
				}
			}
		}
		path := filepath.Join(dir, name)
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, img); err != nil {
			t.Fatal(err)
		}
		return path
	}
	blank, err := AnalyzeImage(write("blank.png", func(y int) bool { return y < 200 && y%20 < 10 }))
	if err != nil {
		t.Fatal(err)
	}
	text, err := AnalyzeImage(write("text.png", func(y int) bool { return y%60 < 20 }))
	if err != nil {
		t.Fatal(err)
	}
	if !hasReason(imageReasons(blank), ReasonMostlyBlank) {
		t.Fatalf("an empty band over most of a tall shot must be flagged: %#v", blank)
	}
	if hasReason(imageReasons(text), ReasonMostlyBlank) {
		t.Fatalf("spaced text is not blank: %#v", text)
	}
}

func hasReason(reasons []ReviewReason, code string) bool {
	for _, reason := range reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}
