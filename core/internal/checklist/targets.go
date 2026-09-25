package checklist

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/zajuna-app/core/internal/coursemaps"
)

// CaptureSpec describes the deterministic capture contract for one checklist item.
// The URL is resolved from the course map; selectors and label hints are applied
// by the local Chromium capture worker. Group rules are the first local mapping
// pass and remain versionable as Zajuna course structures evolve.
type CaptureSpec struct {
	ItemCode    string
	GroupName   string
	Name        string
	MaxSlots    int
	RouteKinds  []string
	CSSSelector string
	LabelHints  []string
}

type CaptureTarget struct {
	ItemCode             string   `json:"itemCode"`
	CoveredItemCodes     []string `json:"coveredItemCodes,omitempty"`
	GroupName            string   `json:"groupName"`
	Name                 string   `json:"name"`
	URL                  string   `json:"url"`
	RouteKey             string   `json:"routeKey,omitempty"`
	ReviewStatus         string   `json:"reviewStatus,omitempty"`
	SlotNumber           int      `json:"slotNumber"`
	ActivityID           string   `json:"activityId,omitempty"`
	ActivityTitle        string   `json:"activityTitle,omitempty"`
	PhaseSection         int      `json:"phaseSection,omitempty"`
	Technical            bool     `json:"technical,omitempty"`
	CSSSelector          string   `json:"cssSelector"`
	CSSSelectorFallbacks []string `json:"cssSelectorFallbacks,omitempty"`
	RevealSelectors      []string `json:"revealSelectors,omitempty"`
	HideSelectors        []string `json:"hideSelectors,omitempty"`
	ViewportWidth        int      `json:"viewportWidth,omitempty"`
	ViewportHeight       int      `json:"viewportHeight,omitempty"`
	FullPage             bool     `json:"fullPage,omitempty"`
	LabelHint            string   `json:"labelHint,omitempty"`
	RouteKind            string   `json:"routeKind,omitempty"`
	RequireSelector      bool     `json:"requireSelector,omitempty"`
	OwnerOnly            bool     `json:"ownerOnly,omitempty"`
	// Row batching for list/table evidence: slot N shows only rows
	// [RowBatch*RowsPerShot, RowBatch*RowsPerShot+RowsPerShot) of the rows
	// matched by RowSelector inside the captured container (the header stays
	// visible). A batch past the last row yields no evidence.
	RowSelector string `json:"rowSelector,omitempty"`
	RowsPerShot int    `json:"rowsPerShot,omitempty"`
	RowBatch    int    `json:"rowBatch,omitempty"`
	// OptionalSlot: the slot may not exist on this course (no evidence, not
	// a failure). Used for per-phase course sections.
	OptionalSlot bool `json:"optionalSlot,omitempty"`
	// RowMatch keeps only list rows whose text contains one of these terms
	// (accent/case-insensitive), so each announcement item shows its own
	// announcements instead of the same generic rows as every other item.
	RowMatch []string `json:"rowMatch,omitempty"`
	// RowRequireReply keeps only discussions with at least one reply whose
	// last message is by the instructor: the proof that the instructor
	// answered, instead of a discussion the instructor merely opened.
	RowRequireReply bool `json:"rowRequireReply,omitempty"`
	// SemanticCheck names the content rule this capture enforced. It is
	// persisted with the evidence so the automatic review can tell apart
	// evidence captured with an older, weaker rule (see SemanticCheckForItem).
	SemanticCheck string `json:"semanticCheck,omitempty"`
	// CourseLayout is the state the course sections are put in before a
	// whole-course capture ("menu" or "first-section"), so the evidence does
	// not depend on what the instructor left open in Zajuna.
	CourseLayout string `json:"courseLayout,omitempty"`
	// AbsenceSelector proves the right page loaded when the evidence
	// selector did not match, so the slot is a real absence (see
	// capture.ErrContentAbsent) and not a failure.
	AbsenceSelector string `json:"absenceSelector,omitempty"`
	// MaxCaptureWidth bounds the shot width: a wider element is split into
	// windows of whole columns and ColumnBatch (zero-based) picks one. A
	// window past the last column yields no evidence.
	MaxCaptureWidth int `json:"maxCaptureWidth,omitempty"`
	ColumnBatch     int `json:"columnBatch,omitempty"`
}

// courseLayoutForGroup: 4.1 proves the menu (every section closed) and 3.1
// the material and evidence links (the first section open with its whole
// subtree). Other course captures crop one section and open only that one.
func courseLayoutForGroup(groupName string) string {
	switch groupName {
	case "menu_curso":
		return "menu"
	case "disponibilidad":
		return "first-section"
	}
	return ""
}

// Semantic content rules enforced at capture time.
const (
	// SemanticForumReplies: discussions answered by the instructor.
	SemanticForumReplies = "forum-replies"
	// SemanticForumConclusion: an instructor discussion titled as conclusion.
	SemanticForumConclusion = "forum-conclusion"
	// SemanticForumDates: a forum page that shows its opening/closing dates.
	SemanticForumDates = "forum-dates"
)

// SemanticCheckForItem returns the content rule an item's evidence must have
// been captured with, or "" when the route and selector already identify it.
func SemanticCheckForItem(itemCode string) string {
	switch itemCode {
	case "9.1.5", "9.1.6", "9.1.7":
		return SemanticForumReplies
	case "14.1.1", "14.1.2":
		return SemanticForumConclusion
	case "9.1.3", "9.1.4":
		return SemanticForumDates
	}
	return ""
}

// forumPageSelector recognises a rendered forum view (its discussion list
// or search form), not a Moodle error or permission page.
const forumPageSelector = `#page-mod-forum-view #region-main form[action*="/mod/forum/search.php"], #page-mod-forum-view #region-main [data-region="discussion-list-container"], #page-mod-forum-view #region-main table.discussion-list`

// ForumDatesSelector matches a forum page only when Moodle shows its activity
// dates ("Apertura:", "Cierre:", "Vencimiento:", "Fecha límite:"). A forum
// without configured dates is not evidence of 9.1.3/9.1.4.
const ForumDatesSelector = `#page-mod-forum-view #region-main:has([data-region="activity-dates"], .activity-dates, :text-matches("^\\s*(Apertura|Abri[óo]|Cierre|Cierra|Vencimiento|Vence|Fecha l[ií]mite|Fecha de corte)\\s*:", "i"))`

type CapturePlanSummary struct {
	ItemCount        int `json:"itemCount"`
	ResolvedItems    int `json:"resolvedItems"`
	UnresolvedItems  int `json:"unresolvedItems"`
	SlotCount        int `json:"slotCount"`
	MaxSlotCount     int `json:"maxSlotCount"`
	CaptureUnitCount int `json:"captureUnitCount"`
	CoverageCount    int `json:"coverageCount"`
}

