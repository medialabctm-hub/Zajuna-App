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
	// Only named subsections: the parent sections ("Seguimiento y
	// Evaluación", "Sesiones en línea") prove other things (7.1.x asks the
	// section to be hidden, 8.x its organisation), not that files exist.
	if title := courseSectionTitleForItem(itemCode); title != "" && title != seguimientoSectionTitle && title != sesionesSectionTitle {
		return ContentCheckSection, title
	}
	return "", ""
}
