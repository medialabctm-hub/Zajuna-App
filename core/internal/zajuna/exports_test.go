package zajuna

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestParseForumIndexReadsForumsAndDiscussionCounts(t *testing.T) {
	body := `<table><tr><th>Foro</th></tr>
		<tr><td class="cell c0"><a href="view.php?f=282639" >ANUNCIOS</a></td><td class="cell c2"><a href="view.php?f=282639" >62</a></td></tr>
		<tr><td class="cell c1"><a href="view.php?f=282640" >Foro de inducci&oacute;n</a></td><td class="cell c3"><a href="view.php?f=282640" >32</a></td></tr></table>`
	entries := parseForumIndex(body)
	if len(entries) != 2 || entries[0].ForumID != 282639 || entries[0].Discussions != 62 || entries[1].Name != "Foro de inducción" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestParseForumExportKeepsOnlyIdsAndDates(t *testing.T) {
	// Moodle's JSON data format: one array per sheet.
	sheet := `[[{"id":"10","discussion":"5","parent":"0","userid":"7","userfullname":"Instructor","created":"1700000000","message":"hola"},
		{"id":11,"discussion":5,"parent":10,"userid":42,"created":1700000100,"message":"aporte"},
		{"id":12,"discussion":5,"parent":0,"userid":43,"created":1700000050}]]`
	posts, err := parseForumExport([]byte(sheet))
	if err != nil || len(posts) != 3 || posts[1].ParentID != 10 || posts[1].UserID != 42 || posts[0].Created != 1700000000 {
		t.Fatalf("posts = %#v, %v", posts, err)
	}
	flat, err := parseForumExport([]byte(`[{"id":1,"userid":2}]`))
	if err != nil || len(flat) != 1 {
		t.Fatalf("flat = %#v, %v", flat, err)
	}
	if _, err := parseForumExport([]byte(`<html>sin permiso</html>`)); !errors.Is(err, ErrExportUnavailable) {
		t.Fatalf("html err = %v", err)
	}
	// Post 11 answers the instructor (not waiting); post 12 starts a discussion.
	count, oldest := UnansweredPosts(posts, 7)
	if count != 1 || oldest != 1700000050 {
		t.Fatalf("unanswered = %d, oldest %d", count, oldest)
	}
	// A learner answering another learner waits for the instructor too.
	posts = append(posts, ForumPostRecord{ID: 15, ParentID: 12, UserID: 45, Created: 1700000400})
	if count, _ := UnansweredPosts(posts, 7); count != 2 {
		t.Fatalf("with a reply between learners, unanswered = %d", count)
	}
	// Once the instructor answers 12, only 15 waits.
	posts = append(posts, ForumPostRecord{ID: 16, ParentID: 12, UserID: 7, Created: 1700000500})
	if count, _ := UnansweredPosts(posts, 7); count != 1 {
		t.Fatalf("after replying to 12, unanswered = %d", count)
	}
}

func TestParseGradeHistoryDropsNamesAndKeepsFeedback(t *testing.T) {
	csvBody := string(rune(0xFEFF)) + `"Fecha y Hora",Nombre,"Nombre de usuario","Correo electrónico","Ítem de calificación","Calificación original","Calificación revisada",Calificador,Fuente,Anuladas,Bloquear,"Excluir de los cálculos","Texto de retroalimentación"
"lunes, 3 de marzo de 2025, 10:00","Aprendiz Uno",a1,a1@example.com,"Taller. AA4-EV02",-,A,"Instructor",mod/assign,No,No,No,"Muy bien"
"lunes, 3 de marzo de 2025, 10:05","Aprendiz Dos",a2,a2@example.com,"Taller. AA4-EV02",-,-,"Instructor",mod/assign,No,No,No,
`
	entries, err := parseGradeHistory(csvBody)
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %#v, %v", entries, err)
	}
	if !entries[0].Graded || !entries[0].HasFeedback || entries[1].Graded || entries[1].HasFeedback || entries[0].Item != "Taller. AA4-EV02" {
		t.Fatalf("entries = %#v", entries)
	}
	if _, err := parseGradeHistory("<html></html>"); !errors.Is(err, ErrExportUnavailable) {
		t.Fatalf("html err = %v", err)
	}
}

func TestGradeItemCountAndPendingGradingsThroughAJAX(t *testing.T) {
	client, session := ajaxTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		switch r.URL.Query().Get("info") {
		case "core_grades_get_grade_tree":
			_, _ = w.Write([]byte(`[{"error":false,"data":"{\"children\":[{\"id\":\"1\",\"iscategory\":false},{\"id\":\"2\",\"iscategory\":true,\"children\":[{\"id\":\"3\",\"iscategory\":false}]}]}"}]`))
		case "core_calendar_get_action_events_by_course":
			_, _ = w.Write([]byte(`[{"error":false,"data":{"events":[{"url":"https://z/zajuna/mod/assign/view.php?id=3010232","action":{"name":"Calificar","itemcount":3}},{"url":"https://z/zajuna/mod/forum/view.php?id=9","action":{"name":"Vista","itemcount":1}}]}}]`))
		}
	})
	count, err := client.GradeItemCount(context.Background(), session, 41080)
	if err != nil || count != 2 {
		t.Fatalf("grade items = %d, %v", count, err)
	}
	pending, err := client.PendingGradings(context.Background(), session, 41080)
	if err != nil || len(pending) != 1 || pending[0].CMID != 3010232 || pending[0].Count != 3 {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
}

func TestPublishedSheetsAndTheirIssues(t *testing.T) {
	page := `<iframe src="https://docs.google.com/spreadsheets/d/e/2PACX-abc_DEF-1/pubhtml?widget=true&amp;headers=false&amp;gid=1553500186"></iframe>`
	urls := PublishedSheetCSVURLs(page)
	if len(urls) != 1 || urls[0] != "https://docs.google.com/spreadsheets/d/e/2PACX-abc_DEF-1/pub?single=true&output=csv&gid=1553500186" {
		t.Fatalf("urls = %#v", urls)
	}
	issues := SheetIssues([][]string{{"Fase", "Fecha fin fase"}, {"Planear", "#¡REF!"}, {"Hacer", "#REF!"}})
	if len(issues) != 2 || !strings.Contains(strings.Join(issues, " "), "#¡REF!") {
		t.Fatalf("issues = %#v", issues)
	}
	if got := SheetIssues([][]string{{"", " "}}); len(got) != 1 || !strings.Contains(got[0], "vacía") {
		t.Fatalf("empty = %#v", got)
	}
	if got := SheetIssues([][]string{{"Fase", "2026-03-01"}}); len(got) != 0 {
		t.Fatalf("a clean sheet has no issues: %#v", got)
	}
	if _, err := FetchPublishedSheet(context.Background(), "https://evil.example/spreadsheets/d/e/x/pub?output=csv"); !errors.Is(err, ErrExportUnavailable) {
		t.Fatalf("other hosts must be rejected: %v", err)
	}
}
