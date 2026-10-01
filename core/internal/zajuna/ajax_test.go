package zajuna

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSesskeyAndUserIDCandidatesFromAMoodlePage(t *testing.T) {
	body := `<script>M.cfg = {"wwwroot":"https:\/\/zajuna.sena.edu.co\/zajuna","sesskey":"AbC123xyz9"};</script>
		<div data-userid="1487116"></div><a href="/zajuna/user/profile.php?id=2000001">x</a><a href="/zajuna/user/profile.php?id=1487116">y</a><a href="/user/profile.php?id=1">guest</a>`
	if got := parseSesskey(body); got != "AbC123xyz9" {
		t.Fatalf("sesskey = %q", got)
	}
	if parseSesskey("<html>sin sesión</html>") != "" {
		t.Fatal("a page without sesskey must not invent one")
	}
	got := userIDCandidates(body)
	if len(got) != 2 || got[0] != 1487116 || got[1] != 2000001 {
		t.Fatalf("candidates = %#v (distinct, without the guest)", got)
	}
}

func TestResolveUserIDPicksTheSessionsOwnName(t *testing.T) {
	answer := `[{"error":false,"data":[{"id":2000001,"fullname":"Otra Persona"},{"id":1487116,"fullname":"Instructor De Prueba"}]}]`
	client, session := ajaxTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(answer))
	})
	session.ProfileName = "INSTRUCTOR DE PRUEBA"
	if got := client.ResolveUserID(context.Background(), session, []int{1487116, 2000001}); got != 1487116 {
		t.Fatalf("userId = %d", got)
	}
	session.ProfileName = "Nadie Conocido"
	if got := client.ResolveUserID(context.Background(), session, []int{1487116, 2000001}); got != 0 {
		t.Fatalf("an unknown name must give 0, got %d", got)
	}
	answer = `[{"error":false,"data":[{"id":1487116,"fullname":"Instructor De Prueba"}]}]`
	session.ProfileName = ""
	if got := client.ResolveUserID(context.Background(), session, []int{1487116, 2000001}); got != 1487116 {
		t.Fatalf("a single visible user without a known name = %d", got)
	}
}

func ajaxTestSession(t *testing.T, handler http.HandlerFunc) (*Client, Session) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := newClient(server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	return client, Session{Client: &http.Client{Jar: jar}, BaseURL: server.URL, Sesskey: "key123", UserID: 7}
}

func TestCallAJAXDecodesDataAndTypedErrors(t *testing.T) {
	var lastBody string
	client, session := ajaxTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		lastBody = string(raw)
		if r.URL.Path != "/zajuna/lib/ajax/service.php" || r.URL.Query().Get("sesskey") != "key123" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request %s %s", r.URL, r.Header.Get("Content-Type"))
		}
		switch r.URL.Query().Get("info") {
		case "ok_fn":
			_, _ = w.Write([]byte(`[{"error":false,"data":{"value":3}}]`))
		case "disabled_fn":
			_, _ = w.Write([]byte(`[{"error":true,"exception":{"message":"no disponible","errorcode":"servicenotavailable"}}]`))
		case "expired_fn":
			_, _ = w.Write([]byte(`{"error":"Su sesión ha expirado","errorcode":"servicerequireslogin"}`))
		default:
			_, _ = w.Write([]byte(`<html>login</html>`))
		}
	})
	var out struct{ Value int }
	if err := client.CallAJAX(context.Background(), session, "ok_fn", map[string]any{"courseid": 5}, &out); err != nil || out.Value != 3 {
		t.Fatalf("ok_fn = %#v, %v", out, err)
	}
	var sent []map[string]any
	if err := json.Unmarshal([]byte(lastBody), &sent); err != nil || sent[0]["methodname"] != "ok_fn" {
		t.Fatalf("request body = %s", lastBody)
	}
	if err := client.CallAJAX(context.Background(), session, "disabled_fn", nil, nil); !errors.Is(err, ErrAJAXUnavailable) {
		t.Fatalf("disabled_fn err = %v", err)
	}
	if err := client.CallAJAX(context.Background(), session, "expired_fn", nil, nil); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expired_fn err = %v", err)
	}
	if err := client.CallAJAX(context.Background(), session, "html_fn", nil, nil); !errors.Is(err, ErrAJAXUnavailable) {
		t.Fatalf("html_fn err = %v", err)
	}
	session.Sesskey = ""
	if err := client.CallAJAX(context.Background(), session, "ok_fn", nil, nil); !errors.Is(err, ErrAJAXUnavailable) {
		t.Fatalf("without sesskey err = %v", err)
	}
}

