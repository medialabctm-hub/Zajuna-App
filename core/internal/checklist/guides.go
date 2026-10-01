package checklist

import (
	"strings"
)

// Guide kinds: why an item is out of the app's reach. In all of them the
// missing piece is content only the instructor can create in Zajuna, so the
// app guides instead of capturing.
const (
	// GuideContentAbsent: the right page loaded but lacks what the item asks
	// for (a forum without the instructor's replies, without a conclusion,
	// without dates; an activity without graded submissions).
	GuideContentAbsent = "content-absent"
	// GuideEmptySection: the subsection exists but shows no file or activity.
	GuideEmptySection = "empty-section"
	// GuideRouteMissing: route discovery did not find where the item lives
	// in the course (the section or forum does not exist, or has another name).
	GuideRouteMissing = "route-missing"
	// GuideContentError: the evidence exists but what Zajuna shows is wrong
	// (a schedule sheet with #REF! cells). Only the instructor can fix the
	// source document.
	GuideContentError = "content-error"
)

// Completion actions the interface offers to finish an item from a guide.
const (
	// GuideActionRecapture: "Ya lo hice": capture only this item again.
	GuideActionRecapture = "recapture"
	// GuideActionUpload: the instructor uploads the evidence themselves.
	GuideActionUpload = "upload"
	// GuideActionRediscover: search the course routes again (a new section).
	GuideActionRediscover = "rediscover"
)

// GuideSignal is what the core detected for one item of a ficha.
type GuideSignal struct {
	ItemCode string
	Kind     string
	// Detected is the plain-words detail of the last capture or review.
	Detected string
	// ZajunaURL is the exact page of the course where the item lives, when
	// the route map knows it; the guide links straight to it.
	ZajunaURL   string
	ZajunaLabel string
	// Slots that still need evidence (empty when the whole item does).
	MissingSlots []int
}

// GuideTemplate is text the instructor can copy into Zajuna.
type GuideTemplate struct {
	Label string `json:"label"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body"`
}

// Guide tells the instructor how to complete an item the app cannot prove by
// itself, and what to hand back so the app finishes the work.
type Guide struct {
	ItemCode      string `json:"itemCode"`
	Description   string `json:"description"`
	CategoryCode  string `json:"categoryCode"`
	CategoryLabel string `json:"categoryLabel"`
	Kind          string `json:"kind"`
	Headline      string `json:"headline"`
	// Requirement is what the guideline asks for, in one sentence.
	Requirement string `json:"requirement"`
	// Location is the navigation path to the item inside the course.
	Location string   `json:"location"`
	Why      string   `json:"why"`
	Detected string   `json:"detected,omitempty"`
	Steps    []string `json:"steps"`
	Handoff  string   `json:"handoff"`
	// EvidenceHint is exactly what to upload if the instructor hands the
	// evidence to Zajuna Sync instead of waiting for a capture.
	EvidenceHint string         `json:"evidenceHint"`
	Actions      []string       `json:"actions"`
	Template     *GuideTemplate `json:"template,omitempty"`
	ZajunaURL    string         `json:"zajunaUrl,omitempty"`
	ZajunaLabel  string         `json:"zajunaLabel,omitempty"`
	MissingSlots []int          `json:"missingSlots,omitempty"`
	// AlsoItems are other items fixed by the same action: the schedule items
	// (1.2.x) share one capture per sheet, so one guide stands for all.
	AlsoItems []string `json:"alsoItems,omitempty"`
}

func kindWhy(kind string) string {
	switch kind {
	case GuideEmptySection:
		return "La subsección existe en Zajuna, pero está vacía. Solo tú puedes subir ese contenido; nosotros no publicamos nada en tu curso."
	case GuideRouteMissing:
		return "No encontramos en el curso la sección o el foro donde vive este ítem. Puede que no exista todavía o que tenga otro nombre."
	case GuideContentError:
		return "La evidencia está en Zajuna, pero el contenido tiene errores que solo tú puedes corregir en el documento original. Nosotros no editamos tus archivos."
	default:
		return "La página está en Zajuna, pero todavía no tiene lo que pide el checklist. Ese contenido depende de ti; nosotros no publicamos nada en tu curso."
	}
}

