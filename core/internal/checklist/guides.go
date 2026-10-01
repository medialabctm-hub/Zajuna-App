package checklist

import (
	"fmt"
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
	ItemCode      string         `json:"itemCode"`
	Description   string         `json:"description"`
	CategoryCode  string         `json:"categoryCode"`
	CategoryLabel string         `json:"categoryLabel"`
	Kind          string         `json:"kind"`
	Headline      string         `json:"headline"`
	Why           string         `json:"why"`
	Detected      string         `json:"detected,omitempty"`
	Steps         []string       `json:"steps"`
	Handoff       string         `json:"handoff"`
	Actions       []string       `json:"actions"`
	Template      *GuideTemplate `json:"template,omitempty"`
	ZajunaURL     string         `json:"zajunaUrl,omitempty"`
	ZajunaLabel   string         `json:"zajunaLabel,omitempty"`
	MissingSlots  []int          `json:"missingSlots,omitempty"`
}

// guideContent is the hand-written part of a guide.
type guideContent struct {
	headline string
	steps    []string
	handoff  string
	template *GuideTemplate
}

const (
	handoffRecapture = "Cuando lo hayas publicado en Zajuna, pulsa «Ya lo hice, verificar». Capturamos solo este ítem y, si la evidencia sale bien, lo marcamos como cumplido."
	handoffDocument  = "Súbelo a Zajuna y pulsa «Ya lo hice, verificar». Si prefieres, sube aquí una captura o el PDF. Lo revisamos y, si está bien, el ítem queda cumplido."
)

var conclusionTemplate = &GuideTemplate{
	Label: "Texto sugerido para la conclusión",
	Title: "Conclusión del foro temático: [nombre del foro]",
	Body: "Apreciados aprendices:\n\n" +
		"Agradezco sus aportes en este foro. A partir de sus participaciones destaco:\n" +
		"1. [Idea principal 1]\n" +
		"2. [Idea principal 2]\n" +
		"3. [Idea principal 3]\n\n" +
		"Conclusión: [síntesis del tema y relación con la actividad de proyecto].\n\n" +
		"Cordialmente,\n[Nombre del instructor]",
}

