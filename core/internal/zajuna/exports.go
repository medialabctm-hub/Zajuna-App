package zajuna

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Session-only sources: everything here works with the instructor's own
// login (cookie + sesskey) and Moodle features any instructor has, without
// asking the SENA for tokens or plugins. Verified on the live site on
// 2026-10-01 (docs/zajuna-ajax-2026-10-01.md). People's names, e-mails and
// message texts are dropped as soon as they are parsed: only ids, dates and
// counts reach the app.

// ErrExportUnavailable means a Moodle export did not answer as expected
// (permission, format). Callers carry on without it.
var ErrExportUnavailable = errors.New("la exportación de Zajuna no está disponible")

var (
	forumInstancePattern = regexp.MustCompile(`data-forumid=["'](\d+)["']|forum/export\.php\?id=(\d+)`)
	forumIndexRow        = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	forumIndexLink       = regexp.MustCompile(`(?is)<a href="[^"]*view\.php\?f=(\d+)"[^>]*>([^<]*)</a>`)
)

// ForumInstanceID returns the forum id (the instance the export and some
// AJAX calls need) of the forum activity with this course-module id.
func (c *Client) ForumInstanceID(ctx context.Context, session Session, cmID int) (int, error) {
	body, err := c.GetPage(ctx, session, "/zajuna/mod/forum/view.php?id="+strconv.Itoa(cmID))
	if err != nil {
		return 0, err
	}
	for _, match := range forumInstancePattern.FindAllStringSubmatch(body, -1) {
		for _, group := range match[1:] {
			if id, convErr := strconv.Atoi(group); convErr == nil && id > 0 {
				return id, nil
			}
		}
	}
	return 0, fmt.Errorf("%w: el foro %d no expone su id", ErrExportUnavailable, cmID)
}

// ForumIndexEntry is one row of the course's forum index.
type ForumIndexEntry struct {
	ForumID     int
	Name        string
	Discussions int
}

// ForumIndex reads /mod/forum/index.php?id=<course>: every forum of the
// course with its discussion count, in one page.
func (c *Client) ForumIndex(ctx context.Context, session Session, courseID int) ([]ForumIndexEntry, error) {
	body, err := c.GetPage(ctx, session, "/zajuna/mod/forum/index.php?id="+strconv.Itoa(courseID))
	if err != nil {
		return nil, err
	}
	return parseForumIndex(body), nil
}

func parseForumIndex(body string) []ForumIndexEntry {
	entries := []ForumIndexEntry{}
	seen := map[int]bool{}
	for _, row := range forumIndexRow.FindAllStringSubmatch(body, -1) {
		links := forumIndexLink.FindAllStringSubmatch(row[1], -1)
		if len(links) == 0 {
			continue
		}
		id, err := strconv.Atoi(links[0][1])
		if err != nil || seen[id] {
			continue
		}
		entry := ForumIndexEntry{ForumID: id, Name: strings.TrimSpace(html.UnescapeString(links[0][2]))}
		// The discussions column is a link to the same forum whose text is
		// only a number.
		for _, link := range links[1:] {
			if count, convErr := strconv.Atoi(strings.TrimSpace(link[2])); convErr == nil && link[1] == links[0][1] {
				entry.Discussions = count
				break
			}
		}
		seen[id] = true
		entries = append(entries, entry)
	}
	return entries
}

// ForumPostRecord is one post of an exported forum: ids and dates only.
type ForumPostRecord struct {
	ID           int
	DiscussionID int
	ParentID     int
	UserID       int
	Created      int64
}

// ExportForum downloads the whole forum through Moodle's «Exportar» (JSON)
// and keeps ids and dates. It needs the forum instance id (ForumInstanceID).
func (c *Client) ExportForum(ctx context.Context, session Session, forumID int) ([]ForumPostRecord, error) {
	if forumID <= 0 || !session.HasAJAX() {
		return nil, fmt.Errorf("%w: foro o sesión inválidos", ErrExportUnavailable)
	}
	form := url.Values{
		"id":                              {strconv.Itoa(forumID)},
		"sesskey":                         {session.Sesskey},
		"_qf__mod_forum_form_export_form": {"1"},
		"useridsselected":                 {"_qf__force_multiselect_submission"},
		"discussionids":                   {"_qf__force_multiselect_submission"},
		"from[enabled]":                   {"0"},
		"to[enabled]":                     {"0"},
		"format":                          {"json"},
		"striphtml":                       {"1"},
		"humandates":                      {"0"},
		"submitbutton":                    {"Exportar"},
	}
	response, err := c.doFormResponse(ctx, session, "/zajuna/mod/forum/export.php", form, "/zajuna/mod/forum/export.php?id="+strconv.Itoa(forumID))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: exportación HTTP %d", ErrExportUnavailable, response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("leer exportación de Zajuna: %w", err)
	}
	return parseForumExport(raw)
}