func kindActions(kind string) []string {
	switch kind {
	case GuideRouteMissing:
		return []string{GuideActionRediscover, GuideActionRecapture, GuideActionUpload}
	default:
		return []string{GuideActionRecapture, GuideActionUpload}
	}
}

// guideSteps adapts the item's steps to why it is out of reach: a missing
// route first needs the section or forum created (or renamed) with the exact
// name, so route discovery can find it.
func guideSteps(content guideContent, kind string) []string {
	switch kind {
	case GuideRouteMissing:
		first := []string{
			"Activa la edición del curso («Activar edición», arriba a la derecha).",
			"Comprueba si ya existe " + content.create + ". Si existe con otro nombre, renómbralo; si no existe, créalo con ese nombre exacto y hazlo visible.",
		}
		return append(first, content.steps...)
	case GuideEmptySection:
		if len(content.emptySteps) > 0 {
			return append([]string(nil), content.emptySteps...)
		}
	case GuideContentError:
		return append([]string(nil), contentErrorSteps...)
	}
	return append([]string(nil), content.steps...)
}

// contentErrorSteps fix a published Google Sheet (the course schedules):
// Zajuna shows the published copy, so the source sheet is what changes.
var contentErrorSteps = []string{
	"Abre el cronograma original en Google Sheets con la cuenta que lo creó (el archivo, no la vista publicada que se ve en Zajuna).",
	"Busca las celdas con error (#REF!, #N/A, #¡VALOR!…): casi siempre son fórmulas que apuntan a una fila o pestaña que se borró.",
	"Corrige la fórmula o escribe el valor correcto (por ejemplo, la fecha de fin de fase) y espera un par de minutos: la hoja publicada en Zajuna se actualiza sola.",
}

// guideHandoff says what the instructor hands back so Zajuna Sync finishes.
func guideHandoff(kind string) string {
	upload := "Si prefieres, pulsa «Subir mi evidencia» y entrégala tú (abajo te decimos qué subir): la revisamos y, si está bien, el ítem queda cumplido."
	if kind == GuideRouteMissing {
		return "Cuando exista en Zajuna, pulsa «Buscar rutas de nuevo» para que la encontremos y después «Ya lo hice, verificar»: capturamos solo este ítem y lo marcamos como cumplido si la evidencia sale bien. " + upload
	}
	return "Cuando lo tengas en Zajuna, pulsa «Ya lo hice, verificar»: capturamos solo este ítem y, si la evidencia sale bien, lo marcamos como cumplido. " + upload
}

func guideEvidenceHint(content guideContent, kind string) string {
	if kind == GuideContentError {
		return "una captura del cronograma ya corregido tal como se ve en Zajuna, sin celdas con error."
	}
	return content.evidenceHint
}

// fallbackContent covers the items the app normally completes by itself
// (cronogramas, profile, course menu…): if one ever needs the instructor, the
// guide is built from its description.
func fallbackContent(item Item) guideContent {
	return guideContent{
		headline:    "Completa en Zajuna: " + item.Description,
		requirement: item.Description + ".",
		steps: []string{
			stepActivarEdicion,
			"Abre la página del curso donde está este ítem («Abrir en Zajuna» te lleva cuando la conocemos).",
			"Agrega o corrige lo que pide el ítem y guarda los cambios.",
		},
		create:       "la sección o el recurso donde vive este ítem, con el nombre del lineamiento",
		evidenceHint: "una captura de Zajuna donde se vea «" + item.Description + "».",
	}
}