// itemGuides holds the items with specific instructions; the rest fall back to
// their group and then to the generic text of their kind.
var itemGuides = map[string]guideContent{
	"9.1.1": {
		headline: "Nombra el foro de dudas según el lineamiento",
		steps: []string{
			"En la sección de foros del curso, abre el foro de dudas y pulsa «Editar ajustes».",
			"Cambia el nombre a «Foro de Dudas e Inquietudes» y guarda los cambios.",
		},
		handoff: handoffRecapture,
	},
	"9.1.2": {
		headline: "Crea al menos un Foro de Dudas e Inquietudes",
		steps: []string{
			"Activa la edición en el curso de Zajuna.",
			"En la sección de foros, pulsa «Añadir una actividad o un recurso» y elige «Foro».",
			"Nómbralo «Foro de Dudas e Inquietudes» y guarda los cambios.",
		},
		handoff: "Cuando exista, pulsa «Buscar rutas de nuevo» para que encontremos el foro, y después «Ya lo hice, verificar».",
	},
	"9.1.3": {
		headline: "Configura las fechas del foro temático",
		steps: []string{
			"Abre el foro temático y entra en «Editar ajustes».",
			"En «Disponibilidad», activa y completa la fecha de entrega o de vencimiento y la fecha límite.",
			"Guarda los cambios y comprueba que las fechas se ven en la página del foro.",
		},
		handoff: handoffRecapture,
	},
	"9.1.4": {
		headline: "Abre el foro temático en las fechas del cronograma",
		steps: []string{
			"Revisa en el cronograma de la fase las fechas del foro temático.",
			"En «Editar ajustes» del foro, haz que la disponibilidad coincida con esas fechas y que el foro sea visible.",
			"Guarda los cambios.",
		},
		handoff: handoffRecapture,
	},
	"9.1.5": {
		headline: "Responde las dudas del foro de dudas",
		steps: []string{
			"Abre el Foro de Dudas e Inquietudes.",
			"Responde cada debate de los aprendices con «Responder». La respuesta debe ser el último mensaje del debate.",
			"Hazlo en máximo un día hábil desde que el aprendiz publica.",
		},
		handoff: handoffRecapture,
	},
	"9.1.6": {
		headline: "Responde a los aprendices en los foros temáticos",
		steps: []string{
			"Abre el foro temático de la actividad.",
			"Responde a las participaciones de los aprendices con «Responder». Tu mensaje debe quedar como el último de cada debate.",
			"Hazlo en máximo un día hábil.",
		},
		handoff: handoffRecapture,
	},
	"9.1.7": {
		headline: "Publica la retroalimentación de los foros temáticos",
		steps: []string{
			"Abre el foro temático de la actividad.",
			"Responde a cada aprendiz con retroalimentación sobre su aporte: qué hizo bien y qué puede mejorar.",
			"Comprueba que tu respuesta quede como último mensaje del debate.",
		},
		handoff: handoffRecapture,
	},
	"14.1.1": {
		headline: "Publica la conclusión del foro temático",
		steps: []string{
			"Abre el foro temático cuando termine según el cronograma (o al día siguiente).",
			"Pulsa «Añadir un nuevo tema de debate».",
			"Pon un asunto que contenga la palabra «Conclusión» y pega el texto sugerido completado.",
			"Pulsa «Enviar al foro».",
		},
		handoff:  handoffRecapture,
		template: conclusionTemplate,
	},
	"14.1.2": {
		headline: "Publica la conclusión como un debate nuevo",
		steps: []string{
			"Abre el foro temático y pulsa «Añadir un nuevo tema de debate». No la publiques como respuesta dentro de otro debate.",
			"El asunto debe contener la palabra «Conclusión», por ejemplo «Conclusión del foro temático».",
			"Pega el texto sugerido completado y pulsa «Enviar al foro».",
		},
		handoff:  handoffRecapture,
		template: conclusionTemplate,
	},
	"7.3.2": {
		headline: "Sube un documento de retención",
		steps: []string{
			"Activa la edición en el curso y abre Seguimiento y Evaluación → Seguimiento a la Formación → Documentos de retención.",
			"Pulsa «Añadir una actividad o un recurso», elige «Archivo» y sube el documento (PDF).",
			"Guarda los cambios.",
		},
		handoff: handoffDocument,
	},
	"7.3.3": {
		headline: "Sube las actas de las reuniones EEF",
		steps: []string{
			"Activa la edición en el curso y abre Seguimiento y Evaluación → Seguimiento a la Formación → Reuniones EEF – Actas.",
			"Pulsa «Añadir una actividad o un recurso», elige «Archivo» y sube el acta (PDF).",
			"Guarda los cambios.",
		},
		handoff: handoffDocument,
	},
	"13.1.1": {
		headline: "Publica 2 actas mensuales de reuniones EEF",
		steps: []string{
			"Abre la subsección Reuniones EEF – Actas del curso.",
			"Sube como «Archivo» las 2 actas del mes, con un nombre que indique la fecha (por ejemplo «Acta EEF 2026-09-15»).",
			"Guarda los cambios.",
		},
		handoff: handoffDocument,
	},
	"13.1.2": {
		headline: "Publica el acta de comité al terminar la fase",
		steps: []string{
			"Abre la subsección Comités evaluativos – Actas.",
			"Sube como «Archivo» al menos un acta de comité por fase terminada.",
			"Guarda los cambios.",
		},
		handoff: handoffDocument,
	},
	"13.1.3": {
		headline: "Publica un documento en Documentos de retención",
		steps: []string{
			"Abre la subsección Documentos de retención del curso.",
			"Sube como «Archivo» al menos un documento de retención.",
			"Guarda los cambios.",
		},
		handoff: handoffDocument,
	},
	"10.1.1": {
		headline: "Califica y retroalimenta las evidencias entregadas",
		steps: []string{
			"Abre la actividad de evidencia y pulsa «Ver todas las entregas».",
			"Califica cada entrega y escribe un comentario de retroalimentación.",
			"Guarda. La entrega debe quedar en estado «Calificado».",
		},
		handoff: handoffRecapture,
	},
	"10.1.2": {
		headline: "Retroalimenta las evidencias en máximo tres días hábiles",
		steps: []string{
			"Abre la actividad y revisa las entregas que no tienen calificación.",
			"Califica y retroalimenta cada una en máximo tres días hábiles desde la entrega.",
		},
		handoff: handoffRecapture,
	},
	"11.2.3": {
		headline: "Publica el anuncio semanal de la sesión en línea",
		steps: []string{
			"Abre el foro de Anuncios del curso y pulsa «Añadir un nuevo tema».",
			"Indica fecha, hora y enlace de la sesión en línea de la semana.",
			"Pulsa «Enviar al foro».",
		},
		handoff: handoffRecapture,
		template: &GuideTemplate{
			Label: "Texto sugerido para el anuncio",
			Title: "Sesión en línea de la semana — [fecha]",
			Body:  "Apreciados aprendices:\n\nLos invito a la sesión en línea de esta semana.\nFecha: [día y fecha]\nHora: [hora]\nEnlace: [enlace de la sesión]\nTema: [tema de la sesión]\n\nCordialmente,\n[Nombre del instructor]",
		},
	},
	"11.3": {
		headline: "Publica el anuncio de aprendices aprobados de la fase",
		steps: []string{
			"Abre el foro de Anuncios y pulsa «Añadir un nuevo tema».",
			"Publica la lista de aprendices que aprobaron la fase.",
			"Pulsa «Enviar al foro».",
		},
		handoff: handoffRecapture,
	},
}

