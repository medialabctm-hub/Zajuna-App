package zajuna

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/coursemaps"
)

func TestParseSessionUserID(t *testing.T) {
	cases := []struct{ body, want string }{
		{`<script>M.cfg = {"wwwroot":"https:\/\/zajuna.sena.edu.co\/zajuna","homeurl":{},"sesskey":"x","courseId":41080,"userId":73215};</script>`, "73215"},
		{`<div id="nav-notification-popover-container" class="popover-region" data-userid="4411" data-region="popover-region">`, "4411"},
		{`<script>M.cfg = {"wwwroot":"x","userId":1};</script>`, ""},
		// Other users' profile links (teachers, forum authors) are not the session user.
		{`<a href="/zajuna/user/profile.php?id=999">Otro instructor</a>`, ""},
	}
	for _, tc := range cases {
		if got := parseSessionUserID(tc.body); got != tc.want {
			t.Fatalf("parseSessionUserID(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestForumAccessDeniedDetectsMoodleNotice(t *testing.T) {
	if !forumAccessDenied(`<div class="alert alert-warning">No dispone de permiso para ver los debates de este foro</div>`) {
		t.Fatal("Spanish noviewdiscussionspermission notice was not detected")
	}
	if !forumAccessDenied(`You do not have the permission to view discussions in this forum`) {
		t.Fatal("English noviewdiscussionspermission notice was not detected")
	}
	if forumAccessDenied(`<h2>Anuncios</h2><table class="discussion-list"></table>`) {
		t.Fatal("a normal forum page was flagged as denied")
	}
}

// MDL-219 and the profile without id, end to end against a local fixture:
// the forum that redirects with the permission notice is marked restricted
// and left out of 11.4, and the profile URL carries the session user's id.
func TestDiscoverCourseMapMarksDeniedForumsAndPinsProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zajuna/course/view.php":
			notice := ""
			if r.URL.Query().Get("denied") == "1" {
				notice = `<div id="user-notifications"><div class="alert alert-warning">No dispone de permiso para ver los debates de este foro</div></div>`
			}
			_, _ = w.Write([]byte(`<html><head><title>Curso fixture</title><script>M.cfg = {"wwwroot":"x","courseId":41080,"userId":73215};</script></head><body>` + notice + `
				<a href="/zajuna/mod/forum/view.php?id=501">Anuncios de coordinación</a>
				<a href="/zajuna/mod/forum/view.php?id=502">Anuncios semanales</a>
				<a href="/zajuna/user/profile.php?id=999">Otro instructor</a>
			</body></html>`))
		case "/zajuna/mod/forum/view.php":
			if r.URL.Query().Get("id") == "501" {
				http.Redirect(w, r, "/zajuna/course/view.php?id=41080&denied=1", http.StatusSeeOther)
				return
			}
			_, _ = w.Write([]byte(`<html><title>Anuncios semanales</title><table class="discussion-list"></table></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newClient(server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	session := Session{Client: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, BaseURL: server.URL}
	record, err := client.DiscoverCourseMap(context.Background(), session, "41080", CrawlOptions{MaxDepth: 2, MaxPages: 10, MaxLinksPerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	restricted := map[string]bool{}
	for _, route := range record.Routes {
		restricted[route.URL] = route.Restricted
	}
	if !restricted[server.URL+"/zajuna/mod/forum/view.php?id=501"] || restricted[server.URL+"/zajuna/mod/forum/view.php?id=502"] {
		t.Fatalf("unexpected restricted flags: %#v", restricted)
	}
	var announcements []string
	if err := json.Unmarshal(record.ByItemCode["11.4"], &announcements); err != nil {
		t.Fatal(err)
	}
	for _, value := range announcements {
		if strings.Contains(value, "id=501") {
			t.Fatalf("11.4 still selects the forum without access: %#v", announcements)
		}
	}
	if len(announcements) != 1 || !strings.Contains(announcements[0], "id=502") {
		t.Fatalf("11.4 must keep the accessible forum: %#v", announcements)
	}
	wantProfile := server.URL + "/zajuna/user/profile.php?id=73215"
	if record.ProfileURL != wantProfile {
		t.Fatalf("profile URL = %q, want %q", record.ProfileURL, wantProfile)
	}
	var profile []string
	if err := json.Unmarshal(record.ByItemCode["2.1.1"], &profile); err != nil || len(profile) != 1 || profile[0] != wantProfile {
		t.Fatalf("2.1.1 must use the session user's profile: %#v (%v)", profile, err)
	}
}

func TestResolverSkipsRestrictedRoutes(t *testing.T) {
	routes := []coursemaps.Route{
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=1", Kind: "forum", Title: "Anuncios", Restricted: true},
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=2", Kind: "forum", Title: "Anuncios del curso"},
	}
	groups := buildExactChecklistRouteGroups(routes, "41080", "")
	for _, code := range []string{"11.1.1", "11.4"} {
		values := groups[code]
		if len(values) != 1 || !strings.Contains(values[0], "id=2") {
			t.Fatalf("%s must resolve only to the accessible forum: %#v", code, values)
		}
	}
	generic := checklistRouteGroups(routes)
	if len(generic["11.4"]) != 1 || !strings.Contains(generic["11.4"][0], "id=2") {
		t.Fatalf("generic projection kept the restricted forum: %#v", generic["11.4"])
	}
}
