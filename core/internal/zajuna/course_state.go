package zajuna

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CourseSection is one section of the course as core_courseformat_get_state
// reports it. Zajuna nests sections (a "Seguimiento a la Formación" section
// holds "Documentos de retención…"), so a section's content is its own
// activities plus those of its children.
type CourseSection struct {
	ID       string
	Number   int
	Title    string
	ParentID string
	Children []string
	Modules  []string
	Visible  bool
	// HasSummary is nil when Moodle did not say; a summary (text, a link, an
	// embedded file) is content the browser would capture.
	HasSummary *bool
}

// CourseModule is one activity or resource of the course.
type CourseModule struct {
	ID        flexibleID   `json:"id"`
	Name      string       `json:"name"`
	Module    string       `json:"module"`
	SectionID flexibleID   `json:"sectionid"`
	Visible   flexibleBool `json:"visible"`
}

// CourseState is the structure of a course: sections and activities.
type CourseState struct {
	Sections []CourseSection
	Modules  []CourseModule
	byID     map[string]int
}

// flexibleID accepts the ids Moodle sends as strings or numbers.
type flexibleID string

func (id *flexibleID) UnmarshalJSON(data []byte) error {
	text := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if text == "null" {
		text = ""
	}
	*id = flexibleID(text)
	return nil
}

// flexibleBool accepts true/false, 1/0 and "1"/"0".
type flexibleBool bool

func (value *flexibleBool) UnmarshalJSON(data []byte) error {
	switch strings.Trim(strings.TrimSpace(string(data)), `"`) {
	case "true", "1":
		*value = true
	default:
		*value = false
	}
	return nil
}

type rawCourseState struct {
	CM      []CourseModule `json:"cm"`
	Section []struct {
		ID         flexibleID        `json:"id"`
		Number     int               `json:"number"`
		Title      string            `json:"title"`
		ParentID   flexibleID        `json:"parentid"`
		Visible    flexibleBool      `json:"visible"`
		HasSummary *flexibleBool     `json:"hassummary"`
		CMList     []flexibleID      `json:"cmlist"`
		Children   []json.RawMessage `json:"children"`
	} `json:"section"`
}

// CourseState reads the structure of a course through AJAX
// (core_courseformat_get_state, the call behind the course index).
func (c *Client) CourseState(ctx context.Context, session Session, courseID int) (CourseState, error) {
	if courseID <= 0 {
		return CourseState{}, fmt.Errorf("%w: curso inválido", ErrAJAXUnavailable)
	}
	var encoded string
	if err := c.CallAJAX(ctx, session, "core_courseformat_get_state", map[string]any{"courseid": courseID}, &encoded); err != nil {
		return CourseState{}, err
	}
	return ParseCourseState(encoded)
}

