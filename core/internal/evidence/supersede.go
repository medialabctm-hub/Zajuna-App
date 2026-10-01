package evidence

import (
	"fmt"
	"strings"
)

// EvidenceSourceManual is the evidence source of a file the instructor uploaded.
const EvidenceSourceManual = "manual"

// SlotEvidence is the minimum needed to decide whether a checklist item is
// fully approved: which item and slot an evidence fills, whether the
// instructor uploaded it and its review status.
type SlotEvidence struct {
	ID       string
	ItemCode string
	Slot     int
	Manual   bool
	Status   string
}

// IsManualSource tells an instructor upload apart from an automatic capture.
func IsManualSource(source string) bool {
	return strings.TrimSpace(source) == EvidenceSourceManual
}

func slotKey(itemCode string, slot int) string {
	if slot < 1 {
		slot = 1
	}
	return fmt.Sprintf("%s\x00%d", itemCode, slot)
}

// SupersededEvidence returns the automatic captures replaced by an approved
// upload of the instructor in the same item and slot. This is how an item the
// app cannot prove by itself (an empty section, a forum without the content
// the checklist asks for) gets completed: the instructor supplies the
// evidence and the capture that showed the gap stops counting. The capture is
// kept; it just no longer blocks the item.
func SupersededEvidence(entries []SlotEvidence) map[string]bool {
	approvedManual := map[string]bool{}
	for _, entry := range entries {
		if entry.ItemCode != "" && entry.Manual && entry.Status == ReviewApproved {
			approvedManual[slotKey(entry.ItemCode, entry.Slot)] = true
		}
	}
	superseded := map[string]bool{}
	if len(approvedManual) == 0 {
		return superseded
	}
	for _, entry := range entries {
		if entry.ItemCode != "" && !entry.Manual && approvedManual[slotKey(entry.ItemCode, entry.Slot)] {
			superseded[entry.ID] = true
		}
	}
	return superseded
}

// ItemApprovals returns, per item with evidence, whether every evidence that
// still counts (see SupersededEvidence) is approved.
func ItemApprovals(entries []SlotEvidence) map[string]bool {
	superseded := SupersededEvidence(entries)
	approved := map[string]bool{}
	for _, entry := range entries {
		if entry.ItemCode == "" || superseded[entry.ID] {
			continue
		}
		ok := entry.Status == ReviewApproved
		if current, seen := approved[entry.ItemCode]; seen {
			approved[entry.ItemCode] = current && ok
		} else {
			approved[entry.ItemCode] = ok
		}
	}
	return approved
}