func parseForumExport(raw []byte) ([]ForumPostRecord, error) {
	text := strings.TrimSpace(strings.TrimPrefix(string(raw), string(rune(0xFEFF))))
	if !strings.HasPrefix(text, "[") {
		return nil, fmt.Errorf("%w: la exportación no es JSON", ErrExportUnavailable)
	}
	// Moodle's JSON data format writes one array per sheet: [[{…},{…}]].
	// A flat array of objects is accepted too.
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &rows); err != nil {
		var sheets [][]map[string]json.RawMessage
		if sheetErr := json.Unmarshal([]byte(text), &sheets); sheetErr != nil {
			return nil, fmt.Errorf("%w: JSON ilegible: %v", ErrExportUnavailable, sheetErr)
		}
		rows = nil
		for _, sheet := range sheets {
			rows = append(rows, sheet...)
		}
	}
	posts := make([]ForumPostRecord, 0, len(rows))
	for _, row := range rows {
		post := ForumPostRecord{
			ID: rawInt(row["id"]), DiscussionID: rawInt(row["discussion"]), ParentID: rawInt(row["parent"]),
			UserID: rawInt(row["userid"]), Created: int64(rawInt(row["created"])),
		}
		if post.ID > 0 {
			posts = append(posts, post)
		}
	}
	return posts, nil
}

func rawInt(value json.RawMessage) int {
	text := strings.Trim(strings.TrimSpace(string(value)), `"`)
	if number, err := strconv.ParseFloat(text, 64); err == nil {
		return int(number)
	}
	return 0
}

// UnansweredPosts counts the posts of other people that start a discussion
// or answer someone else and have no reply from the instructor, and returns
// the oldest one's date. Replies to the instructor's own posts are not
// waiting for an answer.
func UnansweredPosts(posts []ForumPostRecord, instructorID int) (int, int64) {
	answered := map[int]bool{}
	for _, post := range posts {
		if post.UserID == instructorID && post.ParentID > 0 {
			answered[post.ParentID] = true
		}
	}
	byID := make(map[int]ForumPostRecord, len(posts))
	for _, post := range posts {
		byID[post.ID] = post
	}
	count, oldest := 0, int64(0)
	for _, post := range posts {
		if post.UserID == instructorID || answered[post.ID] {
			continue
		}
		if parent, ok := byID[post.ParentID]; ok && parent.UserID == instructorID {
			continue
		}
		count++
		if oldest == 0 || post.Created < oldest {
			oldest = post.Created
		}
	}
	return count, oldest
}

// GradeHistoryEntry is one grading event: when, which grade item, and
// whether it carries feedback. Learner and grader names are not kept.
type GradeHistoryEntry struct {
	Time        string
	Item        string
	Graded      bool
	HasFeedback bool
}

// GradeHistory downloads the course's grade history as CSV
// (grade/report/history, any instructor can export it).
func (c *Client) GradeHistory(ctx context.Context, session Session, courseID int) ([]GradeHistoryEntry, error) {
	body, err := c.GetPage(ctx, session, "/zajuna/grade/report/history/index.php?id="+strconv.Itoa(courseID)+"&showreport=1&download=csv")
	if err != nil {
		return nil, err
	}
	return parseGradeHistory(body)
}

func parseGradeHistory(body string) ([]GradeHistoryEntry, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, string(rune(0xFEFF)))))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: el historial no es CSV", ErrExportUnavailable)
	}
	column := func(names ...string) int {
		for index, name := range header {
			folded := FoldText(name)
			for _, wanted := range names {
				if folded == wanted {
					return index
				}
			}
		}
		return -1
	}
	timeCol := column("fecha y hora", "date and time")
	itemCol := column("item de calificacion", "grade item")
	gradeCol := column("calificacion revisada", "revised grade")
	feedbackCol := column("texto de retroalimentacion", "feedback text")
	if timeCol < 0 || itemCol < 0 {
		return nil, fmt.Errorf("%w: el CSV del historial tiene otras columnas", ErrExportUnavailable)
	}
	entries := []GradeHistoryEntry{}
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("%w: CSV ilegible", ErrExportUnavailable)
		}
		cell := func(index int) string {
			if index < 0 || index >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		grade := cell(gradeCol)
		entries = append(entries, GradeHistoryEntry{
			Time: cell(timeCol), Item: cell(itemCol),
			Graded:      grade != "" && grade != "-",
			HasFeedback: cell(feedbackCol) != "",
		})
	}
	return entries, nil
}

