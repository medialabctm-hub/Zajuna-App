package evidence

import "testing"

func TestApprovedManualUploadSupersedesTheCaptureOfItsSlot(t *testing.T) {
	entries := []SlotEvidence{
		{ID: "capture-1", ItemCode: "7.3.2", Slot: 1, Status: ReviewPending},
		{ID: "manual-1", ItemCode: "7.3.2", Slot: 1, Manual: true, Status: ReviewApproved},
		{ID: "capture-2", ItemCode: "12.1.1", Slot: 4, Status: ReviewPending},
		{ID: "manual-2", ItemCode: "12.1.1", Slot: 3, Manual: true, Status: ReviewApproved},
		{ID: "capture-3", ItemCode: "13.1.1", Slot: 1, Status: ReviewPending},
		{ID: "manual-3", ItemCode: "13.1.1", Slot: 1, Manual: true, Status: ReviewRejected},
	}
	superseded := SupersededEvidence(entries)
	if !superseded["capture-1"] || superseded["capture-2"] || superseded["capture-3"] || len(superseded) != 1 {
		t.Fatalf("superseded = %#v", superseded)
	}
	approved := ItemApprovals(entries)
	if !approved["7.3.2"] {
		t.Fatal("7.3.2 is completed by the instructor's upload")
	}
	if approved["12.1.1"] {
		t.Fatal("an upload in another slot does not replace slot 4")
	}
	if approved["13.1.1"] {
		t.Fatal("a rejected upload replaces nothing")
	}
}
