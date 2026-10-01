package checklist

// Content checks the capture runs through Zajuna's AJAX API before opening
// the browser. They only decide that the content an item needs is NOT in
// Zajuna (an empty subsection, a forum without the instructor's replies or
// conclusion); any other answer, or an API failure, leaves the decision to
// the browser capture as before.
const (
	ContentCheckSection         = "section"
	ContentCheckForumReplies    = "forum-replies"
	ContentCheckForumConclusion = "forum-conclusion"
	ContentCheckForumDates      = "forum-dates"
	// ContentCheckGradeItems: the gradebook has no grade item (5.1).
	ContentCheckGradeItems = "grade-items"
)

// ContentCheckForItem returns the AJAX check for an item and, for a section
// check, the title its subsection starts with.
func ContentCheckForItem(itemCode string) (kind string, sectionTitle string) {
	switch SemanticCheckForItem(itemCode) {
	case SemanticForumReplies:
		return ContentCheckForumReplies, ""
	case SemanticForumConclusion:
		return ContentCheckForumConclusion, ""
	case SemanticForumDates:
		return ContentCheckForumDates, ""
	}
	switch itemCode {
	case "5.1":
		return ContentCheckGradeItems, ""
	}
	// Only named subsections: the parent sections ("Seguimiento y
	// Evaluación", "Sesiones en línea") prove other things (7.1.x asks the
	// section to be hidden, 8.x its organisation), not that files exist.
	if title := courseSectionTitleForItem(itemCode); title != "" && title != seguimientoSectionTitle && title != sesionesSectionTitle {
		return ContentCheckSection, title
	}
	return "", ""
}

// IsScheduleItem tells the course schedule items (1.x): their evidence is a
// published Google Sheet whose cells the capture also checks for errors.
func IsScheduleItem(itemCode string) bool {
	for _, item := range Items() {
		if item.ItemCode == itemCode {
			return item.GroupName == "cronograma_general" || item.GroupName == "cronograma_vigente"
		}
	}
	return false
}

// GradingInsightItem tells the items (10.1.x) whose absence message is
// completed with what Zajuna reports about grading.
func GradingInsightItem(itemCode string) bool {
	return itemCode == "10.1.1" || itemCode == "10.1.2"
}

// courseSectionNames are the full names of the subsections the guideline
// asks for, by the title prefix used to find them.
var courseSectionNames = map[string]string{
	"Reporte del Curso":      "Reporte del Curso",
	comitesSectionTitle:      "Comités evaluativos - Actas",
	"Documentos de retenci":  "Documentos de retención",
	"Reuniones EEF":          "Reuniones EEF - Actas",
	"Planes de Mejoramiento": "Planes de Mejoramiento",
	"Registro de Novedades":  "Registro de Novedades",
	"Llamados de atenci":     "Llamados de atención",
}

// CourseSectionName is the full name of a subsection the guideline asks for.
func CourseSectionName(titlePrefix string) string {
	if name, ok := courseSectionNames[titlePrefix]; ok {
		return name
	}
	return titlePrefix
}