var groupGuides = map[string]guideContent{
	"foros": {
		headline: "Completa el foro en Zajuna",
		steps: []string{
			"Abre el foro correspondiente del curso.",
			"Publica o responde lo que pide el ítem. Tu mensaje debe quedar visible en la lista de debates.",
		},
		handoff: handoffRecapture,
	},
	"conclusion_foros": {
		headline: "Publica la conclusión del foro temático",
		steps: []string{
			"Abre el foro temático y pulsa «Añadir un nuevo tema de debate».",
			"El asunto debe contener la palabra «Conclusión».",
		},
		handoff:  handoffRecapture,
		template: conclusionTemplate,
	},
	"seguimiento_documentos": {
		headline: "Sube el documento a la subsección",
		steps: []string{
			"Activa la edición y abre la subsección de Seguimiento y Evaluación que indica el ítem.",
			"Pulsa «Añadir una actividad o un recurso», elige «Archivo» y sube el documento (PDF).",
		},
		handoff: handoffDocument,
	},
	"documentos_retencion": {
		headline: "Sube los documentos de seguimiento",
		steps: []string{
			"Activa la edición y abre la subsección que indica el ítem.",
			"Sube como «Archivo» los documentos que pide el lineamiento.",
		},
		handoff: handoffDocument,
	},
	"sesiones_semanales": {
		headline: "Publica la grabación y el resumen de la sesión",
		steps: []string{
			"Abre la subsección de Sesiones en línea del mes correspondiente.",
			"Añade la grabación (enlace o «URL») y el resumen de la sesión de la semana que falta.",
		},
		handoff: handoffRecapture,
	},
	"anuncios_fase": {
		headline: "Publica el anuncio de inicio de fase",
		steps: []string{
			"Abre el foro de Anuncios y pulsa «Añadir un nuevo tema».",
			"Incluye el nombre de la fase, sus fechas de inicio y fin, qué consultar y los pasos a seguir.",
		},
		handoff: handoffRecapture,
	},
	"anuncios_semanales": {
		headline: "Publica el anuncio en el foro de Anuncios",
		steps: []string{
			"Abre el foro de Anuncios del curso y pulsa «Añadir un nuevo tema».",
			"Redacta el anuncio que pide el ítem y pulsa «Enviar al foro».",
		},
		handoff: handoffRecapture,
	},
	"evidencias_aprendizaje": {
		headline: "Califica las entregas con retroalimentación",
		steps: []string{
			"Abre la actividad y pulsa «Ver todas las entregas».",
			"Califica y escribe la retroalimentación de cada entrega.",
		},
		handoff: handoffRecapture,
	},
}