// courseStateFixture mirrors the shape of core_courseformat_get_state on the
// live site (2026-10-01): nested sections, children as {"id": …} objects.
const courseStateFixture = `{"course":{"id":"41080"},
 "section":[
  {"id":"1","number":0,"title":"General","parentid":0,"visible":true,"hassummary":true,"cmlist":["10"],"children":[]},
  {"id":"142","number":142,"title":"Seguimiento y Evaluación","parentid":0,"visible":true,"hassummary":false,"cmlist":[],"children":[{"id":"149","section":149,"children":[]}]},
  {"id":"149","number":149,"title":"Seguimiento a la Formación","parentid":"142","visible":true,"hassummary":false,"cmlist":[],"children":[{"id":"150"},{"id":"154"},{"id":"155"}]},
  {"id":"150","number":150,"title":"Comités evaluativos - Actas","parentid":"149","visible":true,"hassummary":false,"cmlist":["20"],"children":[]},
  {"id":"154","number":154,"title":"Documentos de retención de aprendices","parentid":"149","visible":true,"hassummary":false,"cmlist":[],"children":[]},
  {"id":"155","number":155,"title":"Reuniones EEF - Actas / Marzo","parentid":"149","visible":true,"hassummary":false,"cmlist":[],"children":[]}
 ],
 "cm":[{"id":10,"name":"ANUNCIOS","module":"forum","sectionid":1,"visible":1},{"id":"20","name":"Acta 1","module":"resource","sectionid":"150","visible":true}]}`

func TestParseCourseStateCountsNestedContent(t *testing.T) {
	state, err := ParseCourseState(courseStateFixture)
	if err != nil {
		t.Fatal(err)
	}
	retention := state.SectionsWithTitlePrefix("Documentos de retenci")
	if len(retention) != 1 || state.ContentCount(retention[0].ID) != 0 {
		t.Fatalf("retention = %#v", retention)
	}
	if got := state.ContentCount("149"); got != 1 {
		t.Fatalf("Seguimiento a la Formación holds 1 file through its children, got %d", got)
	}
	if len(state.SectionsWithTitlePrefix("comites evaluativos")) != 1 {
		t.Fatal("titles must match without case or accents")
	}
	if !state.ProvablyEmpty(retention[0].ID) || state.ProvablyEmpty("149") || state.ProvablyEmpty("1") {
		t.Fatal("only a section without activities and without summary is provably empty")
	}
	if _, err := ParseCourseState(`{"section":[]}`); !errors.Is(err, ErrAJAXUnavailable) {
		t.Fatalf("empty state err = %v", err)
	}
}

func TestParseCourseStateUsesParentIDAndUnknownSummaries(t *testing.T) {
	// No children lists: the nesting comes only from parentid.
	state, err := ParseCourseState(`{"section":[
		{"id":"10","title":"Reuniones EEF - Actas","parentid":0,"hassummary":false,"cmlist":[]},
		{"id":"11","title":"Marzo","parentid":"10","hassummary":false,"cmlist":["99"]},
		{"id":"12","title":"Sin dato de resumen","parentid":0,"cmlist":[]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if state.ContentCount("10") != 1 || state.ProvablyEmpty("10") {
		t.Fatal("a parent whose files live in a subsection is not empty")
	}
	if state.ProvablyEmpty("12") {
		t.Fatal("a section whose summary state is unknown is never called empty")
	}
}

func TestForumUserPostsKeepsOnlySubjectsDatesAndReplies(t *testing.T) {
	client, session := ajaxTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"userid":7`) || !strings.Contains(string(raw), `"cmid":3010216`) {
			t.Errorf("args = %s", raw)
		}
		_, _ = w.Write([]byte(`[{"error":false,"data":{"discussions":[{"id":2162820,"name":"Conclusión del foro","authorfullname":"X","posts":{"parentposts":[],"userposts":[{"subject":"Conclusión del foro","hasparent":false,"timecreated":1740065593,"message":"<p>texto</p>"},{"subject":"Re: duda","hasparent":true,"timecreated":1740065600}]}}]}}]`))
	})
	posts, err := client.ForumUserPosts(context.Background(), session, 3010216)
	if err != nil || len(posts) != 2 {
		t.Fatalf("posts = %#v, %v", posts, err)
	}
	if posts[0].IsReply || !posts[1].IsReply || posts[0].DiscussionID != 2162820 || posts[0].TimeCreated != 1740065593 {
		t.Fatalf("posts = %#v", posts)
	}
	session.UserID = 0
	if _, err := client.ForumUserPosts(context.Background(), session, 3010216); !errors.Is(err, ErrAJAXUnavailable) {
		t.Fatalf("unknown user err = %v", err)
	}
}

func TestSectionTitlesTolerateArticles(t *testing.T) {
	state, err := ParseCourseState(`{"section":[{"id":"14","title":"Reporte de curso","hassummary":false,"cmlist":[],"children":[]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.SectionsWithTitlePrefix("Reporte del Curso")) != 1 {
		t.Fatal("«Reporte de curso» is «Reporte del Curso»")
	}
	if len(state.SectionsWithTitlePrefix("Comités evaluativos")) != 0 {
		t.Fatal("other titles do not match")
	}
}
