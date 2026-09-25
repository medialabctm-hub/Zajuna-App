package zajuna

import "testing"

func TestPhaseForSectionUsesTheEnclosingPhase(t *testing.T) {
	phases := []detectedPhase{{Section: 14, Name: "Fase 1 Planear"}, {Section: 41, Name: "Fase 2 Hacer"}}
	bySection := map[int]string{14: "Fase 1 Planear", 41: "Fase 2 Hacer"}
	cases := map[int]string{9: "", 14: "Fase 1 Planear", 19: "Fase 1 Planear", 40: "Fase 1 Planear", 45: "Fase 2 Hacer"}
	for section, want := range cases {
		if got := phaseForSection(phases, bySection, section); got != want {
			t.Fatalf("section %d: got %q, want %q", section, got, want)
		}
	}
}
