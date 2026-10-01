package checklist

import (
	"strings"
	"testing"
)

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
		if guide.ItemCode == "14.1.1" && guide.Detected != "Última verificación: la lista no tiene publicaciones del instructor autenticado sobre «conclusión»." {
			t.Fatalf("detected = %q", guide.Detected)
		}
	}
}

// The items whose content only the instructor can create in Zajuna. The eight
// evidenced in ficha 3135429 are mandatory; the rest share their nature.
var instructorDependentItems = []string{
	"7.3.2", "7.3.3", "13.1.1", "13.1.3", "9.1.6", "9.1.7", "14.1.1", "14.1.2",
	"7.3.1", "7.4.1", "7.4.2", "7.4.3", "7.4.4", "13.1.2", "13.2.1", "13.2.2",
	"9.1.1", "9.1.2", "9.1.3", "9.1.4", "9.1.5", "10.1.1", "10.1.2", "12.1.1", "12.1.2",
	"11.1.1", "11.1.2", "11.1.3", "11.1.4", "11.2.1", "11.2.2", "11.2.3", "11.3", "11.4",
}

func TestInstructorDependentItemsHaveCompleteGuides(t *testing.T) {
	catalog := map[string]bool{}
	for _, item := range Items() {
		catalog[item.ItemCode] = true
	}
	for code := range itemGuides {
		if !catalog[code] {
			t.Errorf("guide for %s, which is not a checklist item", code)
		}
	}
	for _, code := range instructorDependentItems {
		content, ok := itemGuides[code]
		if !ok {
			t.Errorf("%s needs a specific guide", code)
			continue
		}
		if content.headline == "" || content.requirement == "" || content.location == "" || content.create == "" || content.evidenceHint == "" || len(content.steps) == 0 {
			t.Errorf("%s: incomplete guide %#v", code, content)
		}
		for _, kind := range []string{GuideContentAbsent, GuideEmptySection, GuideRouteMissing} {
			guide, ok := BuildGuide(GuideSignal{ItemCode: code, Kind: kind})
			if !ok || len(guide.Steps) == 0 || guide.Location == "" || guide.Requirement == "" || guide.EvidenceHint == "" {
				t.Errorf("%s/%s: %#v", code, kind, guide)
				continue
			}
			// The handoff ends where Zajuna Sync takes over: verify or hand the evidence.
			if !strings.Contains(guide.Handoff, "«Ya lo hice, verificar»") || !strings.Contains(guide.Handoff, "«Subir mi evidencia»") || guide.EvidenceHint != content.evidenceHint {
				t.Errorf("%s/%s handoff does not lead to Zajuna Sync: %q", code, kind, guide.Handoff)
			}
		}
		missing, _ := BuildGuide(GuideSignal{ItemCode: code, Kind: GuideRouteMissing})
		if !strings.Contains(missing.Handoff, "«Buscar rutas de nuevo»") || !strings.Contains(strings.Join(missing.Steps, " "), content.create) {
			t.Errorf("%s: a missing route must ask to create %q and search the routes again", code, content.create)
		}
	}
	for _, code := range []string{"9.1.5", "9.1.6", "9.1.7", "14.1.1", "14.1.2", "11.2.3", "12.1.2"} {
		if itemGuides[code].template == nil {
			t.Errorf("%s must offer a text template", code)
		}
	}
	empty, _ := BuildGuide(GuideSignal{ItemCode: "7.3.2", Kind: GuideEmptySection})
	if !strings.Contains(strings.Join(empty.Steps, " "), "solo tiene su título") {
		t.Errorf("7.3.2 empty-section steps = %#v", empty.Steps)
	}
}

func TestItemsTheAppCompletesFallBackToAValidGuide(t *testing.T) {
	for _, code := range []string{"1.1.1", "2.1.4", "4.1", "8.2", "15.1"} {
		if _, ok := itemGuides[code]; ok {
			t.Errorf("%s is completed by the app; it should use the fallback", code)
		}
		guide, ok := BuildGuide(GuideSignal{ItemCode: code, Kind: GuideContentAbsent})
		if !ok || guide.Requirement == "" || guide.EvidenceHint == "" || len(guide.Steps) == 0 || guide.Handoff == "" {
			t.Errorf("%s fallback: %#v", code, guide)
		}
	}
}

func TestDetectGuidesAsksToFixAScheduleWithErrors(t *testing.T) {
	guides := DetectGuides(GuideInput{Evidences: []GuideEvidence{{ItemCode: "1.2.1", Slot: 1, ContentError: "la hoja del cronograma tiene 1 celda con el error #¡REF!"}}})
	if len(guides) != 1 || guides[0].Kind != GuideContentError {
		t.Fatalf("guides = %#v", guideCodes(guides))
	}
	guide := guides[0]
	if !strings.Contains(guide.Detected, "#¡REF!") || !strings.Contains(strings.Join(guide.Steps, " "), "Google Sheets") || guide.EvidenceHint == "" {
		t.Fatalf("content-error guide = %#v", guide)
	}
}

func TestScheduleErrorsAreAdviceNotPendingGuides(t *testing.T) {
	input := GuideInput{
		Items:     []GuideItemState{{ItemCode: "1.2.1", Status: "SI"}, {ItemCode: "1.2.2", Status: "SI"}},
		Evidences: []GuideEvidence{{ItemCode: "1.2.1", Slot: 1, Approved: true, ContentError: "la hoja del cronograma tiene 1 celda con el error #REF!"}, {ItemCode: "1.2.2", Slot: 1, Approved: true, ContentError: "la hoja del cronograma tiene 1 celda con el error #REF!"}},
	}
	if guides := DetectGuides(input); len(guides) != 0 {
		t.Fatalf("fulfilled schedules are not pending guides: %#v", guideCodes(guides))
	}
	advice := DetectAdvice(input)
	if len(advice) != 1 || !advice[0].Advisory || advice[0].ItemCode != "1.2.1" || strings.Join(advice[0].AlsoItems, ",") != "1.2.2" {
		t.Fatalf("advice = %#v", advice)
	}
}

func TestScheduleErrorsShareOneGuide(t *testing.T) {
	evidences := []GuideEvidence{}
	for _, code := range []string{"1.2.1", "1.2.2", "1.2.5"} {
		evidences = append(evidences, GuideEvidence{ItemCode: code, Slot: 1, ContentError: "la hoja del cronograma tiene 1 celda con el error #REF!"})
	}
	evidences = append(evidences, GuideEvidence{ItemCode: "1.2.1", Slot: 3, ContentError: "la hoja del cronograma tiene 2 celdas con el error #N/A"})
	targets := []CaptureTarget{
		{ItemCode: "1.2.1", SlotNumber: 1, URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=1", Name: "Planear"},
		{ItemCode: "1.2.1", SlotNumber: 3, URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=3", Name: "Verificar"},
	}
	guides := DetectGuides(GuideInput{Evidences: evidences, Targets: targets})
	if len(guides) != 1 || guides[0].ItemCode != "1.2.1" || strings.Join(guides[0].AlsoItems, ",") != "1.2.2,1.2.5" {
		t.Fatalf("guides = %#v", guides)
	}
	if !strings.Contains(guides[0].Detected, "#REF!") || !strings.Contains(guides[0].Detected, "#N/A") || len(guides[0].MissingSlots) != 2 {
		t.Fatalf("every faulty sheet must be listed: %#v", guides[0])
	}
}
