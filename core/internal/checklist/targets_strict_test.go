package checklist

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/coursemaps"
)

const strictCourseURL = "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080"

// fullStrictRecord maps every checklist item to a plausible route so the
// whole target plan is generated at once.
func fullStrictRecord() (coursemaps.Record, map[string]bool) {
	forum := "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=77"
	page := "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10"
	byURL := map[string]string{
		"1.1.": page, "1.2.": strictCourseURL + "&section=2", "2.1.": "https://zajuna.sena.edu.co/zajuna/user/profile.php",
		"5.1": "https://zajuna.sena.edu.co/zajuna/grade/report/grader/index.php?id=41080",
		"9.":  forum, "11.": forum, "14.": forum, "15.1": forum,
		"12.": strictCourseURL,
	}
	byItemCode := map[string]json.RawMessage{}
	for _, spec := range CaptureSpecs() {
		target := strictCourseURL
		for prefix, value := range byURL {
			if spec.ItemCode == prefix || strings.HasPrefix(spec.ItemCode, prefix) {
				target = value
			}
		}
		encoded, _ := json.Marshal([]string{target})
		byItemCode[spec.ItemCode] = encoded
	}
	record := coursemaps.Record{
		CourseURL:  strictCourseURL,
		ByItemCode: byItemCode,
		Routes: []coursemaps.Route{
			{Kind: "page", URL: page, Title: "Cronograma General"},
			{Kind: "phase", URL: strictCourseURL + "&section=2", Title: "FASE 2 HACER"},
			{Kind: "forum", URL: forum, Title: "Foro temático GA2-250201022-AA1-EV01"},
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=301", ActivityID: "301", Title: "Storyboard", Technical: true},
			{Kind: "grading", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=301&action=grading", ActivityID: "301", Title: "Calificación: Storyboard", Technical: true},
		},
	}
	return record, map[string]bool{"301": true}
}

func TestEveryChecklistTargetRequiresItsSemanticSelector(t *testing.T) {
	record, selected := fullStrictRecord()
	targets, _, err := BuildCaptureTargetsForActivities(record, selected)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) < 40 {
		t.Fatalf("expected the full plan, got %d targets", len(targets))
	}
	for _, target := range targets {
		if !target.RequireSelector {
			t.Fatalf("%s slot %d must never fall back to a generic full-page shot: %#v", target.ItemCode, target.SlotNumber, target)
		}
		for _, selector := range append([]string{target.CSSSelector}, target.CSSSelectorFallbacks...) {
			if strings.TrimSpace(selector) == "#page-content" {
				t.Fatalf("%s uses the whole-layout wrapper as fallback: %#v", target.ItemCode, target.CSSSelectorFallbacks)
			}
			if selector == "#page-user-profile" && target.GroupName != "perfil_instructor" {
				t.Fatalf("%s borrows the profile container: %#v", target.ItemCode, target.CSSSelectorFallbacks)
			}
		}
	}
}

func TestSectionBoundItemsFallBackOnlyToTheirParentSection(t *testing.T) {
	record := coursemaps.Record{ByItemCode: map[string]json.RawMessage{
		"7.3.1": json.RawMessage(`"` + strictCourseURL + `"`),
		"7.1.1": json.RawMessage(`"` + strictCourseURL + `"`),
	}}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		switch target.ItemCode {
		case "7.3.1":
			// Its own subsection (see TestSeguimientoItemsTargetTheirOwnSubsection)
			// and then only the parent section.
			want := []string{courseSectionByTitle(courseSectionTitleForItem("7.3.1")), courseSectionByTitle(seguimientoSectionTitle)}
			if !reflect.DeepEqual(target.CSSSelectorFallbacks, want) {
				t.Fatalf("7.3.1 must fall back only to its parent section, got %#v", target.CSSSelectorFallbacks)
			}
		case "7.1.1":
			// Its own section is matched top-level and anchored at the start
			// of the name (see TestSectionTitlesAreAnchoredAtTheStart).
			want := []string{topLevelCourseSectionByTitle(seguimientoSectionTitle)}
			if !reflect.DeepEqual(target.CSSSelectorFallbacks, want) {
				t.Fatalf("7.1.1 must only use its own section, got %#v", target.CSSSelectorFallbacks)
			}
		}
	}
}

func TestSplitSlotsKeepsTheRemainder(t *testing.T) {
	for _, tc := range []struct {
		max, sources int
		want         []int
	}{
		{5, 1, []int{5}},
		{5, 2, []int{3, 2}},
		{5, 3, []int{2, 2, 1}},
		{5, 5, []int{1, 1, 1, 1, 1}},
		{1, 1, []int{1}},
	} {
		if got := splitSlots(tc.max, tc.sources); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("splitSlots(%d, %d) = %v, want %v", tc.max, tc.sources, got, tc.want)
		}
	}
	if splitSlots(5, 0) != nil {
		t.Fatal("no sources means no slots")
	}
}

func TestRowBatchesSpreadTheRemainderOverSeveralRoutes(t *testing.T) {
	forums := []string{
		"https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=77",
		"https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=78",
	}
	encoded, _ := json.Marshal(forums)
	record := coursemaps.Record{
		ByItemCode: map[string]json.RawMessage{"11.1.1": encoded},
		Routes: []coursemaps.Route{
			{Kind: "forum", URL: forums[0], Title: "Anuncios fase 1"},
			{Kind: "forum", URL: forums[1], Title: "Anuncios fase 2"},
		},
	}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	maxSlots := captureSpecFor("11.1.1").MaxSlots
	var slots []CaptureTarget
	for _, target := range targets {
		if target.ItemCode == "11.1.1" {
			slots = append(slots, target)
		}
	}
	if len(slots) != maxSlots {
		t.Fatalf("expected all %d slots (remainder included), got %d: %#v", maxSlots, len(slots), slots)
	}
	perURL := map[string][]int{}
	for index, target := range slots {
		if target.SlotNumber != index+1 {
			t.Fatalf("slots must stay contiguous: %#v", slots)
		}
		perURL[target.URL] = append(perURL[target.URL], target.RowBatch)
	}
	split := splitSlots(maxSlots, 2)
	for index, forum := range forums {
		batches := perURL[forum]
		if len(batches) != split[index] {
			t.Fatalf("%s got batches %v, want %d", forum, batches, split[index])
		}
		for batch, value := range batches {
			if value != batch {
				t.Fatalf("%s batches must start at 0 and be consecutive: %v", forum, batches)
			}
		}
	}
}

func TestGradingBatchesKeepTheRemainderAcrossActivities(t *testing.T) {
	record := coursemaps.Record{
		CourseURL: strictCourseURL,
		Routes: []coursemaps.Route{
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=1", ActivityID: "1", Title: "A", Technical: true},
			{Kind: "grading", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=1&action=grading", ActivityID: "1", Technical: true},
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=2", ActivityID: "2", Title: "B", Technical: true},
			{Kind: "grading", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=2&action=grading", ActivityID: "2", Technical: true},
		},
	}
	targets, _, err := BuildCaptureTargetsForActivities(record, map[string]bool{"1": true, "2": true})
	if err != nil {
		t.Fatal(err)
	}
	maxSlots := captureSpecFor("10.1.1").MaxSlots
	count := 0
	for _, target := range targets {
		if target.ItemCode == "10.1.1" {
			count++
			if target.SlotNumber != count || !target.RequireSelector {
				t.Fatalf("grading slot %d is wrong: %#v", count, target)
			}
		}
	}
	if count != maxSlots {
		t.Fatalf("expected %d grading slots over two activities, got %d", maxSlots, count)
	}
}