func CaptureSpecs() []CaptureSpec {
	items := Items()
	result := make([]CaptureSpec, 0, len(items))
	for _, item := range items {
		plan := captureGroupPlan(item.GroupName)
		labelHints := captureLabelHints(item.ItemCode, plan.labelHints)
		result = append(result, CaptureSpec{
			ItemCode: item.ItemCode, GroupName: item.GroupName,
			Name: item.Description, MaxSlots: item.MaxEvidences,
			RouteKinds: plan.routeKinds, CSSSelector: plan.selector,
			LabelHints: labelHints,
		})
	}
	return result
}

func captureLabelHints(itemCode string, fallback []string) []string {
	// The instructor profile is one logical full-page evidence. Per-field hints
	// would crop out the photo, name and the rest of the profile.
	if strings.HasPrefix(itemCode, "2.1.") {
		return nil
	}
	// Cronogramas vary between a native Moodle page and an embedded sheet.
	// The route itself identifies the cronograma; a field label is not stable
	// enough to reject the semantic main region when the HTML uses a different
	// layout.
	if strings.HasPrefix(itemCode, "1.1.") || strings.HasPrefix(itemCode, "1.2.") {
		return nil
	}
	// Item 3.1 ("Disponibilidad del material de trabajo y enlaces de envío de
	// evidencias") shares the course main page route with menu_curso and
	// configuracion. Two independent real courses (docs/mdl-33-2026-08-26.md)
	// proved the checklist wording never appears verbatim on that page: the
	// route already identifies the target, so a fragile text hint only makes
	// the capture fail outright.
	if itemCode == "3.1" {
		return nil
	}
	// Item 4.1 ("Menú del Curso organizado con las secciones estipuladas")
	// resolves to the same course main page as 3.1, and its hint ("secciones")
	// is the same anti-pattern: a live run against a real course
	// (docs/mdl-124-verify-2026-09-14.json) proved `#region-main .course-content`
	// on that exact route matches for item 3.1 with no hint at all, but 4.1's
	// "secciones" hint never appears inside it, so the capture falls back all
	// the way to the coarser `#page-content` wrapper instead of using the
	// confirmed, more precise container.
	if itemCode == "4.1" {
		return nil
	}
	// A live run against a real course (docs/mdl-124-seguimiento-2026-09-11.md)
	// proved these checklist descriptions never appear as literal page text
	// either: they describe course-menu structure/organization (a hidden
	// subsection, a missing subsection, a records folder), not visible content.
	// `.course-content .section` matched 284 candidate nodes on the resolved
	// page and 0 matched the hint, so RequireSelector aborted the whole item
	// instead of falling back — the same failure mode fixed for item 3.1.
	// 7.3.2 and 13.1.3 share the literal hint "Documentos de retención": that
	// item can resolve to an actual stored document/resource page instead of
	// the course listing, which has no `.course-content` at all (raw=0), so
	// the same rule applies to both instead of only the group's third slot.
	switch itemCode {
	case "7.1.1", "7.2", "7.3.2", "7.4.1", "7.4.2", "7.4.3", "7.4.4", "8.2", "8.3", "13.1.3", "13.2.2":
		return nil
	}
	// Forum and announcement pages expose the activity title in the heading,
	// while the actual Moodle discussion rows usually contain only the subject
	// and author. The owner filter is the reliable semantic constraint here;
	// requiring a second title hint would reject valid rows.
	if strings.HasPrefix(itemCode, "9.") || strings.HasPrefix(itemCode, "11.") || strings.HasPrefix(itemCode, "14.") || itemCode == "15.1" {
		return nil
	}
	itemHints := map[string][]string{
		"7.1.2": {"Seguimiento a la Formación"},
		"7.3.1": {"Comités evaluativos"}, "7.3.3": {"Reuniones EEF"},
		"8.1":   {"Sesiones en Línea"},
		"9.1.1": {"Dudas e Inquietudes"}, "9.1.2": {"Dudas e Inquietudes"}, "9.1.3": {"Foro Temático"}, "9.1.4": {"Foro Temático"},
		"9.1.5": {"Dudas e Inquietudes"}, "9.1.6": {"Foro Temático"}, "9.1.7": {"Foro Temático"},
		"10.1.1": {"calificación"}, "10.1.2": {"tres días"},
		"11.1.1": {"inicio de fase"}, "11.1.2": {"Fecha de inicio"}, "11.1.3": {"Instrucciones"}, "11.1.4": {"Pasos a seguir"},
		"11.2.1": {"inicio de actividad"}, "11.2.2": {"cierre de actividad"}, "11.2.3": {"sesión en línea"},
		"11.3": {"aprendices aprobados"}, "11.4": {"Anuncio"},
		"12.1.1": {"Grabación"}, "12.1.2": {"Resumen"},
		"13.1.1": {"Reuniones EEF"}, "13.1.2": {"Comités"},
		"13.2.1": {"calificaciones"},
		"14.1.1": {"Foro Temático"}, "14.1.2": {"Conclusión"}, "15.1": {"netiqueta"},
	}
	if hints, ok := itemHints[itemCode]; ok {
		return hints
	}
	return fallback
}

func BuildCaptureTargets(record coursemaps.Record) ([]CaptureTarget, CapturePlanSummary, error) {
	return BuildCaptureTargetsForActivities(record, nil)
}