// BuildGuide turns a detected signal into the guide shown to the instructor.
// It returns false for an item outside the checklist catalog or an unknown kind.
func BuildGuide(signal GuideSignal) (Guide, bool) {
	code := strings.TrimSpace(signal.ItemCode)
	var item Item
	found := false
	for _, candidate := range Items() {
		if candidate.ItemCode == code {
			item, found = candidate, true
			break
		}
	}
	if !found {
		return Guide{}, false
	}
	switch signal.Kind {
	case GuideContentAbsent, GuideEmptySection, GuideRouteMissing, GuideContentError:
	default:
		return Guide{}, false
	}
	content, ok := itemGuides[code]
	if !ok {
		content = fallbackContent(item)
	}
	if signal.Kind == GuideContentError {
		content.headline = "Corrige los errores del cronograma publicado"
	}
	categoryLabel := ""
	for _, category := range Categories() {
		if category.Code == item.CategoryCode {
			categoryLabel = category.Label
			break
		}
	}
	return Guide{
		ItemCode: code, Description: item.Description, CategoryCode: item.CategoryCode, CategoryLabel: categoryLabel,
		Kind: signal.Kind, Headline: content.headline, Requirement: content.requirement, Location: content.location,
		Why: kindWhy(signal.Kind), Detected: strings.TrimSpace(signal.Detected),
		Steps: guideSteps(content, signal.Kind), Handoff: guideHandoff(signal.Kind), EvidenceHint: guideEvidenceHint(content, signal.Kind),
		Actions: kindActions(signal.Kind), Template: content.template,
		ZajunaURL: strings.TrimSpace(signal.ZajunaURL), ZajunaLabel: strings.TrimSpace(signal.ZajunaLabel),
		MissingSlots: append([]int(nil), signal.MissingSlots...),
	}, true
}

// GuideItemState is the checklist status of one item of the ficha.
type GuideItemState struct {
	ItemCode string
	Status   string
}

// GuideEvidence is one evidence of the ficha as the review sees it.
type GuideEvidence struct {
	ItemCode string
	Slot     int
	Approved bool
	// Superseded: an approved upload of the instructor replaced it.
	Superseded bool
	// EmptySection: the review found a subsection without files or activities.
	EmptySection bool
	// ContentError: what is wrong with the content Zajuna shows that makes
	// this item incorrect (a schedule column it is about has #REF!).
	ContentError string
}

// GuideInput is the live state of a ficha that DetectGuides reads.
type GuideInput struct {
	Items     []GuideItemState
	Evidences []GuideEvidence
	// Absences are the plain-words details of what the LAST capture of each
	// item found missing in Zajuna (a slot whose page loaded without the
	// content the item asks for). An item with an absence is not fulfilled.
	Absences map[string]string
	// MapReady: route discovery ran for the course; only then can a missing
	// route be told apart from a course not searched yet.
	MapReady bool
	Targets  []CaptureTarget
}