func kindWhy(kind string) string {
	switch kind {
	case GuideEmptySection:
		return "La subsección existe en Zajuna, pero está vacía. Solo tú puedes subir ese contenido; nosotros no publicamos nada en tu curso."
	case GuideRouteMissing:
		return "No encontramos en el curso la sección o el foro donde vive este ítem. Puede que no exista todavía o que tenga otro nombre."
	default:
		return "La página está en Zajuna, pero todavía no tiene lo que pide el checklist. Ese contenido depende de ti; nosotros no publicamos nada en tu curso."
	}
}

func kindFallback(kind, description string) guideContent {
	switch kind {
	case GuideEmptySection:
		return guideContent{
			headline: "Agrega el contenido de la subsección",
			steps: []string{
				"Activa la edición en el curso y abre la subsección indicada.",
				fmt.Sprintf("Agrega lo que pide el ítem: «%s».", description),
			},
			handoff: handoffDocument,
		}
	case GuideRouteMissing:
		return guideContent{
			headline: "Crea la sección o el recurso en el curso",
			steps: []string{
				fmt.Sprintf("Revisa en Zajuna dónde debería estar: «%s».", description),
				"Si no existe, créalo con el nombre del lineamiento. Si existe con otro nombre, renómbralo.",
			},
			handoff: "Después pulsa «Buscar rutas de nuevo» y «Ya lo hice, verificar». También puedes subir aquí una captura tuya.",
		}
	default:
		return guideContent{
			headline: "Completa el contenido en Zajuna",
			steps: []string{
				fmt.Sprintf("Abre la página indicada y agrega lo que pide el ítem: «%s».", description),
			},
			handoff: handoffRecapture,
		}
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
	case GuideContentAbsent, GuideEmptySection, GuideRouteMissing:
	default:
		return Guide{}, false
	}
	content, ok := itemGuides[code]
	if !ok {
		content, ok = groupGuides[item.GroupName]
	}
	if !ok {
		content = kindFallback(signal.Kind, item.Description)
	}
	categoryLabel := ""
	for _, category := range Categories() {
		if category.Code == item.CategoryCode {
			categoryLabel = category.Label
			break
		}
	}
	steps := append([]string(nil), content.steps...)
	if signal.Kind == GuideRouteMissing {
		// A curated step list assumes the page exists; first it must be found.
		steps = append([]string{"Comprueba que la sección o el foro exista en el curso con el nombre del lineamiento."}, steps...)
	}
	return Guide{
		ItemCode: code, Description: item.Description, CategoryCode: item.CategoryCode, CategoryLabel: categoryLabel,
		Kind: signal.Kind, Headline: content.headline, Why: kindWhy(signal.Kind), Detected: strings.TrimSpace(signal.Detected),
		Steps: steps, Handoff: content.handoff, Actions: kindActions(signal.Kind), Template: content.template,
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
}

// GuideInput is the live state of a ficha that DetectGuides reads.
type GuideInput struct {
	Items     []GuideItemState
	Evidences []GuideEvidence
	// Absences are the plain-words details of the last captures that found
	// the page but not the content ("sin contenido en Zajuna"), per item.
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
	for _, item := range Items() {
		code := item.ItemCode
		if status[code] == string(StatusYes) {
			continue
		}
		counted := evidences[code]
		allApproved := len(counted) > 0
		emptySlots := []int{}
		for _, entry := range counted {
			if !entry.Approved {
				allApproved = false
				if entry.EmptySection {
					emptySlots = append(emptySlots, max(entry.Slot, 1))
				}
			}
		}
		if allApproved {
			continue
		}
		signal := GuideSignal{ItemCode: code}
		switch {
		case len(emptySlots) > 0:
			signal.Kind = GuideEmptySection
			signal.MissingSlots = emptySlots
			signal.Detected = "La última captura muestra la subsección sin archivos ni actividades."
		case len(counted) == 0 && strings.TrimSpace(input.Absences[code]) != "":
			detail := plainGuideDetail(input.Absences[code])
			signal.Kind = GuideContentAbsent
			if strings.Contains(detail, "no tiene actividades ni archivos") {
				// The capture found the subsection with only its title.
				signal.Kind = GuideEmptySection
			}
			signal.Detected = "Última captura: " + detail + "."
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
		if guide, ok := BuildGuide(signal); ok {
			guides = append(guides, guide)
		}
	}
	return guides
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