// BuildCaptureTargetsForActivities applies the instructor's explicit
// activity selection to activity-bound evidence. General course/profile
// evidence remains available, while dates and assignment evidence are tied to
// the selected activity titles and IDs.
func BuildCaptureTargetsForActivities(record coursemaps.Record, selectedActivityIDs map[string]bool) ([]CaptureTarget, CapturePlanSummary, error) {
	if err := ValidateDefinitions(); err != nil {
		return nil, CapturePlanSummary{}, err
	}
	activitiesByID := make(map[string]coursemaps.Activity)
	for _, activity := range coursemaps.Activities(record) {
		activitiesByID[activity.ID] = activity
	}
	// selectionGiven: the instructor saved a selection. If it only held
	// transversal activities it becomes empty, and activity-bound items must
	// then produce nothing, never fall back to every mapped activity.
	selectionGiven := len(selectedActivityIDs) > 0
	selectedActivityIDs = technicalSelection(selectedActivityIDs, activitiesByID)
	routes := newRouteIndex(record)
	targets := make([]CaptureTarget, 0)
	summary := CapturePlanSummary{ItemCount: len(CaptureSpecs())}
	for _, spec := range CaptureSpecs() {
		if selectionBoundItem(spec.ItemCode) && selectionGiven {
			// 10.1.x prove grading and feedback, which live in each selected
			// activity's grading table, not in the course-page card used by
			// 6.1 (that produced byte-identical evidence for 6.1 and 10.1.x).
			added := appendGradingBatchTargets(&targets, record, spec, selectedActivities(activitiesByID, selectedActivityIDs))
			if added > 0 {
				summary.ResolvedItems++
				summary.SlotCount += added
			} else {
				summary.UnresolvedItems++
			}
			summary.MaxSlotCount += spec.MaxSlots
			continue
		}
		if activityBoundItem(spec.ItemCode) && selectionGiven {
			selected := selectedActivities(activitiesByID, selectedActivityIDs)
			if len(selected) > spec.MaxSlots {
				selected = selected[:spec.MaxSlots]
			}
			for index, activity := range selected {
				name := fmt.Sprintf("%s — %s", spec.Name, activity.Title)
				activitySelector := activityCaptureSelector(activity.ID)
				targets = append(targets, CaptureTarget{
					ItemCode: spec.ItemCode, CoveredItemCodes: []string{spec.ItemCode}, GroupName: spec.GroupName, Name: name,
					URL: record.CourseURL, SlotNumber: index + 1,
					ActivityID: activity.ID, ActivityTitle: activity.Title, PhaseSection: activity.PhaseSection, Technical: activity.Technical,
					CSSSelector:          activitySelector,
					CSSSelectorFallbacks: activityCaptureSelectorChain(activity.ID),
					RevealSelectors:      activityRevealSelectors(activity),
					RouteKind:            "course", RequireSelector: true,
				})
			}
			if len(selected) > 0 {
				summary.ResolvedItems++
				summary.SlotCount += len(selected)
			} else {
				// A selection with no technical activity leaves 6.1 without
				// targets: it is unresolved, not silently uncounted.
				summary.UnresolvedItems++
			}
			summary.MaxSlotCount += spec.MaxSlots
			continue
		}
		urls, err := mappedURLs(record.ByItemCode[spec.ItemCode])
		if err != nil {
			return nil, CapturePlanSummary{}, fmt.Errorf("mapa inválido para %s: %w", spec.ItemCode, err)
		}
		if len(urls) == 0 {
			summary.UnresolvedItems++
			summary.MaxSlotCount += spec.MaxSlots
			continue
		}
		summary.ResolvedItems++
		summary.MaxSlotCount += spec.MaxSlots
		type eligibleURL struct {
			url        string
			activityID string
		}
		eligible := make([]eligibleURL, 0, len(urls))
		for _, candidate := range urls {
			if spec.GroupName == "calificaciones" {
				candidate = gradebookSetupURL(candidate)
			}
			route := routes.lookup(candidate)
			if route != nil && !eligibleRouteForGroup(spec.GroupName, *route, selectionGiven, selectedActivityIDs, activitiesByID) {
				continue
			}
			activityID := activityIDForURL(record, candidate)
			if selectionBoundItem(spec.ItemCode) && selectionGiven {
				if activityID == "" || !selectedActivityIDs[activityID] {
					continue
				}
			}
			eligible = append(eligible, eligibleURL{url: candidate, activityID: activityID})
		}
		if len(eligible) > spec.MaxSlots {
			eligible = eligible[:spec.MaxSlots]
		}
		if spec.GroupName == "sesiones_semanales" && len(eligible) == 1 && isCourseViewURL(eligible[0].url) {
			for index := 0; index < spec.MaxSlots; index++ {
				selector := grabacionesSectionSelector(index)
				targets = append(targets, CaptureTarget{
					ItemCode: spec.ItemCode, CoveredItemCodes: []string{spec.ItemCode}, GroupName: spec.GroupName,
					Name: fmt.Sprintf("%s — Evidencia %d", spec.Name, index+1), URL: eligible[0].url, SlotNumber: index + 1,
					CSSSelector: selector, CSSSelectorFallbacks: []string{selector},
					RouteKind: "course", RequireSelector: true, OptionalSlot: true,
				})
			}
			summary.SlotCount += spec.MaxSlots
			continue
		}
		rows, batched := rowBatchPlanFor(spec.ItemCode, spec.GroupName)
		addedCount := 0
		for index, entry := range eligible {
			batchesPerURL := 1
			if batched {
				batchesPerURL = batchesForList(spec.MaxSlots, len(eligible), index)
			}
			hint := ""
			if len(spec.LabelHints) > 0 {
				hint = spec.LabelHints[index%len(spec.LabelHints)]
			}
			activityTitle := ""
			technical := false
			if activity, ok := activitiesByID[entry.activityID]; ok {
				activityTitle = activity.Title
				technical = activity.Technical
			}
			ownerOnly := ownerOnlyForItem(spec.ItemCode)
			selector := captureSelectorForItem(spec.ItemCode, spec.GroupName, spec.CSSSelector)
			fallbacks := captureSelectorChainForItem(spec.ItemCode, spec.GroupName, selector)
			elementOnly := false
			if (spec.GroupName == "cronograma_general" || spec.GroupName == "cronograma_vigente") && strings.Contains(entry.url, "/mod/") {
				// A cronograma published as a page/resource has no course
				// sections: its main region is the evidence (not a fallback).
				selector = `#region-main:has(iframe[src*="docs.google.com/spreadsheets"]), #region-main`
				// No #page-content fallback: it is the whole layout wrapper.
				fallbacks = []string{selector}
				// Capture the content region only (no Zajuna header, side
				// menu or footer): the enlarged sheet is inside it.
				elementOnly = true
			}
			if batched {
				// The container that holds the rows goes first; the previous
				// chain stays as fallback (then captured without batching).
				selector = rows.container
				fallbacks = append([]string{rows.container}, fallbacks...)
			}
			for batch := 0; batch < batchesPerURL; batch++ {
				// Slots are contiguous: skipped routes never leave holes
				// (a gap used to leave items with only "slot 2").
				slot := addedCount + 1
				name := spec.Name
				if spec.MaxSlots > 1 {
					name = fmt.Sprintf("%s — Evidencia %d", name, slot)
				}
				target := CaptureTarget{
					ItemCode: spec.ItemCode, CoveredItemCodes: []string{spec.ItemCode}, GroupName: spec.GroupName,
					Name: name, URL: entry.url, SlotNumber: slot,
					ActivityID: entry.activityID, ActivityTitle: activityTitle, Technical: technical,
					CSSSelector: selector, CSSSelectorFallbacks: fallbacks,
					HideSelectors: forumConfigurationHideSelectors(spec.ItemCode, spec.GroupName),
					ViewportWidth: viewportWidthForGroup(spec.GroupName), ViewportHeight: viewportHeightForGroup(spec.GroupName),
					FullPage:  fullPageForGroup(spec.GroupName) && !batched && !elementOnly,
					LabelHint: hint, RouteKind: routeKindForURL(record, spec.ItemCode, entry.url),
					// Checklist evidence must come from its semantic container:
					// a generic full-page shot of an unrelated layout is never a
					// valid substitute when the selector chain does not match.
					RequireSelector: true,
					OwnerOnly:       ownerOnly,
				}
				if batched {
					target.RowSelector, target.RowsPerShot, target.RowBatch = rows.rowSelector, RowsPerShot, batch
					target.RowMatch = rowMatchForItem(spec.ItemCode)
					target.RowRequireReply = SemanticCheckForItem(spec.ItemCode) == SemanticForumReplies
				}
				target.SemanticCheck = SemanticCheckForItem(spec.ItemCode)
				target.CourseLayout = courseLayoutForGroup(spec.GroupName)
				if target.SemanticCheck == SemanticForumDates {
					target.AbsenceSelector = forumPageSelector
				}
				if spec.GroupName == "calificaciones" && !strings.Contains(target.URL, "/grade/edit/tree/") {
					// The grader report has one column per grade item (~28.000
					// px on a real course): each slot shows the first rows and
					// the next window of columns, never wider than the limit.
					target.RowBatch, target.ColumnBatch, target.MaxCaptureWidth = 0, batch, MaxCaptureWidth
				} else if spec.GroupName == "calificaciones" {
					// The gradebook setup lists every grade item vertically
					// (146 rows on ficha 3135429): the slots split it into
					// readable row batches that together cover the list.
					target.RowsPerShot = GradebookRowsPerShot
				}
				targets = append(targets, target)
				addedCount++
			}
		}
		summary.SlotCount += addedCount
	}
	targets = deduplicateCaptureTargets(targets)
	summary.CaptureUnitCount = len(targets)
	for _, target := range targets {
		summary.CoverageCount += len(target.CoveredItemCodes)
	}
	return targets, summary, nil
}