// GradeItemCount counts the grade items (not categories) of the course's
// gradebook through AJAX (core_grades_get_grade_tree).
func (c *Client) GradeItemCount(ctx context.Context, session Session, courseID int) (int, error) {
	var encoded any
	if err := c.CallAJAX(ctx, session, "core_grades_get_grade_tree", map[string]any{"courseid": courseID}, &encoded); err != nil {
		return 0, err
	}
	if text, ok := encoded.(string); ok {
		var nested any
		if json.Unmarshal([]byte(text), &nested) == nil {
			encoded = nested
		}
	}
	tree, ok := encoded.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("%w: árbol de calificaciones con otra forma", ErrAJAXUnavailable)
	}
	// Only an explicit (possibly empty) children list counts: any other shape
	// is an unknown answer, never "the gradebook is empty".
	if _, hasChildren := tree["children"].([]any); !hasChildren {
		return 0, fmt.Errorf("%w: árbol de calificaciones sin lista de ítems", ErrAJAXUnavailable)
	}
	return countGradeItems(tree["children"]), nil
}

func countGradeItems(node any) int {
	children, ok := node.([]any)
	if !ok {
		return 0
	}
	total := 0
	for _, child := range children {
		entry, ok := child.(map[string]any)
		if !ok {
			continue
		}
		if category, _ := entry["iscategory"].(bool); category {
			total += countGradeItems(entry["children"])
		} else {
			total++
		}
	}
	return total
}

// PendingGrading is an activity the instructor still has submissions to
// grade, from the course's action events.
type PendingGrading struct {
	CMID  int
	Count int
}

var cmIDInURL = regexp.MustCompile(`[?&]id=(\d+)`)

// PendingGradings reads the action events of the course
// (core_calendar_get_action_events_by_course) and returns, per activity, how
// many submissions wait to be graded.
func (c *Client) PendingGradings(ctx context.Context, session Session, courseID int) ([]PendingGrading, error) {
	var answer struct {
		Events []struct {
			URL    string `json:"url"`
			Action struct {
				Name      string `json:"name"`
				URL       string `json:"url"`
				ItemCount int    `json:"itemcount"`
			} `json:"action"`
		} `json:"events"`
	}
	if err := c.CallAJAX(ctx, session, "core_calendar_get_action_events_by_course", map[string]any{"courseid": courseID, "limitnum": 50}, &answer); err != nil {
		return nil, err
	}
	result := []PendingGrading{}
	for _, event := range answer.Events {
		name := FoldText(event.Action.Name)
		if !strings.Contains(name, "calificar") && !strings.Contains(name, "grade") {
			continue
		}
		source := event.URL
		if source == "" {
			source = event.Action.URL
		}
		match := cmIDInURL.FindStringSubmatch(source)
		if len(match) != 2 {
			continue
		}
		id, _ := strconv.Atoi(match[1])
		result = append(result, PendingGrading{CMID: id, Count: event.Action.ItemCount})
	}
	return result, nil
}

// Published Google Sheets (the course schedules, 1.x) are public: their CSV
// is read without any session, only from docs.google.com over HTTPS.

var sheetFramePattern = regexp.MustCompile(`https://docs\.google\.com/spreadsheets/d/e/([A-Za-z0-9_-]+)/pubhtml[^"'<\s]*`)

// PublishedSheetCSVURLs finds the published Google Sheets embedded in a page
// and returns the CSV address of each one (the gid of the embedded tab).
func PublishedSheetCSVURLs(pageHTML string) []string {
	urls := []string{}
	seen := map[string]bool{}
	for _, match := range sheetFramePattern.FindAllStringSubmatch(pageHTML, -1) {
		source := html.UnescapeString(match[0])
		gid := ""
		if parsed, err := url.Parse(source); err == nil {
			gid = parsed.Query().Get("gid")
		}
		csvURL := "https://docs.google.com/spreadsheets/d/e/" + match[1] + "/pub?single=true&output=csv"
		if gid != "" {
			csvURL += "&gid=" + url.QueryEscape(gid)
		}
		if !seen[csvURL] {
			seen[csvURL] = true
			urls = append(urls, csvURL)
		}
	}
	return urls
}

