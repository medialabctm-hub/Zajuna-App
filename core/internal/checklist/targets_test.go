package checklist

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/coursemaps"
)

func TestBuildCaptureTargetsUsesItemCodesAndEvidenceSlots(t *testing.T) {
	record := coursemaps.Record{
		ByItemCode: map[string]json.RawMessage{
			"2.1.1": json.RawMessage(`"https://zajuna.sena.edu.co/zajuna/user/profile.php"`),
			"1.1.1": json.RawMessage(`[
                "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10",
                "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=11"
            ]`),
			"1.2.1": json.RawMessage(`[
                "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080&section=1",
                "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080&section=2"
            ]`),
		},
		Routes: []coursemaps.Route{
			{URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10", Kind: "page"},
			{URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=11", Kind: "page"},
			{URL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080&section=1", Kind: "phase"},
			{URL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080&section=2", Kind: "phase"},
		},
	}
	targets, summary, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(CaptureSpecs()) != 62 || summary.ItemCount != 62 {
		t.Fatalf("expected 62 checklist specs, summary=%#v", summary)
	}
	if summary.ResolvedItems != 3 || summary.SlotCount != 4 || summary.UnresolvedItems != 59 {
		t.Fatalf("unexpected target summary: %#v", summary)
	}
	if len(targets) != 4 || targets[0].ItemCode != "1.1.1" || targets[0].SlotNumber != 1 || targets[0].CSSSelector == "" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
	if targets[2].ItemCode != "1.2.1" || targets[2].RouteKind != "phase" {
		t.Fatalf("phase route was not projected: %#v", targets[2])
	}
}

func TestBuildCaptureTargetsSharesOneUnitAcrossCompatibleScheduleItems(t *testing.T) {
	record := coursemaps.Record{
		ByItemCode: map[string]json.RawMessage{
			"1.1.1": json.RawMessage(`"https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10"`),
			"1.1.2": json.RawMessage(`"https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10"`),
		},
		Routes: []coursemaps.Route{{Kind: "page", URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10"}},
	}
	targets, summary, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || summary.CaptureUnitCount != 1 || summary.CoverageCount != 2 {
		t.Fatalf("expected one physical unit with two logical criteria, targets=%#v summary=%#v", targets, summary)
	}
	if len(targets[0].CoveredItemCodes) != 2 || targets[0].CoveredItemCodes[0] != "1.1.1" || targets[0].CoveredItemCodes[1] != "1.1.2" {
		t.Fatalf("unexpected coverage metadata: %#v", targets[0].CoveredItemCodes)
	}
}

func TestBuildCaptureTargetsBindsDatesToSelectedActivity(t *testing.T) {
	record := coursemaps.Record{
		CourseURL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080",
		Routes: []coursemaps.Route{
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=3010294", ActivityID: "3010294", Title: "Informe técnico", PhaseSection: 19, Technical: true},
			{Kind: "grading", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=3010294&action=grading", ActivityID: "3010294", Title: "Calificación: Informe técnico", PhaseSection: 19, Technical: true},
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=3010361", ActivityID: "3010361", Title: "Storyboard", PhaseSection: 29, Technical: true},
		},
	}
	targets, _, err := BuildCaptureTargetsForActivities(record, map[string]bool{"3010294": true})
	if err != nil {
		t.Fatal(err)
	}
	var bound, grading []CaptureTarget
	for _, target := range targets {
		if target.ItemCode == "6.1" {
			bound = append(bound, target)
		}
		if target.ItemCode == "10.1.1" || target.ItemCode == "10.1.2" {
			grading = append(grading, target)
		}
	}
	if len(bound) != 1 {
		t.Fatalf("expected one selected activity for 6.1, got %#v", bound)
	}
	// 10.1.x use the activity's grading table (not the 6.1 date card), in
	// 2-row batches, shared by both items: 5 slots, each covering both.
	if len(grading) != 5 {
		t.Fatalf("expected 5 row batches of the grading table, got %#v", grading)
	}
	for index, target := range grading {
		if target.URL != "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=3010294&action=grading" || target.RowBatch != index || target.RowsPerShot != 2 || target.SlotNumber != index+1 {
			t.Fatalf("grading batch %d is wrong: %#v", index, target)
		}
		if len(target.CoveredItemCodes) != 2 || target.CoveredItemCodes[0] != "10.1.1" || target.CoveredItemCodes[1] != "10.1.2" {
			t.Fatalf("grading table must be one shared unit for 10.1.1 and 10.1.2: %#v", target.CoveredItemCodes)
		}
	}
	for _, target := range bound {
		if target.ActivityID != "3010294" || target.URL != record.CourseURL || target.PhaseSection != 19 {
			t.Fatalf("target is not bound to the selected course activity: %#v", target)
		}
		if target.CSSSelector != "#region-main .course-content #module-3010294" || len(target.RevealSelectors) != 1 {
			t.Fatalf("target selector is not activity-specific: %#v", target)
		}
	}
}

func TestBuildCaptureTargetsScopesForumsToTechnicalOwnerContent(t *testing.T) {
	record := coursemaps.Record{
		ByItemCode: map[string]json.RawMessage{
			"9.1.6": json.RawMessage(`["https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=77&forceview=1", "https://zajuna.sena.edu.co/zajuna/mod/forum/discuss.php?d=88"]`),
		},
		Routes: []coursemaps.Route{
			{Kind: "forum", URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=77", Title: "Foro temático GA2-250201022-AA1-EV01"},
			{Kind: "forum", URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/discuss.php?d=88", Title: "Respuesta de otro usuario"},
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=301", ActivityID: "301", Title: "Storyboard GA2-250201022-AA1-EV01", Technical: true},
		},
	}
	targets, _, err := BuildCaptureTargetsForActivities(record, map[string]bool{"301": true})
	if err != nil {
		t.Fatal(err)
	}
	var forums []CaptureTarget
	for _, target := range targets {
		if target.ItemCode == "9.1.6" {
			forums = append(forums, target)
		}
	}
	// Only the real forum route is used; its discussion list is captured in
	// contiguous 2-row batches (rows 1–2, 3–4, …) up to the item limit.
	if len(forums) != 5 {
		t.Fatalf("expected 5 row batches of the only real forum route, got %#v", forums)
	}
	for index, target := range forums {
		if !strings.Contains(target.URL, "forum/view.php?id=77") || target.SlotNumber != index+1 || target.RowBatch != index || target.RowsPerShot != RowsPerShot || target.RowSelector == "" {
			t.Fatalf("forum batch %d is wrong: %#v", index, target)
		}
		if !target.OwnerOnly || !target.RequireSelector {
			t.Fatalf("forum target must require authenticated owner filtering: %#v", target)
		}
	}
}

func TestGradingItemsNeverReuseTheDateCardWithoutGradingRoute(t *testing.T) {
	record := coursemaps.Record{
		CourseURL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080",
		Routes: []coursemaps.Route{
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=3010294", ActivityID: "3010294", Title: "Informe técnico", PhaseSection: 19, Technical: true},
		},
	}
	targets, summary, err := BuildCaptureTargetsForActivities(record, map[string]bool{"3010294": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		for _, code := range target.CoveredItemCodes {
			if code == "10.1.1" || code == "10.1.2" {
				t.Fatalf("10.1.x must not duplicate the 6.1 date card: %#v", target)
			}
		}
	}
	if summary.UnresolvedItems == 0 {
		t.Fatal("10.1.x without a grading route must be reported as unresolved")
	}
}

func TestCourseSectionGroupsNeverTargetTheFirstSection(t *testing.T) {
	for _, group := range []string{"seguimiento_evaluacion", "seguimiento_documentos", "documentos_retencion", "sesiones_linea"} {
		if selector := captureGroupPlan(group).selector; !strings.Contains(selector, ":has-text(") && !strings.Contains(selector, ":text-matches(") {
			t.Fatalf("%s must scope .section to its named section, got %q", group, selector)
		}
	}
}

func TestForumConfigurationUsesFullForumRegionWithoutOwnerRequirement(t *testing.T) {
	record := coursemaps.Record{ByItemCode: map[string]json.RawMessage{
		"9.1.3": json.RawMessage(`"https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=77"`),
	}}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.ItemCode == "9.1.3" {
			// The full forum region, but only when it shows the configured
			// dates: a forum without them must not become evidence.
			if target.OwnerOnly || target.CSSSelector != ForumDatesSelector || !target.RequireSelector || len(target.HideSelectors) != 1 || target.HideSelectors[0] != "#region-main table" {
				t.Fatalf("forum configuration should use a strict full-region capture: %#v", target)
			}
			if len(target.CSSSelectorFallbacks) != 1 || target.CSSSelectorFallbacks[0] != ForumDatesSelector {
				t.Fatalf("forum configuration must not fall back to a region without dates: %#v", target.CSSSelectorFallbacks)
			}
			if target.SemanticCheck != SemanticForumDates {
				t.Fatalf("forum configuration must record its semantic rule, got %q", target.SemanticCheck)
			}
			return
		}
	}
	t.Fatal("forum configuration target was not generated")
}

func TestBuildCaptureTargetsUsesGoogleSheetsAwareCronogramaSelector(t *testing.T) {
	record := coursemaps.Record{ByItemCode: map[string]json.RawMessage{
		"1.1.1": json.RawMessage(`"https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10"`),
	}}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.ItemCode == "1.1.1" {
			if !strings.Contains(target.CSSSelector, "docs.google.com/spreadsheets") || target.ViewportWidth != 2560 || target.ViewportHeight != 1200 || target.FullPage {
				// On an activity page the content region is captured (not the
				// full page with Zajuna header/menu/footer).
				t.Fatalf("cronograma selector does not prioritize embedded Google Sheets: %q", target.CSSSelector)
			}
			return
		}
	}
	t.Fatal("cronograma target was not generated")
}

func TestBuildCaptureTargetsItem31MatchesCourseContentWithoutAFragileHint(t *testing.T) {
	// MDL-124: two independent real courses proved the checklist wording
	// ("material de trabajo", "evidencias") never appears verbatim on the
	// course main page item 3.1 resolves to, so `.section` plus that hint
	// found 284 unrelated nodes and matched none of them, aborting the whole
	// capture batch. See docs/mdl-33-2026-08-26.md.
	record := coursemaps.Record{ByItemCode: map[string]json.RawMessage{
		"3.1": json.RawMessage(`"https://zajuna.sena.edu.co/zajuna/course/view.php?id=27932"`),
	}}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.ItemCode == "3.1" {
			if target.CSSSelector != "#region-main .course-content" {
				t.Fatalf("item 3.1 must crop the confirmed course-content wrapper, got %q", target.CSSSelector)
			}
			if target.LabelHint != "" || !target.RequireSelector {
				t.Fatalf("item 3.1 must be strict without a fragile text hint: %#v", target)
			}
			return
		}
	}
	t.Fatal("item 3.1 target was not generated")
}

func TestBuildCaptureTargetsItem41MenuCursoMatchesCourseContentWithoutAFragileHint(t *testing.T) {
	// MDL-124: a live run against a real course
	// (docs/evidence/mdl-124-verify-2026-09-14.json) proved item 4.1's
	// "secciones" hint never appears inside `#region-main .course-content` on
	// its resolved course main page, even though that exact container matched
	// on that exact route for item 3.1 with no hint at all. Requiring the
	// literal checklist wording only pushed 4.1 down its fallback chain to the
	// coarser `#page-content` wrapper instead of the confirmed container.
	record := coursemaps.Record{ByItemCode: map[string]json.RawMessage{
		"4.1": json.RawMessage(`"https://zajuna.sena.edu.co/zajuna/course/view.php?id=27932"`),
	}}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.ItemCode == "4.1" {
			if target.CSSSelector != "#region-main .course-content" {
				t.Fatalf("item 4.1 must crop the confirmed course-content wrapper, got %q", target.CSSSelector)
			}
			if target.LabelHint != "" || !target.RequireSelector {
				t.Fatalf("item 4.1 must be strict without a fragile text hint: %#v", target)
			}
			return
		}
	}
	t.Fatal("item 4.1 target was not generated")
}

func TestBuildCaptureTargetsSeguimientoSesionesDocumentosDropFragileHints(t *testing.T) {
	// MDL-124 follow-up: a live run against a real course
	// (docs/mdl-124-seguimiento-2026-09-11.md) proved these ten checklist
	// descriptions never appear as literal page text either — same failure
	// mode as item 3.1, `.course-content .section` matched 284 nodes and 0
	// matched the hint, hard-aborting the item instead of falling back.
	itemCodes := []string{"7.1.1", "7.2", "7.3.2", "7.4.1", "7.4.2", "7.4.3", "7.4.4", "8.2", "8.3", "13.1.3", "13.2.2"}
	byItemCode := make(map[string]json.RawMessage, len(itemCodes))
	for _, itemCode := range itemCodes {
		byItemCode[itemCode] = json.RawMessage(`"https://zajuna.sena.edu.co/zajuna/course/view.php?id=27932"`)
	}
	targets, _, err := BuildCaptureTargets(coursemaps.Record{ByItemCode: byItemCode})
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool, len(itemCodes))
	for _, target := range targets {
		if target.LabelHint != "" || !target.RequireSelector {
			t.Fatalf("%s must be strict without a fragile text hint: %#v", target.ItemCode, target)
		}
		found[target.ItemCode] = true
	}
	for _, itemCode := range itemCodes {
		if !found[itemCode] {
			t.Fatalf("%s target was not generated", itemCode)
		}
	}
}

func TestBuildCaptureTargetsCapturesInstructorProfileAsFullPage(t *testing.T) {
	record := coursemaps.Record{ByItemCode: map[string]json.RawMessage{
		"2.1.1": json.RawMessage(`"https://zajuna.sena.edu.co/zajuna/user/profile.php"`),
	}}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.ItemCode == "2.1.1" {
			if !target.FullPage || !target.RequireSelector || target.CSSSelector != "#page-user-profile" {
				t.Fatalf("profile target must use a strict full-page capture: %#v", target)
			}
			return
		}
	}
	t.Fatal("profile target was not generated")
}

func TestApplyRouteReviewsPersistsDecisionAndManualOverrides(t *testing.T) {
	targets := []CaptureTarget{{GroupName: "perfil_instructor", RouteKind: "page", URL: "https://zajuna.sena.edu.co/zajuna/user/profile.php", CSSSelector: "#page-user-profile", CSSSelectorFallbacks: []string{"#page-user-profile"}}}
	key := RouteKey(targets[0])
	updated := ApplyRouteReviews(targets, []RouteReview{{RouteKey: key, Status: RouteReviewCorrection, ManualURL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080", ManualSelector: "#region-main .course-content"}})
	if len(updated) != 1 || updated[0].RouteKey != key || updated[0].ReviewStatus != RouteReviewCorrection {
		t.Fatalf("route review was not projected: %#v", updated)
	}
	if updated[0].URL == targets[0].URL || updated[0].CSSSelector != "#region-main .course-content" || len(updated[0].CSSSelectorFallbacks) != 1 {
		t.Fatalf("manual route override was not applied: %#v", updated[0])
	}
}

func TestTransversalActivitiesNeverProduceEvidence(t *testing.T) {
	record := coursemaps.Record{
		CourseURL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080",
		Routes: []coursemaps.Route{
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=1", ActivityID: "1", Title: "Técnica", PhaseSection: 3, Technical: true},
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=2", ActivityID: "2", Title: "Ética", PhaseSection: 2, Technical: false},
		},
	}
	// A selection saved by an older version may still include transversal ones.
	targets, _, err := BuildCaptureTargetsForActivities(record, map[string]bool{"1": true, "2": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.ActivityID == "2" {
			t.Fatalf("transversal activity leaked into the plan: %#v", target)
		}
	}
	if ActivityEvidenceSlots() != 5 {
		t.Fatalf("6.1 admits 5 evidences, got %d", ActivityEvidenceSlots())
	}
}

func TestAnnouncementItemsFilterRowsByTheirOwnTopic(t *testing.T) {
	forum := `["https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=77&forceview=1"]`
	record := coursemaps.Record{
		ByItemCode: map[string]json.RawMessage{"11.1.1": json.RawMessage(forum), "11.2.1": json.RawMessage(forum), "11.2.2": json.RawMessage(forum), "11.2.3": json.RawMessage(forum), "11.3": json.RawMessage(forum)},
		Routes:     []coursemaps.Route{{Kind: "forum", URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=77", Title: "Anuncios"}},
	}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	topics := map[string]string{}
	for _, target := range targets {
		for _, code := range target.CoveredItemCodes {
			topics[code] = strings.Join(target.RowMatch, "|")
		}
	}
	for code, want := range map[string]string{"11.2.1": "inicio de actividad", "11.2.2": "cierre de actividad", "11.3": "aprobados"} {
		if topics[code] != want {
			t.Fatalf("%s must filter by %q, got %q", code, want, topics[code])
		}
	}
	if !strings.Contains(topics["11.2.3"], "invitacion a sesion") || !strings.Contains(topics["11.1.1"], "apertura de fase") {
		t.Fatalf("unexpected topics: %#v", topics)
	}
	if topics["11.2.1"] == topics["11.2.2"] {
		t.Fatal("different announcement items must not share the same rows")
	}
}

func TestSeguimientoItemsTargetTheirOwnSubsection(t *testing.T) {
	want := map[string]string{"7.3.1": "Comités evaluativos", "7.3.3": "Reuniones EEF", "7.4.2": "Planes de Mejoramiento", "7.4.3": "Registro de Novedades", "7.4.4": "Llamados de atenci"}
	seen := map[string]string{}
	for item, title := range want {
		if got := courseSectionTitleForItem(item); got != title {
			t.Fatalf("%s must target %q, got %q", item, title, got)
		}
		if other, dup := seen[title]; dup {
			t.Fatalf("%s and %s must not share a subsection", item, other)
		}
		seen[title] = item
	}
	if chain := captureSelectorChainForItem("7.4.3", "seguimiento_documentos", courseSectionByTitle("Registro de Novedades")); !strings.Contains(chain[1], comitesSectionTitle) {
		t.Fatalf("7.4.x must fall back to Comités evaluativos, got %v", chain)
	}
}

func TestSiteNewsForumIsNeverCourseEvidence(t *testing.T) {
	record := coursemaps.Record{
		ByItemCode: map[string]json.RawMessage{"11.4": json.RawMessage(`["https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=3010173&forceview=1","https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=26790&forceview=1"]`)},
		Routes: []coursemaps.Route{
			{Kind: "forum", URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=3010173", Title: "ANUNCIOS"},
			{Kind: "forum", URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=26790", Title: "Anuncios de la página"},
		},
	}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if strings.Contains(target.URL, "id=26790") {
			t.Fatalf("the site news forum must not be captured: %#v", target)
		}
	}
}

func TestTransversalOnlySelectionNeverFallsBackToEveryActivity(t *testing.T) {
	record := coursemaps.Record{
		CourseURL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080",
		ByItemCode: map[string]json.RawMessage{
			"6.1":    json.RawMessage(`["https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=1"]`),
			"10.1.1": json.RawMessage(`["https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=1"]`),
		},
		Routes: []coursemaps.Route{
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=1", ActivityID: "1", Title: "Técnica", PhaseSection: 3, Technical: true},
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=2", ActivityID: "2", Title: "Ética", PhaseSection: 2, Technical: false},
		},
	}
	targets, _, err := BuildCaptureTargetsForActivities(record, map[string]bool{"2": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		for _, code := range target.CoveredItemCodes {
			if code == "6.1" || code == "10.1.1" || code == "10.1.2" {
				t.Fatalf("a transversal-only selection must not capture activity items: %#v", target)
			}
		}
	}
	if len(TechnicalSelectionForRecord(record, map[string]bool{"2": true})) != 0 {
		t.Fatal("transversal-only selection must count as no selection")
	}
}

func TestSectionTitlesAreAnchoredAtTheStart(t *testing.T) {
	selector := topLevelCourseSectionByTitle(seguimientoSectionTitle)
	if !strings.Contains(selector, `text-matches("^`) || !strings.Contains(selector, "Seguimiento y Evaluaci") || !strings.Contains(selector, ":not(li.section li.section)") {
		t.Fatalf("7.1.x must match a top-level section whose name starts with the title, got %s", selector)
	}
}

func TestForumReplyAndConclusionItemsEnforceTheirContentRule(t *testing.T) {
	forum := `["https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=88"]`
	record := coursemaps.Record{
		ByItemCode: map[string]json.RawMessage{"9.1.5": json.RawMessage(forum), "9.1.6": json.RawMessage(forum), "9.1.7": json.RawMessage(forum), "14.1.1": json.RawMessage(forum), "14.1.2": json.RawMessage(forum)},
		Routes:     []coursemaps.Route{{Kind: "forum", URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=88", Title: "Foro Temático"}},
	}
	targets, _, err := BuildCaptureTargets(record)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]CaptureTarget{}
	for _, target := range targets {
		for _, code := range target.CoveredItemCodes {
			if _, ok := seen[code]; !ok {
				seen[code] = target
			}
		}
	}
	for _, code := range []string{"9.1.5", "9.1.6", "9.1.7"} {
		target, ok := seen[code]
		if !ok {
			t.Fatalf("%s target was not generated", code)
		}
		// An instructor discussion without replies does not prove that the
		// instructor answers.
		if !target.RowRequireReply || !target.OwnerOnly || target.SemanticCheck != SemanticForumReplies {
			t.Fatalf("%s must require instructor replies: %#v", code, target)
		}
	}
	for _, code := range []string{"14.1.1", "14.1.2"} {
		target, ok := seen[code]
		if !ok {
			t.Fatalf("%s target was not generated", code)
		}
		if target.RowRequireReply || strings.Join(target.RowMatch, "|") != "conclusion" || target.SemanticCheck != SemanticForumConclusion {
			t.Fatalf("%s must require a conclusion discussion: %#v", code, target)
		}
	}
	if captureUnitKey(seen["9.1.6"]) == captureUnitKey(seen["14.1.1"]) {
		t.Fatal("replies and conclusion items must not share one capture")
	}
}

func TestSemanticCheckForItem(t *testing.T) {
	for code, want := range map[string]string{
		"9.1.3": SemanticForumDates, "9.1.4": SemanticForumDates,
		"9.1.5": SemanticForumReplies, "9.1.6": SemanticForumReplies, "9.1.7": SemanticForumReplies,
		"14.1.1": SemanticForumConclusion, "14.1.2": SemanticForumConclusion,
		"9.1.1": "", "11.4": "", "6.1": "",
	} {
		if got := SemanticCheckForItem(code); got != want {
			t.Fatalf("SemanticCheckForItem(%s) = %q, want %q", code, got, want)
		}
	}
}

func TestRouteIndexKeepsFirstCanonicalMatch(t *testing.T) {
	record := coursemaps.Record{Routes: []coursemaps.Route{
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=7&forceview=1", Kind: "forum", Title: "primera"},
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=7", Kind: "forum", Title: "segunda"},
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10", Kind: "page"},
	}}
	index := newRouteIndex(record)
	if route := index.lookup("https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=7"); route == nil || route.Title != "primera" {
		t.Fatalf("expected the first canonical match, got %#v", route)
	}
	if route := index.lookup(" https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10 "); route == nil || route.Kind != "page" {
		t.Fatalf("expected the page route, got %#v", route)
	}
	if route := index.lookup("https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=99"); route != nil {
		t.Fatalf("expected no route, got %#v", route)
	}
}

func TestTransversalOnlySelectionDoesNotEnableEveryActivityForum(t *testing.T) {
	coded := "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=50"
	named := "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=51"
	record := coursemaps.Record{
		CourseURL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=9",
		ByItemCode: map[string]json.RawMessage{
			"9.1.6": json.RawMessage(`["` + coded + `","` + named + `"]`),
		},
		Routes: []coursemaps.Route{
			{Kind: "assign", URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=2", ActivityID: "2", Title: "Ética", Technical: false},
			{Kind: "forum", URL: coded, Title: "Foro temático. GA1-220501046-AA2-EV01"},
			{Kind: "forum", URL: named, Title: "Foro Temático"},
		},
	}
	// The saved selection only holds a transversal activity: after dropping
	// it nothing is selected, so an activity-bound forum must not qualify.
	targets, summary, err := BuildCaptureTargetsForActivities(record, map[string]bool{"2": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.URL == coded {
			t.Fatalf("a forum bound to an activity leaked without a technical selection: %#v", target)
		}
	}
	found := false
	for _, target := range targets {
		found = found || target.URL == named
	}
	if !found {
		t.Fatal("a generic named forum stays eligible")
	}
	// 6.1 has no technical activity to show: it counts as unresolved.
	if summary.ResolvedItems+summary.UnresolvedItems != summary.ItemCount {
		t.Fatalf("every item must be counted once: %+v", summary)
	}
}

func TestDistributeSlotBatchesUsesRemainder(t *testing.T) {
	got := distributeSlotBatches(8, 3)
	if len(got) != 3 || got[0] != 3 || got[1] != 3 || got[2] != 2 {
		t.Fatalf("8 slots among 3 lists: got %v, want 3,3,2", got)
	}
	even := distributeSlotBatches(6, 3)
	if len(even) != 3 || even[0] != 2 || even[1] != 2 || even[2] != 2 {
		t.Fatalf("even split: got %v", even)
	}
}
