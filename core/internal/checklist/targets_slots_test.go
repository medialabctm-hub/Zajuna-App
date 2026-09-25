package checklist

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/coursemaps"
)

func TestBatchesForListDistributesTheRemainder(t *testing.T) {
	cases := []struct {
		maxSlots, lists int
		want            []int
	}{
		{5, 1, []int{5}},
		{5, 2, []int{3, 2}},
		{8, 3, []int{3, 3, 2}},
		{15, 4, []int{4, 4, 4, 3}},
		{3, 3, []int{1, 1, 1}},
		{1, 1, []int{1}},
	}
	for _, tc := range cases {
		total := 0
		for index, want := range tc.want {
			got := batchesForList(tc.maxSlots, tc.lists, index)
			if got != want {
				t.Fatalf("batchesForList(%d, %d, %d) = %d, want %d", tc.maxSlots, tc.lists, index, got, want)
			}
			total += got
		}
		if total != tc.maxSlots {
			t.Fatalf("%d slots over %d lists planned %d", tc.maxSlots, tc.lists, total)
		}
	}
}

func forumRecord(itemCode string, forumIDs []int, restricted map[int]bool) coursemaps.Record {
	urls := make([]string, 0, len(forumIDs))
	routes := make([]coursemaps.Route, 0, len(forumIDs))
	for _, id := range forumIDs {
		url := fmt.Sprintf("https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=%d", id)
		urls = append(urls, url)
		routes = append(routes, coursemaps.Route{URL: url, Kind: "forum", Title: fmt.Sprintf("Anuncios fase %d", id), Restricted: restricted[id]})
	}
	encoded, _ := json.Marshal(urls)
	return coursemaps.Record{ByItemCode: map[string]json.RawMessage{itemCode: encoded}, Routes: routes}
}

func targetsFor(t *testing.T, record coursemaps.Record, itemCode string) []CaptureTarget {
	t.Helper()
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	result := make([]CaptureTarget, 0)
	for _, target := range targets {
		for _, code := range target.CoveredItemCodes {
			if code == itemCode {
				result = append(result, target)
				break
			}
		}
	}
	return result
}

// MDL-221: 11.4 allows 8 evidences; 3 announcement lists used to plan
// 8/3 = 2 batches each (6 slots), leaving slots 7 and 8 unplanned.
func TestRowBatchedListsPlanEveryEvidenceSlot(t *testing.T) {
	targets := targetsFor(t, forumRecord("11.4", []int{101, 102, 103}, nil), "11.4")
	if len(targets) != 8 {
		t.Fatalf("expected 8 slots for 11.4, got %d: %#v", len(targets), targets)
	}
	wantBatches := map[string][]int{
		"https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=101": {0, 1, 2},
		"https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=102": {0, 1, 2},
		"https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=103": {0, 1},
	}
	gotBatches := map[string][]int{}
	for index, target := range targets {
		if target.SlotNumber != index+1 {
			t.Fatalf("slots must stay contiguous: slot %d at position %d", target.SlotNumber, index)
		}
		gotBatches[target.URL] = append(gotBatches[target.URL], target.RowBatch)
	}
	for url, want := range wantBatches {
		if fmt.Sprint(gotBatches[url]) != fmt.Sprint(want) {
			t.Fatalf("batches for %s = %v, want %v", url, gotBatches[url], want)
		}
	}
}

func TestGradingTablesPlanEveryEvidenceSlot(t *testing.T) {
	record := coursemaps.Record{Routes: []coursemaps.Route{
		{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=1", ActivityID: "1", Title: "Informe", PhaseSection: 1, Technical: true},
		{Kind: "grading", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=1&action=grading", ActivityID: "1"},
		{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=2", ActivityID: "2", Title: "Storyboard", PhaseSection: 2, Technical: true},
		{Kind: "grading", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=2&action=grading", ActivityID: "2"},
	}}
	targets, _, err := BuildCaptureTargetsForActivities(record, map[string]bool{"1": true, "2": true})
	if err != nil {
		t.Fatal(err)
	}
	var grading []CaptureTarget
	for _, target := range targets {
		if target.ItemCode == "10.1.1" {
			grading = append(grading, target)
		}
	}
	if len(grading) != 5 {
		t.Fatalf("expected 5 grading slots over 2 activities (3+2), got %d: %#v", len(grading), grading)
	}
	for index, want := range []struct {
		activity string
		batch    int
	}{{"1", 0}, {"1", 1}, {"1", 2}, {"2", 0}, {"2", 1}} {
		if grading[index].ActivityID != want.activity || grading[index].RowBatch != want.batch || grading[index].SlotNumber != index+1 {
			t.Fatalf("grading slot %d = %#v, want activity %s batch %d", index+1, grading[index], want.activity, want.batch)
		}
	}
}

// MDL-219: a forum the crawl proved inaccessible never becomes evidence,
// and its slots go to the remaining lists.
func TestRestrictedForumIsNeverATarget(t *testing.T) {
	targets := targetsFor(t, forumRecord("11.4", []int{101, 102}, map[int]bool{101: true}), "11.4")
	if len(targets) != 8 {
		t.Fatalf("the accessible forum must receive all 8 slots, got %d", len(targets))
	}
	for _, target := range targets {
		if target.URL == "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=101" {
			t.Fatalf("restricted forum was planned: %#v", target)
		}
	}
}

// MDL-218: the grader URL is rewritten to the gradebook setup, which lists
// the grade items vertically: each 5.1 slot is a batch of rows of that list,
// together covering it, never the ~28.000 px wide grader.
func TestGradebookSlotsAreRowBatchesOfTheSetupList(t *testing.T) {
	record := coursemaps.Record{
		ByItemCode: map[string]json.RawMessage{"5.1": json.RawMessage(`["https://zajuna.sena.edu.co/zajuna/grade/report/grader/index.php?id=41080"]`)},
		Routes:     []coursemaps.Route{{Kind: "grading", URL: "https://zajuna.sena.edu.co/zajuna/grade/report/grader/index.php?id=41080"}},
	}
	targets := targetsFor(t, record, "5.1")
	if len(targets) != 5 {
		t.Fatalf("expected 5 gradebook slots, got %d", len(targets))
	}
	for index, target := range targets {
		if !strings.Contains(target.URL, "/grade/edit/tree/") || target.RowBatch != index || target.RowsPerShot != GradebookRowsPerShot || target.MaxCaptureWidth != 0 || target.SlotNumber != index+1 {
			t.Fatalf("gradebook slot %d is not a row batch of the setup list: %#v", index+1, target)
		}
	}
	if captureUnitKey(targets[0]) == captureUnitKey(targets[1]) {
		t.Fatal("row batches must be distinct capture units")
	}
}

func TestGradingTargetsShowOnlyGradedSubmissions(t *testing.T) {
	record := coursemaps.Record{
		ByItemCode: map[string]json.RawMessage{"10.1.1": json.RawMessage(`["https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=301"]`)},
		Routes: []coursemaps.Route{
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=301", ActivityID: "301", Title: "Storyboard", Technical: true},
			{Kind: "grading", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=301&action=grading", ActivityID: "301", Title: "Calificación: Storyboard", Technical: true},
		},
	}
	targets, _, err := BuildCaptureTargetsForActivities(record, map[string]bool{"301": true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, target := range targets {
		if target.ItemCode == "10.1.1" {
			found = true
			if strings.Join(target.RowMatch, "|") != GradedRowTerm {
				t.Fatalf("10.1.1 must keep only graded rows: %#v", target)
			}
		}
	}
	if !found {
		t.Fatal("10.1.1 target was not generated")
	}
}