// sheetHTTPClient only follows redirects inside Google over HTTPS (the
// published CSV answers from googleusercontent.com), never elsewhere.
var sheetHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(request *http.Request, via []*http.Request) error {
		host := strings.ToLower(request.URL.Hostname())
		google := host == "docs.google.com" || strings.HasSuffix(host, ".googleusercontent.com")
		if request.URL.Scheme != "https" || !google || len(via) > 5 {
			return fmt.Errorf("%w: redirección fuera de Google", ErrExportUnavailable)
		}
		return nil
	},
}

// FetchPublishedSheet downloads the CSV of the embedded tab of a published
// sheet. Some documents reject the tab's gid for CSV (HTTP 400, seen in
// ficha 3135429); the document's first tab is NOT read instead, because its
// cells may belong to another tab and would report someone else's errors.
func FetchPublishedSheet(ctx context.Context, csvURL string) ([][]string, error) {
	parsed, err := url.Parse(csvURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "docs.google.com" || !strings.HasPrefix(parsed.Path, "/spreadsheets/d/e/") {
		return nil, fmt.Errorf("%w: hoja no permitida", ErrExportUnavailable)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, csvURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := sheetHTTPClient.Do(request)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("%w: %v", ErrExportUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "csv") {
		// Without the exact tab there is nothing reliable to check.
		return nil, fmt.Errorf("%w: la hoja respondió HTTP %d", ErrExportUnavailable, response.StatusCode)
	}
	reader := csv.NewReader(io.LimitReader(response.Body, 8<<20))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: CSV ilegible", ErrExportUnavailable)
	}
	return rows, nil
}

var sheetErrorPattern = regexp.MustCompile(`#(¡?REF!|N/A|¡?VALOR!|VALUE!|DIV/0!|¿?NOMBRE\?|NAME\?|NUM!|NULL!|ERROR!)`)

// SheetIssue is a formula error found in a schedule sheet: the header of the
// column it is in (the closest label above the cell; empty when unknown or
// for an empty sheet), the error code and how many cells show it.
type SheetIssue struct {
	Column string
	Code   string
	Count  int
}

// Message describes the issue in plain words.
func (issue SheetIssue) Message() string {
	if issue.Code == "" {
		return "la hoja del cronograma está vacía"
	}
	plural := "celda"
	if issue.Count != 1 {
		plural = "celdas"
	}
	if issue.Column != "" {
		return fmt.Sprintf("la columna «%s» del cronograma tiene %d %s con el error %s", issue.Column, issue.Count, plural, issue.Code)
	}
	return fmt.Sprintf("la hoja del cronograma tiene %d %s con el error %s", issue.Count, plural, issue.Code)
}

var digitPattern = regexp.MustCompile(`\d`)

// AnalyzeSheet finds the formula errors of a schedule sheet and the column
// each one is in, or reports an empty sheet.
func AnalyzeSheet(rows [][]string) []SheetIssue {
	filled := 0
	type key struct{ column, code string }
	counts := map[key]int{}
	for r, row := range rows {
		for c, cell := range row {
			cell = strings.TrimSpace(cell)
			if cell == "" {
				continue
			}
			filled++
			code := sheetErrorPattern.FindString(cell)
			if code == "" {
				continue
			}
			counts[key{sheetColumnHeader(rows, r, c), code}]++
		}
	}
	if filled == 0 {
		return []SheetIssue{{}}
	}
	issues := make([]SheetIssue, 0, len(counts))
	for k, count := range counts {
		issues = append(issues, SheetIssue{Column: k.column, Code: k.code, Count: count})
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Column != issues[j].Column {
			return issues[i].Column < issues[j].Column
		}
		return issues[i].Code < issues[j].Code
	})
	return issues
}

// sheetColumnHeader is the closest cell above (r, c) that reads like a label:
// text without digits and without an error.
func sheetColumnHeader(rows [][]string, r, c int) string {
	for up := r - 1; up >= 0; up-- {
		if c >= len(rows[up]) {
			continue
		}
		cell := strings.TrimSpace(rows[up][c])
		if cell == "" || sheetErrorPattern.MatchString(cell) || digitPattern.MatchString(cell) {
			continue
		}
		return strings.Join(strings.Fields(cell), " ")
	}
	return ""
}

// SheetIssues describes the issues of a sheet in plain words.
func SheetIssues(rows [][]string) []string {
	issues := AnalyzeSheet(rows)
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.Message())
	}
	return messages
}
