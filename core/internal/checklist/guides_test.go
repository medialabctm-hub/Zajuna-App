package checklist

import "testing"

func guideCodes(guides []Guide) map[string]string {
	codes := map[string]string{}
	for _, guide := range guides {
		codes[guide.ItemCode] = guide.Kind
	}
	return codes
}

func TestDetectGuidesFindsItemsOutOfTheAppsReach(t *testing.T) {
	input := GuideInput{
		Items: []GuideItemState{{ItemCode: "9.1.6", Status: "PENDIENTE"}, {ItemCode: "7.3.2", Status: "PENDIENTE"}, {ItemCode: "1.1.1", Status: "PENDIENTE"}, {ItemCode: "4.1", Status: "SI"}},
		Evidences: []GuideEvidence{
			{ItemCode: "7.3.2", Slot: 1, EmptySection: true},
			{ItemCode: "1.1.1", Slot: 1, Approved: true},
			// A technical problem is the app's job: no guide.
			{ItemCode: "3.1", Slot: 1},
		},
		Absences: map[string]string{"9.1.6": "la lista no tiene respuestas del instructor", "1.1.1": "obsoleta"},
	}
	codes := guideCodes(DetectGuides(input))
	if codes["9.1.6"] != GuideContentAbsent || codes["7.3.2"] != GuideEmptySection {
		t.Fatalf("guides = %#v", codes)
	}
	for _, code := range []string{"1.1.1", "4.1", "3.1"} {
		if _, ok := codes[code]; ok {
			t.Fatalf("%s should not have a guide: %#v", code, codes)
		}
	}
	// Without a course map no route is "missing" yet.
	if len(codes) != 2 {
		t.Fatalf("unexpected guides without a map: %#v", codes)
	}
}

func TestDetectGuidesSkipsFulfilledAndSupersededItems(t *testing.T) {
	input := GuideInput{
		Items: []GuideItemState{{ItemCode: "13.1.1", Status: "SI"}, {ItemCode: "13.1.3", Status: "PENDIENTE"}},
		Evidences: []GuideEvidence{
			{ItemCode: "13.1.1", Slot: 1, EmptySection: true},
			{ItemCode: "13.1.3", Slot: 1, EmptySection: true, Superseded: true},
			{ItemCode: "13.1.3", Slot: 1, Approved: true},
		},
	}
	if guides := DetectGuides(input); len(guides) != 0 {
		t.Fatalf("guides = %#v", guideCodes(guides))
	}
}

func TestDetectGuidesReportsMissingRoutesOnlyWithAMap(t *testing.T) {
	input := GuideInput{MapReady: true, Targets: []CaptureTarget{{ItemCode: "9.1.1", CoveredItemCodes: []string{"9.1.1", "9.1.2"}, URL: "https://zajuna.example/mod/forum/view.php?id=1", Name: "Foro de dudas"}}}
	guides := DetectGuides(input)
	codes := guideCodes(guides)
	if _, ok := codes["9.1.2"]; ok {
		t.Fatalf("9.1.2 is covered by a target: %#v", codes)
	}
	if codes["14.1.1"] != GuideRouteMissing {
		t.Fatalf("14.1.1 kind = %q", codes["14.1.1"])
	}
	// Activity-bound items wait for the activity selection, not the instructor.
	for _, code := range []string{"6.1", "10.1.1"} {
		if _, ok := codes[code]; ok {
			t.Fatalf("%s should wait for the activity selection", code)
		}
	}
}

func TestBuildGuideLinksToZajunaAndOffersCompletion(t *testing.T) {
	input := GuideInput{
		Evidences: []GuideEvidence{{ItemCode: "12.1.1", Slot: 1, Approved: true}, {ItemCode: "12.1.1", Slot: 4, EmptySection: true}},
		Targets: []CaptureTarget{
			{ItemCode: "12.1.1", SlotNumber: 1, URL: "https://zajuna.example/course/section.php?id=1", Name: "Semana 1"},
			{ItemCode: "12.1.1", SlotNumber: 4, URL: "https://zajuna.example/course/section.php?id=4", Name: "Semana 4"},
		},
	}
	guides := DetectGuides(input)
	if len(guides) != 1 {
		t.Fatalf("guides = %#v", guideCodes(guides))
	}
	guide := guides[0]
	if guide.ZajunaURL != "https://zajuna.example/course/section.php?id=4" || guide.ZajunaLabel != "Semana 4" {
		t.Fatalf("link = %q %q", guide.ZajunaURL, guide.ZajunaLabel)
	}
	if len(guide.MissingSlots) != 1 || guide.MissingSlots[0] != 4 {
		t.Fatalf("missing slots = %#v", guide.MissingSlots)
	}
	if guide.Headline == "" || guide.Why == "" || guide.Handoff == "" || len(guide.Steps) == 0 || guide.CategoryLabel == "" {
		t.Fatalf("incomplete guide: %#v", guide)
	}
	if len(guide.Actions) != 2 || guide.Actions[0] != GuideActionRecapture || guide.Actions[1] != GuideActionUpload {
		t.Fatalf("actions = %#v", guide.Actions)
	}
}

func TestEveryChecklistItemGetsAGuideOfEveryKind(t *testing.T) {
	for _, item := range Items() {
		for _, kind := range []string{GuideContentAbsent, GuideEmptySection, GuideRouteMissing} {
			guide, ok := BuildGuide(GuideSignal{ItemCode: item.ItemCode, Kind: kind})
			if !ok || guide.Headline == "" || len(guide.Steps) == 0 || guide.Handoff == "" || len(guide.Actions) == 0 {
				t.Fatalf("%s/%s: %#v", item.ItemCode, kind, guide)
			}
		}
	}
	if _, ok := BuildGuide(GuideSignal{ItemCode: "99.9", Kind: GuideContentAbsent}); ok {
		t.Fatal("unknown item must not get a guide")
	}
	if _, ok := BuildGuide(GuideSignal{ItemCode: "1.1.1", Kind: "other"}); ok {
		t.Fatal("unknown kind must not get a guide")
	}
	conclusion, _ := BuildGuide(GuideSignal{ItemCode: "14.1.2", Kind: GuideContentAbsent})
	if conclusion.Template == nil || conclusion.Template.Body == "" {
		t.Fatal("14.1.2 must offer a text template")
	}
}

func TestDetectGuidesTellsAnEmptySectionAbsenceApart(t *testing.T) {
	guides := DetectGuides(GuideInput{Absences: map[string]string{
		"13.1.3": "la sección no tiene actividades ni archivos",
		"14.1.1": "la lista no tiene publicaciones del instructor autenticado sobre «conclusion»",
	}})
	codes := guideCodes(guides)
	if codes["13.1.3"] != GuideEmptySection || codes["14.1.1"] != GuideContentAbsent {
		t.Fatalf("guides = %#v", codes)
	}
	for _, guide := range guides {
		if guide.ItemCode == "14.1.1" && guide.Detected != "Última captura: la lista no tiene publicaciones del instructor autenticado sobre «conclusión»." {
			t.Fatalf("detected = %q", guide.Detected)
		}
	}
}
