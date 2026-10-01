package workers

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/zajuna"
)

// Optional session-only sources the capture uses when the client offers them
// (the real Zajuna client does; test fakes may not). None of them can make a
// capture fail: an error just leaves the result as it was.

type gradeItemsClient interface {
	GradeItemCount(ctx context.Context, session zajuna.Session, courseID int) (int, error)
}

type gradingInsightClient interface {
	PendingGradings(ctx context.Context, session zajuna.Session, courseID int) ([]zajuna.PendingGrading, error)
	GradeHistory(ctx context.Context, session zajuna.Session, courseID int) ([]zajuna.GradeHistoryEntry, error)
}

type forumExportClient interface {
	ForumInstanceID(ctx context.Context, session zajuna.Session, cmID int) (int, error)
	ExportForum(ctx context.Context, session zajuna.Session, forumID int) ([]zajuna.ForumPostRecord, error)
}

type forumIndexClient interface {
	ForumIndex(ctx context.Context, session zajuna.Session, courseID int) ([]zajuna.ForumIndexEntry, error)
}

type pageClient interface {
	GetPage(ctx context.Context, session zajuna.Session, path string) (string, error)
}

// fetchSheet is a variable so tests do not reach docs.google.com.
var fetchSheet = zajuna.FetchPublishedSheet

// gradeItemsAbsence: the gradebook has no grade item, so no activity is
// associated in Calificaciones (5.1).
func gradeItemsAbsence(ctx context.Context, client any, session zajuna.Session, targetURL string) (string, bool) {
	grades, ok := client.(gradeItemsClient)
	if !ok {
		return "", false
	}
	courseID := courseIDFromAnyURL(targetURL)
	if courseID <= 0 {
		return "", false
	}
	count, err := cachedAJAX(&checklistAJAXCache, &checklistAJAXCache.items, session.Sesskey+"|"+strconv.Itoa(courseID), func() (int, error) {
		return grades.GradeItemCount(ctx, session, courseID)
	})
	if err != nil || count > 0 {
		return "", false
	}
	return ajaxAbsencePrefix + "el calificador del curso no tiene ninguna actividad asociada (verificado con la API de Zajuna)", true
}

// 6.1 (due dates) has no AJAX check: the course card only prints dates when
// the course enables «Mostrar fechas de actividad», so a card without dates
// does not prove a missing due date and the browser keeps deciding.

// scheduleSheetIssues reads the published Google Sheets of a schedule page
// (1.x) and returns what is wrong with them (#REF! cells, an empty sheet).
func scheduleSheetIssues(ctx context.Context, client any, session zajuna.Session, target checklist.CaptureTarget) []string {
	pages, ok := client.(pageClient)
	if !ok || !checklist.IsScheduleItem(target.ItemCode) {
		return nil
	}
	parsed, err := url.Parse(target.URL)
	// Only a schedule's own activity page: the course page embeds every
	// sheet of the course and would mix their errors.
	if err != nil || !strings.Contains(strings.ToLower(parsed.Path), "/mod/") {
		return nil
	}
	issues, _ := cachedAJAX(&checklistAJAXCache, &checklistAJAXCache.sheets, session.Sesskey+"|"+target.URL, func() ([]string, error) {
		body, pageErr := pages.GetPage(ctx, session, parsed.RequestURI())
		if pageErr != nil {
			return nil, pageErr
		}
		found := []string{}
		for _, csvURL := range zajuna.PublishedSheetCSVURLs(body) {
			rows, sheetErr := fetchSheet(ctx, csvURL)
			if sheetErr != nil {
				continue
			}
			found = append(found, zajuna.SheetIssues(rows)...)
		}
		return found, nil
	})
	return issues
}