// deduplicateCaptureTargets collapses only byte-for-byte equivalent capture
// configurations. The checklist items remain visible through
// CoveredItemCodes, while the worker can render one physical artifact and
// persist logical aliases for every covered criterion.
func deduplicateCaptureTargets(targets []CaptureTarget) []CaptureTarget {
	if len(targets) < 2 {
		for index := range targets {
			targets[index].CoveredItemCodes = normalizedCoveredItemCodes(targets[index].ItemCode, targets[index].CoveredItemCodes)
		}
		return targets
	}
	result := make([]CaptureTarget, 0, len(targets))
	byKey := make(map[string]int, len(targets))
	for _, target := range targets {
		target.CoveredItemCodes = normalizedCoveredItemCodes(target.ItemCode, target.CoveredItemCodes)
		key := captureUnitKey(target)
		if existingIndex, ok := byKey[key]; ok {
			result[existingIndex].CoveredItemCodes = mergeCoveredItemCodes(
				result[existingIndex].CoveredItemCodes,
				target.CoveredItemCodes,
			)
			continue
		}
		byKey[key] = len(result)
		result = append(result, target)
	}
	return result
}

func captureUnitKey(target CaptureTarget) string {
	return strings.Join([]string{
		strings.TrimSpace(target.GroupName),
		captureShareKey(target.ItemCode, target.GroupName),
		canonicalRouteURL(target.URL),
		strings.TrimSpace(target.RouteKind),
		strings.TrimSpace(target.CSSSelector),
		strings.Join(target.CSSSelectorFallbacks, "\x00"),
		strings.Join(target.RevealSelectors, "\x00"),
		strings.Join(target.HideSelectors, "\x00"),
		strings.TrimSpace(target.LabelHint),
		strings.TrimSpace(target.ActivityID),
		strconv.Itoa(target.PhaseSection),
		strconv.Itoa(target.SlotNumber),
		strconv.Itoa(target.ViewportWidth),
		strconv.Itoa(target.ViewportHeight),
		strconv.FormatBool(target.FullPage),
		strconv.FormatBool(target.RequireSelector),
		strconv.FormatBool(target.OwnerOnly),
		strings.TrimSpace(target.RowSelector),
		strconv.Itoa(target.RowsPerShot),
		strconv.Itoa(target.RowBatch),
		strconv.FormatBool(target.OptionalSlot),
		strings.Join(target.RowMatch, "\x00"),
		strconv.FormatBool(target.RowRequireReply),
		strings.TrimSpace(target.SemanticCheck),
		strings.TrimSpace(target.CourseLayout),
		strings.TrimSpace(target.AbsenceSelector),
		strconv.Itoa(target.MaxCaptureWidth),
		strconv.Itoa(target.ColumnBatch),
	}, "\x1f")
}

// captureShareKey is intentionally allow-listed. A matching URL and selector
// is not enough to prove that two checklist criteria are interchangeable: for
// example, grading and deadline checks may use the same activity context but
// still require different semantic validators. The allow-list covers
// genuinely shared visual contexts and leaves the rest item-specific.
// Forum/announcement lists and the grading table are shared too: several
// items are proven by the very same rows (e.g. 10.1.1 grading and 10.1.2
// feedback deadline), and capturing them once per item only produced
// byte-identical duplicate evidence. Units still merge only when every other
// capture setting (URL, selectors, owner filter, row batch) is identical.
func captureShareKey(itemCode, groupName string) string {
	switch groupName {
	case "cronograma_general", "cronograma_vigente", "perfil_instructor", "sesiones_semanales",
		"foros", "anuncios_fase", "anuncios_semanales", "conclusion_foros", "evidencias_aprendizaje":
		return groupName
	default:
		return strings.TrimSpace(groupName) + "|" + strings.TrimSpace(itemCode)
	}
}

func normalizedCoveredItemCodes(primary string, codes []string) []string {
	merged := make([]string, 0, len(codes)+1)
	if primary = strings.TrimSpace(primary); primary != "" {
		merged = append(merged, primary)
	}
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		found := false
		for _, existing := range merged {
			if existing == code {
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, code)
		}
	}
	return sortItemCodes(merged)
}

func mergeCoveredItemCodes(left, right []string) []string {
	merged := append([]string{}, left...)
	return normalizedCoveredItemCodes("", append(merged, right...))
}

func sortItemCodes(codes []string) []string {
	order := make(map[string]int, len(Items()))
	for index, item := range Items() {
		order[item.ItemCode] = index
	}
	sort.SliceStable(codes, func(i, j int) bool {
		left, leftOK := order[codes[i]]
		right, rightOK := order[codes[j]]
		if leftOK && rightOK && left != right {
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return codes[i] < codes[j]
	})
	return codes
}

func activityBoundItem(itemCode string) bool {
	return itemCode == "6.1" || selectionBoundItem(itemCode)
}

func selectionBoundItem(itemCode string) bool {
	switch itemCode {
	case "10.1.1", "10.1.2":
		return true
	default:
		return false
	}
}

func selectedActivities(byID map[string]coursemaps.Activity, selectedIDs map[string]bool) []coursemaps.Activity {
	result := make([]coursemaps.Activity, 0, len(selectedIDs))
	for id := range selectedIDs {
		if activity, ok := byID[id]; ok {
			result = append(result, activity)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].PhaseSection != result[j].PhaseSection {
			return result[i].PhaseSection < result[j].PhaseSection
		}
		return strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
	})
	return result
}