// DetectGuides returns, in checklist order, a guide for every item of the
// ficha that is out of the app's reach: not fulfilled yet, without approved
// evidence and with a cause only the instructor can fix in Zajuna. Technical
// failures (a capture that broke, an image too wide) are not included: those
// are the app's job and are retried by capturing again.
func DetectGuides(input GuideInput) []Guide {
	status := make(map[string]string, len(input.Items))
	for _, item := range input.Items {
		status[item.ItemCode] = strings.ToUpper(strings.TrimSpace(item.Status))
	}
	evidences := map[string][]GuideEvidence{}
	for _, entry := range input.Evidences {
		if entry.ItemCode != "" && !entry.Superseded {
			evidences[entry.ItemCode] = append(evidences[entry.ItemCode], entry)
		}
	}
	targets := map[string][]CaptureTarget{}
	for _, target := range input.Targets {
		covered := target.CoveredItemCodes
		if len(covered) == 0 {
			covered = []string{target.ItemCode}
		}
		for _, code := range covered {
			targets[code] = append(targets[code], target)
		}
	}
	guides := []Guide{}
	// One content-error guide per item group: the items share the capture of
	// the same sheet, so fixing it once fixes all of them.
	contentErrorGuide := map[string]int{}
	for _, item := range Items() {
		code := item.ItemCode
		if status[code] == string(StatusYes) {
			continue
		}
		counted := evidences[code]
		allApproved := len(counted) > 0
		emptySlots := []int{}
		errorSlots := []int{}
		contentErrors := []string{}
		for _, entry := range counted {
			if !entry.Approved {
				allApproved = false
				if entry.EmptySection {
					emptySlots = append(emptySlots, max(entry.Slot, 1))
				}
				if message := strings.TrimSpace(entry.ContentError); message != "" {
					errorSlots = append(errorSlots, max(entry.Slot, 1))
					if !containsString(contentErrors, message) {
						contentErrors = append(contentErrors, message)
					}
				}
			}
		}
		// A gap of the last capture (content missing in Zajuna) keeps the item
		// out of reach even when the evidence it has is approved: one forum
		// without dates in 9.1.3 is enough.
		absence := strings.TrimSpace(input.Absences[code])
		if allApproved && absence == "" {
			continue
		}
		signal := GuideSignal{ItemCode: code}
		switch {
		case len(contentErrors) > 0:
			signal.Kind = GuideContentError
			signal.MissingSlots = errorSlots
			signal.Detected = "Última verificación: " + strings.TrimSuffix(strings.Join(contentErrors, "; "), ".") + "."
		case len(emptySlots) > 0:
			signal.Kind = GuideEmptySection
			signal.MissingSlots = emptySlots
			signal.Detected = "La última captura muestra la subsección sin archivos ni actividades."
		case absence != "":
			detail := plainGuideDetail(absence)
			signal.Kind = GuideContentAbsent
			if strings.Contains(detail, "no tiene actividades ni archivos") {
				// The capture found the subsection with only its title.
				signal.Kind = GuideEmptySection
			}
			signal.Detected = "Última verificación: " + detail + "."
		case len(counted) == 0 && input.MapReady && len(targets[code]) == 0 && !selectionBoundItem(code) && !activityBoundItem(code):
			// Activity-bound items without targets wait for the activity
			// selection (a step of the app), not for the instructor's content.
			signal.Kind = GuideRouteMissing
			signal.Detected = "La búsqueda de rutas no encontró dónde está este ítem en el curso."
		default:
			continue
		}
		if target, ok := guideTarget(targets[code], signal.MissingSlots); ok {
			signal.ZajunaURL, signal.ZajunaLabel = target.URL, target.Name
		}
		if signal.Kind == GuideContentError {
			if index, seen := contentErrorGuide[item.GroupName]; seen {
				guides[index].AlsoItems = append(guides[index].AlsoItems, code)
				continue
			}
		}
		if guide, ok := BuildGuide(signal); ok {
			if signal.Kind == GuideContentError {
				contentErrorGuide[item.GroupName] = len(guides)
			}
			guides = append(guides, guide)
		}
	}
	return guides
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

// guideTarget picks the page to open: the target of the first slot that
// needs evidence, or the item's first target.
func guideTarget(targets []CaptureTarget, slots []int) (CaptureTarget, bool) {
	for _, slot := range slots {
		for _, target := range targets {
			if target.SlotNumber == slot && strings.TrimSpace(target.URL) != "" {
				return target, true
			}
		}
	}
	for _, target := range targets {
		if strings.TrimSpace(target.URL) != "" {
			return target, true
		}
	}
	return CaptureTarget{}, false
}

// plainGuideDetail tidies a capture detail for the instructor: the row
// filters name their keywords without accents («conclusion»).
func plainGuideDetail(detail string) string {
	detail = strings.TrimSuffix(strings.TrimSpace(detail), ".")
	return strings.ReplaceAll(detail, "«conclusion»", "«conclusión»")
}