// ParseCourseState decodes the JSON text core_courseformat_get_state returns.
func ParseCourseState(encoded string) (CourseState, error) {
	var raw rawCourseState
	if err := json.Unmarshal([]byte(encoded), &raw); err != nil {
		return CourseState{}, fmt.Errorf("%w: estado del curso ilegible: %v", ErrAJAXUnavailable, err)
	}
	state := CourseState{Modules: raw.CM, byID: map[string]int{}}
	for _, section := range raw.Section {
		entry := CourseSection{ID: string(section.ID), Number: section.Number, Title: strings.TrimSpace(section.Title), ParentID: string(section.ParentID), Visible: bool(section.Visible)}
		if section.HasSummary != nil {
			hasSummary := bool(*section.HasSummary)
			entry.HasSummary = &hasSummary
		}
		for _, module := range section.CMList {
			entry.Modules = append(entry.Modules, string(module))
		}
		for _, child := range section.Children {
			// Children come as ids or as {"id": …} objects depending on the format.
			var object struct {
				ID flexibleID `json:"id"`
			}
			var id flexibleID
			if json.Unmarshal(child, &object) == nil && object.ID != "" {
				id = object.ID
			} else if json.Unmarshal(child, &id) != nil {
				continue
			}
			if id != "" {
				entry.Children = appendUnique(entry.Children, string(id))
			}
		}
		state.byID[entry.ID] = len(state.Sections)
		state.Sections = append(state.Sections, entry)
	}
	if len(state.Sections) == 0 {
		return CourseState{}, fmt.Errorf("%w: el curso no trae secciones", ErrAJAXUnavailable)
	}
	// The children list is not the only source: a format that omits it still
	// sends each subsection's parentid. Both are merged so a parent whose
	// files live in a subsection never looks empty.
	for _, section := range state.Sections {
		if section.ParentID == "" || section.ParentID == "0" || section.ParentID == section.ID {
			continue
		}
		if parent, ok := state.byID[section.ParentID]; ok {
			state.Sections[parent].Children = appendUnique(state.Sections[parent].Children, section.ID)
		}
	}
	return state, nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// SectionsWithTitlePrefix returns the sections whose title starts with the
// prefix, ignoring case and accents ("Documentos de retenci" matches
// "Documentos de retención de aprendices").
func (s CourseState) SectionsWithTitlePrefix(prefix string) []CourseSection {
	wanted := titleKey(prefix)
	if wanted == "" {
		return nil
	}
	result := []CourseSection{}
	for _, section := range s.Sections {
		if strings.HasPrefix(titleKey(section.Title), wanted) {
			result = append(result, section)
		}
	}
	return result
}

// titleKeyStopWords vary between courses («Reporte de curso», «Reporte del
// Curso») without changing which section a title names.
var titleKeyStopWords = map[string]bool{"de": true, "del": true, "la": true, "las": true, "los": true, "el": true, "en": true, "y": true, "a": true, "e": true, "-": true}

// titleKey folds a title and drops its articles and prepositions.
func titleKey(title string) string {
	words := []string{}
	for _, word := range strings.Fields(strings.ReplaceAll(FoldText(title), "-", " ")) {
		if !titleKeyStopWords[word] {
			words = append(words, word)
		}
	}
	return strings.Join(words, " ")
}

// ContentCount counts the activities and resources inside a section and
// all its nested subsections.
func (s CourseState) ContentCount(sectionID string) int {
	return s.contentCount(sectionID, map[string]bool{})
}

func (s CourseState) contentCount(sectionID string, seen map[string]bool) int {
	index, ok := s.byID[sectionID]
	if !ok || seen[sectionID] {
		return 0
	}
	seen[sectionID] = true
	section := s.Sections[index]
	total := len(section.Modules)
	for _, child := range section.Children {
		total += s.contentCount(child, seen)
	}
	return total
}

// ProvablyEmpty reports whether a section and every nested subsection have no
// activity, no resource and, as Moodle states explicitly, no summary. A
// section whose summary state is unknown is never called empty.
func (s CourseState) ProvablyEmpty(sectionID string) bool {
	return s.provablyEmpty(sectionID, map[string]bool{})
}

func (s CourseState) provablyEmpty(sectionID string, seen map[string]bool) bool {
	index, ok := s.byID[sectionID]
	if !ok {
		return false
	}
	if seen[sectionID] {
		return true
	}
	seen[sectionID] = true
	section := s.Sections[index]
	if len(section.Modules) > 0 || section.HasSummary == nil || *section.HasSummary {
		return false
	}
	for _, child := range section.Children {
		if !s.provablyEmpty(child, seen) {
			return false
		}
	}
	return true
}

// ForumUserPost is one post the logged-in instructor wrote in a forum.
type ForumUserPost struct {
	DiscussionID   int
	DiscussionName string
	Subject        string
	IsReply        bool
	TimeCreated    int64
}

type rawUserPosts struct {
	Discussions []struct {
		ID    json.Number `json:"id"`
		Name  string      `json:"name"`
		Posts struct {
			UserPosts []struct {
				Subject   string      `json:"subject"`
				HasParent bool        `json:"hasparent"`
				Created   json.Number `json:"timecreated"`
			} `json:"userposts"`
		} `json:"posts"`
	} `json:"discussions"`
}

// ForumUserPosts reads, in one call, every post the logged-in instructor
// wrote in a forum (mod_forum_get_discussion_posts_by_userid, verified on the
// live site on 2026-10-01). Only subjects, dates and whether each post is a
// reply are kept: message bodies and learners' names are never read.
func (c *Client) ForumUserPosts(ctx context.Context, session Session, cmID int) ([]ForumUserPost, error) {
	if cmID <= 0 || session.UserID <= 0 {
		return nil, fmt.Errorf("%w: foro o usuario desconocido", ErrAJAXUnavailable)
	}
	var raw rawUserPosts
	args := map[string]any{"userid": session.UserID, "cmid": cmID, "sortby": "created", "sortdirection": "ASC"}
	if err := c.CallAJAX(ctx, session, "mod_forum_get_discussion_posts_by_userid", args, &raw); err != nil {
		return nil, err
	}
	posts := []ForumUserPost{}
	for _, discussion := range raw.Discussions {
		for _, entry := range discussion.Posts.UserPosts {
			post := ForumUserPost{DiscussionID: numberToInt(discussion.ID), DiscussionName: strings.TrimSpace(discussion.Name), Subject: strings.TrimSpace(entry.Subject), IsReply: entry.HasParent}
			if created, err := entry.Created.Float64(); err == nil {
				post.TimeCreated = int64(created)
			}
			posts = append(posts, post)
		}
	}
	return posts, nil
}

func numberToInt(value json.Number) int {
	if parsed, err := value.Int64(); err == nil {
		return int(parsed)
	}
	if parsed, err := value.Float64(); err == nil {
		return int(parsed)
	}
	return 0
}

var accentFolder = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ü", "u", "Ñ", "n",
)

// FoldText lowercases and strips Spanish accents so titles compare reliably.
func FoldText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(accentFolder.Replace(value)), " "))
}

// ModuleCard reads the course-page card of one activity through AJAX
// (core_course_get_module, verified on the live site on 2026-10-01). It is
// the same HTML the course page shows: name, dates and restrictions.
func (c *Client) ModuleCard(ctx context.Context, session Session, cmID int) (string, error) {
	if cmID <= 0 {
		return "", fmt.Errorf("%w: actividad inválida", ErrAJAXUnavailable)
	}
	var card string
	if err := c.CallAJAX(ctx, session, "core_course_get_module", map[string]any{"id": cmID, "sectionreturn": nil}, &card); err != nil {
		return "", err
	}
	return card, nil
}