// absenceInsight completes an absence with what Zajuna reports through the
// session: learners' posts still waiting for the instructor (9.1.5–9.1.7)
// or submissions waiting to be graded (10.1.x). It never changes the
// decision, only the explanation shown in the guide.
func (w *CaptureChecklistWorker) absenceInsight(ctx context.Context, params checklistTargetParams) string {
	target := params.Target
	switch {
	case checklist.SemanticCheckForItem(target.ItemCode) == checklist.SemanticForumReplies:
		exporter, ok := w.client.(forumExportClient)
		cmID := forumCMIDFromURL(target.URL)
		if !ok || cmID <= 0 || params.Session.UserID <= 0 {
			return ""
		}
		forumID, err := cachedAJAX(&checklistAJAXCache, &checklistAJAXCache.forumIDs, params.Session.Sesskey+"|"+strconv.Itoa(cmID), func() (int, error) {
			return exporter.ForumInstanceID(ctx, params.Session, cmID)
		})
		if err != nil {
			if forumID = w.forumIDFromIndex(ctx, params, cmID); forumID <= 0 {
				return ""
			}
		}
		posts, err := cachedAJAX(&checklistAJAXCache, &checklistAJAXCache.exports, params.Session.Sesskey+"|"+strconv.Itoa(forumID), func() ([]zajuna.ForumPostRecord, error) {
			return exporter.ExportForum(ctx, params.Session, forumID)
		})
		if err != nil {
			return ""
		}
		count, oldest := zajuna.UnansweredPosts(posts, params.Session.UserID)
		if count == 0 {
			return ""
		}
		return fmt.Sprintf("; hay %d mensajes sin respuesta tuya en el foro, el más antiguo del %s", count, time.Unix(oldest, 0).Format("02/01/2006"))
	case checklist.GradingInsightItem(target.ItemCode):
		insight, ok := w.client.(gradingInsightClient)
		courseID := w.courseIDForFicha(ctx, params.Input.FichaID)
		cmID, _ := strconv.Atoi(strings.TrimSpace(target.ActivityID))
		if !ok || courseID <= 0 {
			return ""
		}
		parts := []string{}
		key := params.Session.Sesskey + "|" + strconv.Itoa(courseID)
		pending, pendingErr := cachedAJAX(&checklistAJAXCache, &checklistAJAXCache.pending, key, func() ([]zajuna.PendingGrading, error) {
			return insight.PendingGradings(ctx, params.Session, courseID)
		})
		if pendingErr == nil {
			for _, entry := range pending {
				if entry.CMID == cmID && entry.Count > 0 {
					parts = append(parts, fmt.Sprintf("Zajuna indica %d entregas por calificar en esta actividad", entry.Count))
				}
			}
		}
		if title := zajuna.FoldText(target.ActivityTitle); title != "" {
			history, historyErr := cachedAJAX(&checklistAJAXCache, &checklistAJAXCache.history, key, func() ([]zajuna.GradeHistoryEntry, error) {
				return insight.GradeHistory(ctx, params.Session, courseID)
			})
			if historyErr == nil {
				graded, feedback := 0, 0
				for _, entry := range history {
					if zajuna.FoldText(entry.Item) != title {
						continue
					}
					if entry.Graded {
						graded++
					}
					if entry.HasFeedback {
						feedback++
					}
				}
				parts = append(parts, fmt.Sprintf("el historial registra %d calificaciones y %d retroalimentaciones para ella", graded, feedback))
			}
		}
		if len(parts) == 0 {
			return ""
		}
		return "; " + strings.Join(parts, "; ")
	}
	return ""
}

// courseIDFromAnyURL reads the course id of course and gradebook pages
// (course/view.php?id=, grade/…?id=, mod/assign/view.php has a cm id instead).
func courseIDFromAnyURL(raw string) int {
	parsed, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	path := strings.ToLower(parsed.Path)
	if !strings.HasSuffix(path, "/course/view.php") && !strings.Contains(path, "/grade/") {
		if id, convErr := strconv.Atoi(parsed.Query().Get("courseid")); convErr == nil {
			return id
		}
		return 0
	}
	id, _ := strconv.Atoi(parsed.Query().Get("id"))
	return id
}

// courseIDForFicha reads the Moodle course id of a ficha (the grading page of
// an activity carries the activity's id, not the course's).
func (w *CaptureChecklistWorker) courseIDForFicha(ctx context.Context, fichaID string) int {
	if w.fichaStore == nil {
		return 0
	}
	ficha, err := w.fichaStore.GetFicha(ctx, fichaID)
	if err != nil {
		return 0
	}
	id, _ := strconv.Atoi(strings.TrimSpace(ficha.CourseID))
	return id
}

// forumIDFromIndex finds a forum's instance id in the course's forum index
// (/mod/forum/index.php) by the activity name the course state reports for
// the course-module id. Used when the forum page does not expose its id.
func (w *CaptureChecklistWorker) forumIDFromIndex(ctx context.Context, params checklistTargetParams, cmID int) int {
	index, okIndex := w.client.(forumIndexClient)
	states, okState := w.client.(ajaxContentClient)
	courseID := w.courseIDForFicha(ctx, params.Input.FichaID)
	if !okIndex || !okState || courseID <= 0 {
		return 0
	}
	state, err := states.CourseState(ctx, params.Session, courseID)
	if err != nil {
		return 0
	}
	name := ""
	for _, module := range state.Modules {
		if string(module.ID) == strconv.Itoa(cmID) {
			name = zajuna.FoldText(module.Name)
			break
		}
	}
	if name == "" {
		return 0
	}
	entries, err := index.ForumIndex(ctx, params.Session, courseID)
	if err != nil {
		return 0
	}
	match := 0
	for _, entry := range entries {
		if zajuna.FoldText(entry.Name) == name {
			if match != 0 {
				return 0 // two forums with the same name: ambiguous
			}
			match = entry.ForumID
		}
	}
	return match
}