func activityIDForURL(record coursemaps.Record, targetURL string) string {
	for _, route := range record.Routes {
		if route.URL != targetURL || route.Kind != "assign" {
			continue
		}
		if strings.TrimSpace(route.ActivityID) != "" {
			return strings.TrimSpace(route.ActivityID)
		}
		parsed, err := url.Parse(route.URL)
		if err == nil {
			return strings.TrimSpace(parsed.Query().Get("id"))
		}
	}
	parsed, err := url.Parse(targetURL)
	if err == nil && strings.Contains(parsed.Path, "/mod/assign/") {
		return strings.TrimSpace(parsed.Query().Get("id"))
	}
	return ""
}

func activityCaptureSelector(activityID string) string {
	id := strings.TrimSpace(activityID)
	return fmt.Sprintf("#region-main .course-content #module-%s", id)
}

func activityCaptureSelectorChain(activityID string) []string {
	id := strings.TrimSpace(activityID)
	if id == "" {
		return nil
	}
	return []string{
		activityCaptureSelector(id),
		fmt.Sprintf("#region-main .course-content li#module-%s .activity-item", id),
		fmt.Sprintf("#region-main .course-content .activity-item:has(a[href*=\"id=%s\"])", id),
		fmt.Sprintf("#region-main .course-content li#module-%s", id),
		fmt.Sprintf("#region-main .course-content a[href*=\"id=%s\"]", id),
	}
}

func activityRevealSelectors(activity coursemaps.Activity) []string {
	if activity.PhaseSection <= 0 {
		return nil
	}
	return []string{fmt.Sprintf("#collapssesection%d", activity.PhaseSection)}
}

