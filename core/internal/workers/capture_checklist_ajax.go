package workers

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/zajuna"
)

// ajaxContentClient is the part of the Zajuna client that reads course
// structure and forum posts through Moodle's AJAX API. Test fakes that do not
// implement it simply skip the check.
type ajaxContentClient interface {
	CourseState(ctx context.Context, session zajuna.Session, courseID int) (zajuna.CourseState, error)
	ForumUserPosts(ctx context.Context, session zajuna.Session, cmID int) ([]zajuna.ForumUserPost, error)
	ModuleCard(ctx context.Context, session zajuna.Session, cmID int) (string, error)
}

// ajaxAbsencePrefix keeps the wording the capture uses for an absence, so the
// review and the guides read the detail the same way.
const ajaxAbsencePrefix = "sin contenido en Zajuna: "

// ajaxContentCache shares one course state and one list of the instructor's
// posts per forum across the targets of a run (several items point to the
// same course or forum). Entries are keyed by sesskey: a new login never
// reuses another session's answers.
type ajaxContentCache struct {
	mu      sync.Mutex
	courses map[string]*ajaxEntry[zajuna.CourseState]
	forums  map[string]*ajaxEntry[[]zajuna.ForumUserPost]
}

type ajaxEntry[T any] struct {
	once  sync.Once
	value T
	err   error
}

func cachedAJAX[T any](cache *ajaxContentCache, table *map[string]*ajaxEntry[T], key string, load func() (T, error)) (T, error) {
	cache.mu.Lock()
	if *table == nil {
		*table = map[string]*ajaxEntry[T]{}
	}
	entry, ok := (*table)[key]
	if !ok {
		entry = &ajaxEntry[T]{}
		(*table)[key] = entry
		// Bound the cache: a long-running app keeps one worker.
		if len(*table) > 64 {
			*table = map[string]*ajaxEntry[T]{key: entry}
		}
	}
	cache.mu.Unlock()
	entry.once.Do(func() { entry.value, entry.err = load() })
	return entry.value, entry.err
}

var checklistAJAXCache ajaxContentCache

// ajaxContentAbsence asks Zajuna's AJAX API whether the content the item needs
// exists. It returns a plain-words detail and true only when the API
// confirms it is missing; any doubt or failure returns false and the browser
// capture decides as before.
func (w *CaptureChecklistWorker) ajaxContentAbsence(ctx context.Context, params checklistTargetParams) (string, bool) {
	client, ok := w.client.(ajaxContentClient)
	if !ok || !params.Session.HasAJAX() {
		return "", false
	}
	target := params.Target
	kind, sectionTitle := checklist.ContentCheckForItem(target.ItemCode)
	switch kind {
	case checklist.ContentCheckSection:
		courseID := courseIDFromURL(target.URL)
		if courseID <= 0 {
			return "", false
		}
		state, err := cachedAJAX(&checklistAJAXCache, &checklistAJAXCache.courses, params.Session.Sesskey+"|"+strconv.Itoa(courseID), func() (zajuna.CourseState, error) {
			return client.CourseState(ctx, params.Session, courseID)
		})
		if err != nil {
			return "", false
		}
		return sectionAbsence(state, sectionTitle)
	case checklist.ContentCheckForumDates:
		cmID := forumCMIDFromURL(target.URL)
		if cmID <= 0 {
			return "", false
		}
		card, err := client.ModuleCard(ctx, params.Session, cmID)
		if err != nil {
			return "", false
		}
		return forumDatesAbsence(card)
	case checklist.ContentCheckForumReplies, checklist.ContentCheckForumConclusion:
		cmID := forumCMIDFromURL(target.URL)
		if cmID <= 0 {
			return "", false
		}
		posts, err := cachedAJAX(&checklistAJAXCache, &checklistAJAXCache.forums, params.Session.Sesskey+"|"+strconv.Itoa(cmID), func() ([]zajuna.ForumUserPost, error) {
			return client.ForumUserPosts(ctx, params.Session, cmID)
		})
		if err != nil {
			return "", false
		}
		if kind == checklist.ContentCheckForumReplies {
			return forumRepliesAbsence(posts)
		}
		return forumConclusionAbsence(posts)
	}
	return "", false
}

// sectionAbsence: every subsection with the item's title holds no activity
// or file, counting its nested subsections. A title not found is not an
// absence (it may be a route problem the browser reports).
func sectionAbsence(state zajuna.CourseState, title string) (string, bool) {
	sections := state.SectionsWithTitlePrefix(title)
	if len(sections) == 0 {
		return "", false
	}
	for _, section := range sections {
		// A summary (text, a link, an embedded file) is content the browser
		// would capture, so only a provably empty section is an absence.
		if !state.ProvablyEmpty(section.ID) {
			return "", false
		}
	}
	return fmt.Sprintf("%sla sección «%s» no tiene actividades ni archivos (verificado con la API de Zajuna)", ajaxAbsencePrefix, sections[0].Title), true
}

// forumRepliesAbsence: the instructor has not replied to anyone in the forum.
// When there are replies, the browser still checks that the instructor's is
// the last message of each discussion.
func forumRepliesAbsence(posts []zajuna.ForumUserPost) (string, bool) {
	for _, post := range posts {
		if post.IsReply {
			return "", false
		}
	}
	return ajaxAbsencePrefix + "el foro no tiene respuestas tuyas a los aprendices (verificado con la API de Zajuna)", true
}

// forumConclusionAbsence: no post of the instructor, new discussion or reply,
// mentions a conclusion.
func forumConclusionAbsence(posts []zajuna.ForumUserPost) (string, bool) {
	for _, post := range posts {
		if strings.Contains(zajuna.FoldText(post.Subject+" "+post.DiscussionName), "conclusion") {
			return "", false
		}
	}
	return ajaxAbsencePrefix + "el foro no tiene ninguna publicación tuya con «conclusión» en el asunto (verificado con la API de Zajuna)", true
}

// moduleDatePattern finds the dates Moodle prints on an activity card, with
// the same labels the browser rule accepts (checklist.ForumDateLabels).
var moduleDatePattern = regexp.MustCompile(`(?i)` + checklist.ForumDateLabels + `\s*:`)

// moduleDatesMarker: Moodle wraps an activity's dates in this region; the
// browser rule accepts it whatever the labels say.
var moduleDatesMarker = regexp.MustCompile(`data-region=["']activity-dates["']|class=["'][^"']*\bactivity-dates\b`)

// forumDatesAbsence: the forum card shows no date at all, so the forum has no
// start or end configured (9.1.3, 9.1.4). Anything the browser rule would
// accept is not an absence.
func forumDatesAbsence(card string) (string, bool) {
	if strings.TrimSpace(card) == "" || moduleDatesMarker.MatchString(card) || moduleDatePattern.MatchString(card) {
		return "", false
	}
	return ajaxAbsencePrefix + "el foro no tiene fechas de apertura ni de cierre configuradas (verificado con la API de Zajuna)", true
}

func courseIDFromURL(raw string) int {
	parsed, err := url.Parse(raw)
	if err != nil || !strings.HasSuffix(strings.ToLower(parsed.Path), "/course/view.php") {
		return 0
	}
	id, _ := strconv.Atoi(parsed.Query().Get("id"))
	return id
}

func forumCMIDFromURL(raw string) int {
	parsed, err := url.Parse(raw)
	if err != nil || !strings.HasSuffix(strings.ToLower(parsed.Path), "/mod/forum/view.php") {
		return 0
	}
	id, _ := strconv.Atoi(parsed.Query().Get("id"))
	return id
}
