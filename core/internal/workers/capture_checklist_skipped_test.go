package workers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/checklist"
)

func TestTallyTargetOutcomesDoesNotCountSkippedAsFailure(t *testing.T) {
	tally := tallyTargetOutcomes([]targetOutcome{
		{captured: true, evidenceRecords: 2},
		{skipped: true},
		{skipped: true},
		{failure: "6.1: timeout"},
	})
	if tally.captured != 1 || tally.skipped != 2 || tally.failed != 1 || tally.evidenceRecords != 2 {
		t.Fatalf("unexpected tally %+v", tally)
	}
	if !reflect.DeepEqual(tally.failures, []string{"6.1: timeout"}) {
		t.Fatalf("failures = %v", tally.failures)
	}
	onlySkipped := tallyTargetOutcomes([]targetOutcome{{captured: true}, {skipped: true}})
	if onlySkipped.failed != 0 || len(onlySkipped.failures) != 0 {
		t.Fatalf("skipped outcomes must not produce failures: %+v", onlySkipped)
	}
}

func TestCaptureChecklistPrunePlanKeepsCapturedFailedAndUnexecutedSlots(t *testing.T) {
	planned := []checklist.CaptureTarget{
		{ItemCode: "6.1", SlotNumber: 1},
		{ItemCode: "6.1", SlotNumber: 2},
		{ItemCode: "6.1", SlotNumber: 3},
		{ItemCode: "7.1", CoveredItemCodes: []string{"7.1", "7.2"}, SlotNumber: 1},
		{ItemCode: "7.2", SlotNumber: 2}, // filtered out of this run
	}
	executed := planned[:4]
	outcomes := []targetOutcome{
		{captured: true},          // 6.1#1 fresh
		{failure: "6.1: timeout"}, // 6.1#2 keeps previous evidence
		{skipped: true},           // 6.1#3 empty batch -> prune
		{captured: true},          // 7.1/7.2#1
	}
	itemCodes, keep := captureChecklistPrunePlan(planned, executed, outcomes)
	if !reflect.DeepEqual(itemCodes, []string{"6.1", "7.1", "7.2"}) {
		t.Fatalf("itemCodes = %v", itemCodes)
	}
	want := map[string]map[int]bool{
		"6.1": {1: true, 2: true},
		"7.1": {1: true},
		"7.2": {1: true, 2: true},
	}
	if !reflect.DeepEqual(keep, want) {
		t.Fatalf("keep = %v, want %v", keep, want)
	}
}

func TestAbsentZajunaContentIsNotACaptureFailure(t *testing.T) {
	tally := tallyTargetOutcomes([]targetOutcome{
		{captured: true, evidenceRecords: 1},
		{failure: "9.1.6: el selector requerido no apareció en la página destino: la lista no tiene publicaciones del instructor autenticado"},
		{failure: "10.1.1: el selector requerido no apareció en la página destino: #region-main table.generaltable (candidatos=0)"},
		{failure: "7.2: navegar para captura: timeout"},
	})
	if tally.absent != 2 || tally.failed != 1 || len(tally.absences) != 2 {
		t.Fatalf("unexpected tally: %#v", tally)
	}
}

func TestCaptureChecklistPrunePlanDropsEvidenceOnlyForTypedAbsences(t *testing.T) {
	// 9.1.6 slot 3 was captured by an older rule (an instructor discussion
	// without replies). The new rule loads the list and finds no answered
	// discussion (a typed absence): the old evidence is retired.
	planned := []checklist.CaptureTarget{
		{ItemCode: "9.1.6", SlotNumber: 1},
		{ItemCode: "9.1.6", SlotNumber: 3},
		{ItemCode: "10.1.1", SlotNumber: 1},
	}
	outcomes := []targetOutcome{
		{captured: true},
		{absent: true, failure: "9.1.6: la lista no tiene respuestas del instructor autenticado"},
		// Recognised as an absence only by its message, which an error or
		// permission page also produces: reported, but the evidence stays.
		{failure: "10.1.1: el selector requerido no apareció en la página destino: #region-main table.generaltable (candidatos=0)"},
	}
	_, keep := captureChecklistPrunePlan(planned, planned, outcomes)
	if !reflect.DeepEqual(keep, map[string]map[int]bool{"9.1.6": {1: true}, "10.1.1": {1: true}}) {
		t.Fatalf("keep = %v", keep)
	}
	if tally := tallyTargetOutcomes(outcomes); tally.absent != 2 || tally.failed != 0 {
		t.Fatalf("both absences are reported, got %+v", tally)
	}
}

func TestAbsenceMessage(t *testing.T) {
	absent := fmt.Errorf("%w (%w): la página cargó pero no muestra el foro", capture.ErrSelectorNotFound, capture.ErrContentAbsent)
	dates := checklist.CaptureTarget{ItemCode: "9.1.3", SemanticCheck: checklist.SemanticForumDates}
	if message := absenceMessage(dates, absent); !strings.Contains(message, "no muestra fechas") {
		t.Fatalf("a forum without dates is explained in plain words, got %q", message)
	}
	if message := absenceMessage(checklist.CaptureTarget{ItemCode: "9.1.6"}, absent); message != absent.Error() {
		t.Fatalf("other absences keep the capture message, got %q", message)
	}
	if !errors.Is(absent, capture.ErrSelectorNotFound) || !errors.Is(absent, capture.ErrContentAbsent) {
		t.Fatal("an absence is still a selector miss for callers that only know that error")
	}
}

func TestLockFichaCaptureWaitIsCancellable(t *testing.T) {
	unlock, err := lockFichaCapture(context.Background(), "ficha-lock-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lockFichaCapture(ctx, "ficha-lock-test"); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled job must stop waiting for the ficha lock, got %v", err)
	}
	unlock()
	again, err := lockFichaCapture(context.Background(), "ficha-lock-test")
	if err != nil {
		t.Fatalf("the lock must be free after unlock: %v", err)
	}
	again()
}

func TestAbsenceIsReportedForEveryCoveredItem(t *testing.T) {
	tally := tallyTargetOutcomes([]targetOutcome{{absent: true, failure: "9.1.6: sin réplicas", coveredItemCodes: []string{"9.1.6", "9.1.7"}}})
	if tally.absent != 1 || !reflect.DeepEqual(tally.absences, []string{"9.1.6: sin réplicas", "9.1.7: sin réplicas"}) {
		t.Fatalf("every covered item must carry the absence: %#v", tally)
	}
}