func mappedURLs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var urls []string
	if err := json.Unmarshal(raw, &urls); err != nil {
		var single string
		if singleErr := json.Unmarshal(raw, &single); singleErr != nil {
			return nil, err
		}
		urls = []string{single}
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(urls))
	for _, value := range urls {
		value = strings.TrimSpace(value)
		key := canonicalRouteURL(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result, nil
}

func routeKindForURL(record coursemaps.Record, itemCode, targetURL string) string {
	for _, route := range record.Routes {
		if route.URL == targetURL {
			return route.Kind
		}
	}
	for _, kind := range captureSpecFor(itemCode).RouteKinds {
		return kind
	}
	return "route"
}

// routeIndex resolves a mapped URL to its route without rescanning (and
// re-parsing) every route of the course for each candidate. An exact URL match
// is also a canonical match, so keeping the first route per canonical URL
// returns the same route the previous linear scan did.
type routeIndex map[string]*coursemaps.Route

func newRouteIndex(record coursemaps.Record) routeIndex {
	index := make(routeIndex, len(record.Routes))
	for position := range record.Routes {
		key := canonicalRouteURL(record.Routes[position].URL)
		if _, exists := index[key]; !exists {
			index[key] = &record.Routes[position]
		}
	}
	return index
}

func (index routeIndex) lookup(targetURL string) *coursemaps.Route {
	return index[canonicalRouteURL(targetURL)]
}

func canonicalRouteURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	query := parsed.Query()
	query.Del("forceview")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// eligibleRouteForGroup prevents the route map's auxiliary forum links from
// becoming evidence. Forum pages are only eligible when they are a real
// view.php activity route, are not a discussion child/order/action URL, and
// are not explicitly transversal. If a route has an activity code, it must
// match one of the instructor-selected activities; generic named forums and
// announcements remain eligible because their author is checked in Chromium.
func eligibleRouteForGroup(groupName string, route coursemaps.Route, selectionGiven bool, selectedActivityIDs map[string]bool, activitiesByID map[string]coursemaps.Activity) bool {
	if route.Restricted {
		return false
	}
	if !ownerFilteredGroup(groupName) {
		return true
	}
	if route.Kind != "forum" {
		return false
	}
	parsed, err := url.Parse(route.URL)
	if err != nil || !strings.HasSuffix(strings.ToLower(parsed.Path), "/view.php") {
		return false
	}
	query := parsed.Query()
	if query.Get("id") == "" || query.Get("parent") != "" || query.Get("d") != "" || query.Get("o") != "" || query.Get("sesskey") != "" {
		return false
	}
	title := strings.TrimSpace(route.Title)
	if title == "" || genericForumNavigationTitle(title) {
		return false
	}
	if !coursemaps.IsTechnicalActivity(title) {
		return false
	}
	if len(selectedActivityIDs) == 0 {
		// No selection: every forum qualifies. A saved selection that only
		// held transversal activities is empty here too, but then a forum
		// bound to an activity must not qualify (the instructor chose none).
		return !selectionGiven || (strings.TrimSpace(route.ActivityID) == "" && !routeHasActivityCode(title))
	}
	if activityID := strings.TrimSpace(route.ActivityID); activityID != "" {
		return selectedActivityIDs[activityID]
	}
	if routeHasActivityCode(title) {
		for id := range selectedActivityIDs {
			activity, ok := activitiesByID[id]
			if ok && activityCodeOverlap(title, activity.Title) {
				return true
			}
		}
		return false
	}
	return true
}

func ownerFilteredGroup(groupName string) bool {
	switch groupName {
	case "foros", "anuncios_fase", "anuncios_semanales", "conclusion_foros", "netiqueta":
		return true
	default:
		return false
	}
}

func ownerOnlyForItem(itemCode string) bool {
	switch itemCode {
	case "9.1.5", "9.1.6", "9.1.7",
		"11.1.1", "11.1.2", "11.1.3", "11.1.4", "11.2.1", "11.2.2", "11.2.3", "11.3", "11.4",
		"14.1.1", "14.1.2", "15.1":
		return true
	default:
		return false
	}
}

// RequiresInstructorIdentity tells the worker whether a batch contains a
// target that must be scoped to the authenticated instructor before capture.
func RequiresInstructorIdentity(targets []CaptureTarget) bool {
	for _, target := range targets {
		if target.OwnerOnly {
			return true
		}
	}
	return false
}

func genericForumNavigationTitle(title string) bool {
	value := strings.ToLower(strings.TrimSpace(title))
	// "Anuncios de la página" is the Zajuna site-wide news forum, reached
	// from the navigation: it is not part of the course and redirects to the
	// login page, so every batch failed and forced new logins (MDL-219).
	for _, term := range []string{"debate", "comenzado por", "último mensaje", "ultimo mensaje", "réplicas", "replicas", "fijar esta discusión", "fijar esta discusion", "mostrar comentarios", "excepciones", "anuncios de la página", "anuncios de la pagina", "anuncios del sitio", "noticias del sitio"} {
		if value == term {
			return true
		}
	}
	return false
}

func routeHasActivityCode(title string) bool {
	value := strings.ToUpper(title)
	return strings.Contains(value, "GA") && strings.Contains(value, "AA")
}

func activityCodeOverlap(left, right string) bool {
	left = strings.ToUpper(strings.TrimSpace(left))
	right = strings.ToUpper(strings.TrimSpace(right))
	if left == "" || right == "" {
		return false
	}
	for _, leftToken := range activityReferencePattern.FindAllString(left, -1) {
		for _, rightToken := range activityReferencePattern.FindAllString(right, -1) {
			if leftToken == rightToken {
				return true
			}
		}
	}
	return false
}

var activityReferencePattern = regexp.MustCompile(`(?i)AA\d+(?:-EV\d+)?`)

type groupPlan struct {
	routeKinds []string
	selector   string
	labelHints []string
	ownerOnly  bool
}

func captureGroupPlan(groupName string) groupPlan {
	switch groupName {
	case "cronograma_general":
		return groupPlan{[]string{"page"}, `#region-main .course-content:has(iframe[src*="docs.google.com/spreadsheets"]), #region-main .course-content`, []string{"Fases", "Actividades de proyecto", "Actividades de aprendizaje", "duración", "Fecha de inicio"}, false}
	case "cronograma_vigente":
		return groupPlan{[]string{"phase"}, "#region-main .course-content .section", []string{"Nombre de la Fase", "actividades de proyecto", "Actividades de aprendizaje", "Resultados de Aprendizaje", "Fecha de inicio", "Evidencias a presentar", "Instructor"}, false}
	case "perfil_instructor":
		return groupPlan{[]string{"profile"}, "#page-user-profile", nil, false}
	case "disponibilidad":
		// Resolves to the course main page (client_maps_resolver.go), the same
		// route as menu_curso and configuracion. `.course-content .section`
		// matches hundreds of unrelated nodes there (284 in both real courses
		// audited for MDL-124) instead of the topic sections it was meant for,
		// so it is not a usable crop even without a label hint.
		return groupPlan{[]string{"page", "resource", "url"}, "#region-main .course-content", nil, false}
	case "menu_curso":
		// Resolves to the same course main page as "disponibilidad" (item 3.1).
		// The checklist wording ("secciones") never appears literally there
		// either, so the route alone identifies the target instead of a hint.
		return groupPlan{[]string{"course"}, "#region-main .course-content", nil, false}
	case "calificaciones":
		return groupPlan{[]string{"grading"}, gradebookSetupTable, []string{"calificaciones"}, false}
	case "configuracion":
		return groupPlan{[]string{"page", "course", "phase"}, "#region-main .course-content .section", nil, false}
	case "seguimiento_evaluacion", "seguimiento_documentos", "documentos_retencion":
		// `.section` alone took the first section of the course page (the
		// ANUNCIOS banner, section 0) for every item. Playwright's :has-text
		// scopes it to the named section; the fallback chain then uses the
		// whole course content instead of an unrelated banner.
		return groupPlan{[]string{"page", "course", "phase"}, courseSectionByTitle(seguimientoSectionTitle), nil, false}
	case "sesiones_linea":
		return groupPlan{[]string{"page", "course", "phase"}, courseSectionByTitle(sesionesSectionTitle), nil, false}
	case "foros", "anuncios_fase", "anuncios_semanales", "conclusion_foros", "netiqueta":
		return groupPlan{[]string{"forum"}, "#region-main .forum_list .forum", []string{"Foro", "Anuncio", "sesión en línea"}, true}
	case "evidencias_aprendizaje":
		return groupPlan{[]string{"assign", "grading"}, "#region-main .assign", []string{"calificación", "retroalimentación"}, false}
	case "sesiones_semanales":
		return groupPlan{[]string{"page", "resource", "url"}, "#region-main .course-content .section", []string{"Grabación", "Resumen"}, false}
	default:
		return groupPlan{[]string{"page", "course"}, "#region-main", nil, false}
	}
}

func captureSelectorChain(groupName, primary string) []string {
	selectors := make([]string, 0, 8)
	add := func(selector string) {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return
		}
		for _, existing := range selectors {
			if existing == selector {
				return
			}
		}
		selectors = append(selectors, selector)
	}
	add(primary)
	groupSelectors := map[string][]string{
		"cronograma_general":     {"#region-main .course-content", "#region-main"},
		"cronograma_vigente":     {"#region-main .course-content .section", "#region-main .course-content", "#region-main"},
		"disponibilidad":         {"#region-main .course-content"},
		"perfil_instructor":      {"#page-user-profile", "#region-main"},
		"menu_curso":             {"#region-main .course-content", ".course-content"},
		"calificaciones":         {gradebookSetupTable, "#region-main table", "#region-main"},
		"foros":                  {"#region-main .forum_list .forum", "#region-main .forumpost", "#region-main [data-region='post']"},
		"anuncios_fase":          {"#region-main .forum_list .forum", "#region-main .forumpost", "#region-main [data-region='post']"},
		"anuncios_semanales":     {"#region-main .forum_list .forum", "#region-main .forumpost", "#region-main [data-region='post']"},
		"conclusion_foros":       {"#region-main .forum_list .forum", "#region-main .forumpost", "#region-main [data-region='post']"},
		"netiqueta":              {"#region-main .forum_list .forum", "#region-main .forumpost", "#region-main [data-region='post']"},
		"evidencias_aprendizaje": {"#region-main .assign", "#region-main table", "#region-main"},
		"sesiones_semanales":     {"#region-main .course-content .section", "#region-main"},
	}
	for _, selector := range groupSelectors[groupName] {
		add(selector)
	}
	if ownerFilteredGroup(groupName) {
		selectors = append(selectors,
			"#region-main table.forumheaderlist tr",
			"#region-main .discussion",
			"#region-main .forum-post",
			"#region-main article",
		)
		return selectors
	}
	// `#page-content` wraps the whole Moodle layout (navigation, blocks and
	// footer): matching it is a full-page shot in disguise, so it is never a
	// fallback. The profile container only belongs to the profile chain.
	for _, selector := range []string{"#region-main .course-content", "#region-main", ".course-content"} {
		add(selector)
	}
	return selectors
}

func captureSelectorForItem(itemCode, groupName, fallback string) string {
	if groupName == "foros" && (itemCode == "9.1.3" || itemCode == "9.1.4") {
		return ForumDatesSelector
	}
	if groupName == "foros" && (itemCode == "9.1.1" || itemCode == "9.1.2") {
		return "#page-mod-forum-view #region-main"
	}
	if title := courseSectionTitleForItem(itemCode); title != "" {
		if title == seguimientoSectionTitle || title == sesionesSectionTitle {
			return topLevelCourseSectionByTitle(title)
		}
		return courseSectionByTitle(title)
	}
	return fallback
}

func captureSelectorChainForItem(itemCode, groupName, primary string) []string {
	if primary == ForumDatesSelector {
		// No fallback: a forum page without dates must not become evidence.
		return []string{ForumDatesSelector}
	}
	title := courseSectionTitleForItem(itemCode)
	if title == "" {
		return captureSelectorChain(groupName, primary)
	}
	// Section-bound items are identified only by their named section. A
	// missing subsection falls back to its parent section, never to the
	// whole course page or its first (banner) section.
	chain := []string{primary}
	if title != seguimientoSectionTitle && title != sesionesSectionTitle {
		parent := seguimientoSectionTitle
		if strings.HasPrefix(itemCode, "8.") {
			parent = sesionesSectionTitle
		} else if strings.HasPrefix(itemCode, "7.4.") && title != comitesSectionTitle {
			parent = comitesSectionTitle
		}
		chain = append(chain, courseSectionByTitle(parent))
	}
	return chain
}

// splitSlots spreads an item's evidence limit over its sources so the
// remainder of an uneven split is not lost: 5 slots over 2 sources become
// 3+2, not 2+2. Every source keeps at least one slot.
func splitSlots(maxSlots, sources int) []int {
	if sources <= 0 {
		return nil
	}
	result := make([]int, sources)
	base, remainder := maxSlots/sources, maxSlots%sources
	for index := range result {
		result[index] = base
		if index < remainder {
			result[index]++
		}
		if result[index] < 1 {
			result[index] = 1
		}
	}
	return result
}

const (
	seguimientoSectionTitle = "Seguimiento y Evaluaci"
	sesionesSectionTitle    = "Sesiones en l"
	comitesSectionTitle     = "Comités evaluativos"
)

// rowMatchForItem returns the announcement topics each item is about.
// 11.4 (format) and 15.1 (netiqueta) apply to every instructor post.
func rowMatchForItem(itemCode string) []string {
	switch itemCode {
	case "11.1.1", "11.1.2", "11.1.3", "11.1.4":
		return []string{"apertura de fase", "inicio de fase", "apertura fase"}
	case "11.2.1":
		return []string{"inicio de actividad"}
	case "11.2.2":
		return []string{"cierre de actividad"}
	case "11.2.3":
		return []string{"invitacion a sesion", "sesion en linea"}
	case "11.3":
		return []string{"aprobados"}
	case "14.1.1", "14.1.2":
		return []string{"conclusion"}
	}
	return nil
}

// courseSectionTitleForItem maps checklist items to the Moodle section or
// subsection whose own title identifies them. Verified against a real SENA
// course (docs/api-local.md): "Seguimiento y Evaluación" holds "Reporte del
// Curso", "Seguimiento a la Formación", "Comités evaluativos - Actas" and
// "Documentos de retención de aprendices"; "Sesiones en línea" holds the
// per-phase "Grabaciones sesiones en línea" subsections. Titles are
// substrings matched case-insensitively by Playwright's :has-text.
func courseSectionTitleForItem(itemCode string) string {
	switch itemCode {
	case "7.1.1", "7.1.2":
		return seguimientoSectionTitle
	case "7.2", "13.2.1", "13.2.2":
		return "Reporte del Curso"
	case "7.3.1", "7.4.1", "13.1.2":
		return comitesSectionTitle
	case "7.3.2", "13.1.3":
		return "Documentos de retenci"
	case "7.3.3", "13.1.1":
		return "Reuniones EEF"
	case "7.4.2":
		return "Planes de Mejoramiento"
	case "7.4.3":
		return "Registro de Novedades"
	case "7.4.4":
		return "Llamados de atenci"
	case "8.1", "8.2", "8.3":
		return sesionesSectionTitle
	}
	return ""
}

// courseSectionByTitle matches the section whose OWN header carries the
// title. `.section:has-text()` also matched every ancestor section (e.g.
// "Información general" containing a nested "Seguimiento y evaluación").
func courseSectionByTitle(title string) string {
	// :text-matches only matches an element's own text, so the variant with
	// the title inside a link (<h3 class="sectionname"><a>…</a></h3>, other
	// Moodle themes) is listed too.
	pattern := sectionTitlePattern(title)
	return fmt.Sprintf(`#region-main .course-content li.section:has(> .course-section-header .sectionname:text-matches(%q, "i")), #region-main .course-content li.section:has(> .course-section-header .sectionname a:text-matches(%q, "i"))`, pattern, pattern)
}

// sectionTitlePattern anchors the title at the start of the section name:
// a substring also matched "Planeación, Seguimiento y Evaluación" when the
// item was about the "Seguimiento y Evaluación" section.
func sectionTitlePattern(title string) string {
	return `^\s*` + regexp.QuoteMeta(title)
}

// topLevelCourseSectionByTitle only matches a main course section, never a
// subsection with the same name (a SENA course also has a hidden
// "Seguimiento y evaluación" inside "Información general").
func topLevelCourseSectionByTitle(title string) string {
	pattern := sectionTitlePattern(title)
	return fmt.Sprintf(`#region-main .course-content li.section:not(li.section li.section):has(> .course-section-header .sectionname:text-matches(%q, "i")), #region-main .course-content li.section:not(li.section li.section):has(> .course-section-header .sectionname a:text-matches(%q, "i"))`, pattern, pattern)
}

// grabacionesSectionSelector picks the N-th per-phase recordings section.
func grabacionesSectionSelector(index int) string {
	// "Fase 1 Planear: Grabaciones sesiones en línea": the phase comes first,
	// so this one is matched anywhere in the name.
	return `#region-main .course-content li.section:has(> .course-section-header .sectionname:has-text("Grabaciones sesiones en l"))` + fmt.Sprintf(" >> nth=%d", index)
}

func isCourseViewURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.HasSuffix(strings.ToLower(parsed.Path), "/course/view.php")
}

func forumConfigurationHideSelectors(itemCode, groupName string) []string {
	if groupName == "foros" && !ownerOnlyForItem(itemCode) {
		return []string{"#region-main table"}
	}
	return nil
}

func viewportWidthForGroup(groupName string) int {
	if groupName == "cronograma_general" || groupName == "cronograma_vigente" {
		return 2560
	}
	if groupName == "calificaciones" {
		return 1440
	}
	return 0
}

func viewportHeightForGroup(groupName string) int {
	if groupName == "cronograma_general" || groupName == "cronograma_vigente" {
		return 1200
	}
	if groupName == "calificaciones" {
		return 900
	}
	return 0
}

func fullPageForGroup(groupName string) bool {
	// The instructor profile is a single logical evidence. It must include the
	// photo, identity, description, contact details and availability instead of
	// cropping only the first visible profile card. Cronogramas use the same
	// contract so native HTML phases include the complete table below the fold.
	return groupName == "perfil_instructor" || groupName == "cronograma_general" || groupName == "cronograma_vigente"
}

func captureSpecFor(itemCode string) CaptureSpec {
	for _, spec := range CaptureSpecs() {
		if spec.ItemCode == itemCode {
			return spec
		}
	}
	return CaptureSpec{ItemCode: itemCode, RouteKinds: []string{"route"}}
}

// RowsPerShot is the batch size for list/table evidence: slot 1 shows rows
// 1–2, slot 2 rows 3–4, and so on, up to the item's evidence limit.
const RowsPerShot = 2

// MaxCaptureWidth matches the widest viewport used elsewhere (cronogramas):
// wider shots become unreadable once scaled into the report.
const MaxCaptureWidth = 2560

// GradebookRowsPerShot is the rows of the gradebook setup list per 5.1 slot:
// five slots cover 150 grade items, each shot stays well under the review's
// height limit.
const GradebookRowsPerShot = 30

// batchesForList splits an item's evidence limit among its lists. Integer
// division alone left slots unplanned (5 slots over 2 lists planned 4): the
// remainder goes to the first lists, so every slot up to the limit exists.
func batchesForList(maxSlots, lists, index int) int {
	if lists <= 0 || maxSlots <= lists {
		return 1
	}
	batches := maxSlots / lists
	if index < maxSlots%lists {
		batches++
	}
	return batches
}

type rowBatchPlan struct {
	container   string
	rowSelector string
}

// rowBatchPlanFor lists the list/table-shaped evidence captured in row
// batches instead of one whole element (or a single row) per slot.
func rowBatchPlanFor(itemCode, groupName string) (rowBatchPlan, bool) {
	if groupName == "calificaciones" {
		return rowBatchPlan{container: gradebookSetupTable, rowSelector: "tbody tr:not(.spacer)"}, true
	}
	if ownerOnlyForItem(itemCode) {
		// Instructor-authored discussions/announcements; the capture worker
		// applies the owner filter per row.
		return rowBatchPlan{container: "#region-main table.discussion-list, #region-main table.forumheaderlist", rowSelector: "tbody tr"}, true
	}
	return rowBatchPlan{}, false
}

// distributeSlotBatches splits MaxSlots among n lists, giving leftover
// slots to the first lists so 8 slots and 3 lists become 3,3,2 — not 2,2,2.
func distributeSlotBatches(maxSlots, n int) []int {
	if n <= 0 {
		return nil
	}
	if n > maxSlots {
		n = maxSlots
	}
	base := maxSlots / n
	if base < 1 {
		base = 1
	}
	counts := make([]int, n)
	used := 0
	for i := 0; i < n; i++ {
		counts[i] = base
		used += base
	}
	for i := 0; used < maxSlots && i < n; i++ {
		counts[i]++
		used++
	}
	return counts
}

const gradingTableSelector = "#region-main table.generaltable"

// GradedRowTerm is Moodle's status text of a graded submission.
const GradedRowTerm = "calificado"

// appendGradingBatchTargets captures each selected activity's grading table
// (grade, feedback and modification date) in row batches, splitting the
// item's evidence limit among the activities.
func appendGradingBatchTargets(targets *[]CaptureTarget, record coursemaps.Record, spec CaptureSpec, selected []coursemaps.Activity) int {
	type gradingActivity struct {
		activity coursemaps.Activity
		url      string
	}
	withGrading := make([]gradingActivity, 0, len(selected))
	for _, activity := range selected {
		for _, route := range record.Routes {
			if route.Kind == "grading" && strings.TrimSpace(route.ActivityID) == strings.TrimSpace(activity.ID) && strings.TrimSpace(route.URL) != "" {
				withGrading = append(withGrading, gradingActivity{activity: activity, url: route.URL})
				break
			}
		}
	}
	if len(withGrading) > spec.MaxSlots {
		withGrading = withGrading[:spec.MaxSlots]
	}
	if len(withGrading) == 0 {
		return 0
	}
	batchCounts := distributeSlotBatches(spec.MaxSlots, len(withGrading))
	added := 0
	for index, entry := range withGrading {
		for batch := 0; batch < batchCounts[index]; batch++ {
			added++
			*targets = append(*targets, CaptureTarget{
				ItemCode: spec.ItemCode, CoveredItemCodes: []string{spec.ItemCode}, GroupName: spec.GroupName,
				Name: fmt.Sprintf("%s — %s", spec.Name, entry.activity.Title), URL: entry.url, SlotNumber: added,
				ActivityID: entry.activity.ID, ActivityTitle: entry.activity.Title, PhaseSection: entry.activity.PhaseSection, Technical: entry.activity.Technical,
				CSSSelector:          gradingTableSelector,
				CSSSelectorFallbacks: []string{gradingTableSelector, "#region-main .gradingtable table", "#region-main table"},
				RouteKind:            "grading", RequireSelector: true,
				RowSelector: "tbody tr:not(.emptyrow)", RowsPerShot: RowsPerShot, RowBatch: batch,
				// 10.1.x prove feedback and grading: only graded submissions
				// ("Calificado" in the status column), never the first rows
				// of students without a submission.
				RowMatch: []string{GradedRowTerm},
			})
		}
	}
	return added
}

// technicalSelection drops transversal activities from a saved selection:
// they belong to other instructors and only produce wrong evidence (older
// versions allowed selecting them). IDs missing from the map are dropped too.
func technicalSelection(selected map[string]bool, activitiesByID map[string]coursemaps.Activity) map[string]bool {
	if len(selected) == 0 {
		return selected
	}
	result := make(map[string]bool, len(selected))
	for id, ok := range selected {
		if activity, exists := activitiesByID[id]; ok && exists && activity.Technical {
			result[id] = true
		}
	}
	return result
}

// ActivityEvidenceSlots is how many selected activities feed each
// activity-bound item (6.1, 10.1.1, 10.1.2): the first ones in phase order.
func ActivityEvidenceSlots() int {
	for _, spec := range CaptureSpecs() {
		if spec.ItemCode == "6.1" {
			return spec.MaxSlots
		}
	}
	return 5
}

// TechnicalSelectionForRecord keeps only the technical activities of a saved
// selection, using the course map. Callers use it before deciding whether a
// selection exists (a transversal-only selection counts as none).
func TechnicalSelectionForRecord(record coursemaps.Record, selected map[string]bool) map[string]bool {
	activitiesByID := make(map[string]coursemaps.Activity)
	for _, activity := range coursemaps.Activities(record) {
		activitiesByID[activity.ID] = activity
	}
	return technicalSelection(selected, activitiesByID)
}

// gradebookSetupTable is the gradebook setup tree (categories and the
// activities associated with each), used as evidence for 5.1.
const gradebookSetupTable = "#region-main table#grade_edit_tree_table, #region-main table.setup-grades"

// gradebookSetupURL rewrites a grader-report URL stored by older course maps
// to the gradebook setup page of the same course.
func gradebookSetupURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.Contains(parsed.Path, "/grade/report/grader/") {
		return raw
	}
	parsed.Path = strings.Replace(parsed.Path, "/grade/report/grader/", "/grade/edit/tree/", 1)
	return parsed.String()
}
