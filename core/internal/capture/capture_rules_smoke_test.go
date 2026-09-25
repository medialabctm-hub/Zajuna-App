package capture

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/zajuna-app/core/internal/checklist"
)

// forumListHTML mirrors the Moodle 4 discussion list of a SENA course: the
// columns are identified by their header text, as in Zajuna.
func forumListHTML(rows string) string {
	return `<html><body id="page-mod-forum-view"><div id="region-main">
<table class="table discussion-list"><thead><tr>
<th>Debate</th><th>Comenzado por</th><th>Último mensaje</th><th>Réplicas</th>
</tr></thead><tbody>` + rows + `</tbody></table></div></body></html>`
}

func forumRow(topic, author, lastPoster, replies string) string {
	return `<tr><td>` + topic + `</td><td class="author">` + author + `</td><td>` + lastPoster + `</td><td>` + replies + `</td></tr>`
}

func serveHTML(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func replyOptions() CaptureOptions {
	return CaptureOptions{
		Selector:        "#region-main table.discussion-list",
		RequireSelector: true, OwnerOnly: true, OwnerName: "ALEX FERNANDO",
		RowSelector: "tbody tr", RowsPerShot: 2, RowRequireReply: true,
	}
}

func TestForumReplyFilterSmoke(t *testing.T) {
	if os.Getenv("ZAJUNA_RUN_BROWSER_SMOKE") != "1" {
		t.Skip("browser smoke disabled")
	}
	runtime := Resolve("")
	answered := forumListHTML(
		// Opened by the instructor, never answered: not proof of replies.
		forumRow("Apertura del FORO", "ALEX FERNANDO ZAPATA", "ALEX FERNANDO ZAPATA", "0") +
			// A student has the last word: the instructor did not answer.
			forumRow("Activar efecto Bloom", "ALEX FERNANDO ZAPATA", "ESTEBAN CASTILLO", "1") +
			// A student question answered by the instructor.
			forumRow("Duda sobre UVs", "ESTEBAN CASTILLO", "ALEX FERNANDO ZAPATA", "2"))
	output := filepath.Join(t.TempDir(), "replies.png")
	result, err := runtime.CaptureURLWithMetadataAndCookiesAndOptions(context.Background(), serveHTML(t, answered).URL, output, nil, replyOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsTotal != 1 {
		t.Fatalf("only the answered discussion is evidence, got %d rows", result.RowsTotal)
	}

	unanswered := forumListHTML(forumRow("Apertura del FORO", "ALEX FERNANDO ZAPATA", "ALEX FERNANDO ZAPATA", "0"))
	_, err = runtime.CaptureURLWithMetadataAndCookiesAndOptions(context.Background(), serveHTML(t, unanswered).URL, filepath.Join(t.TempDir(), "none.png"), nil, replyOptions())
	if !errors.Is(err, ErrContentAbsent) || !strings.Contains(err.Error(), "no tiene respuestas del instructor") {
		t.Fatalf("a forum without instructor replies must be an absence, got %v", err)
	}
}

func TestForumDatesSelectorSmoke(t *testing.T) {
	if os.Getenv("ZAJUNA_RUN_BROWSER_SMOKE") != "1" {
		t.Skip("browser smoke disabled")
	}
	runtime := Resolve("")
	options := CaptureOptions{Selector: checklist.ForumDatesSelector, RequireSelector: true}
	withDates := `<html><body id="page-mod-forum-view"><div id="region-main"><h2>Foro temático. AA2-EV01</h2>
<div><strong>Vencimiento:</strong> domingo, 9 de febrero de 2025, 22:57</div></div></body></html>`
	if _, err := runtime.CaptureURLWithMetadataAndCookiesAndOptions(context.Background(), serveHTML(t, withDates).URL, filepath.Join(t.TempDir(), "dates.png"), nil, options); err != nil {
		t.Fatalf("a forum that shows its dates must be captured: %v", err)
	}
	withoutDates := `<html><body id="page-mod-forum-view"><div id="region-main"><h2>Foro Temático</h2>
<p>Añadir un nuevo tema de debate</p></div></body></html>`
	_, err := runtime.CaptureURLWithMetadataAndCookiesAndOptions(context.Background(), serveHTML(t, withoutDates).URL, filepath.Join(t.TempDir(), "nodates.png"), nil, options)
	if !errors.Is(err, ErrSelectorNotFound) || !strings.Contains(err.Error(), "candidatos=0") {
		t.Fatalf("a forum without dates must not match, got %v", err)
	}

	// With the page check, a rendered forum without dates is an absence
	// and a Moodle error page is not (its previous evidence must stay).
	options.AbsenceSelector = `#page-mod-forum-view #region-main form[action*="/mod/forum/search.php"]`
	renderedForum := `<html><body id="page-mod-forum-view"><div id="region-main"><h2>Foro Temático</h2>
<form action="/zajuna/mod/forum/search.php"><input name="search"></form></div></body></html>`
	_, err = runtime.CaptureURLWithMetadataAndCookiesAndOptions(context.Background(), serveHTML(t, renderedForum).URL, filepath.Join(t.TempDir(), "absent.png"), nil, options)
	if !errors.Is(err, ErrContentAbsent) {
		t.Fatalf("a rendered forum without dates must be ErrContentAbsent, got %v", err)
	}
	errorPage := `<html><body id="page-mod-forum-view"><div id="region-main"><div class="alert alert-danger">Lo sentimos, pero usted no tiene permiso</div></div></body></html>`
	_, err = runtime.CaptureURLWithMetadataAndCookiesAndOptions(context.Background(), serveHTML(t, errorPage).URL, filepath.Join(t.TempDir(), "error.png"), nil, options)
	if !errors.Is(err, ErrSelectorNotFound) || errors.Is(err, ErrContentAbsent) {
		t.Fatalf("a Moodle error page is a failure, not an absence, got %v", err)
	}
}

// TestEmbeddedSheetGrowsToItsNestedContentSmoke reproduces the published
// Google Sheets widget: the grid lives in a nested #pageswitcher-content
// frame, so measuring the widget document alone never saw it and long
// cronogramas were cut at a fixed height.
func TestEmbeddedSheetGrowsToItsNestedContentSmoke(t *testing.T) {
	if os.Getenv("ZAJUNA_RUN_BROWSER_SMOKE") != "1" {
		t.Skip("browser smoke disabled")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/docs.google.com/spreadsheets/widget", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body style="margin:0"><div style="height:30px">tabs</div>
<iframe id="pageswitcher-content" src="/docs.google.com/spreadsheets/content" style="display:block;width:100%;height:calc(100% - 30px);border:0"></iframe></body></html>`))
	})
	mux.HandleFunc("/docs.google.com/spreadsheets/content", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body style="margin:0"><table class="waffle" style="height:4200px"><tr><td>Fase</td></tr></table></body></html>`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Cronograma General</title></head><body><div id="region-main">
<iframe src="/docs.google.com/spreadsheets/widget" width="100%" height="726"></iframe></div></body></html>`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	pw, err := Resolve("").Start()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server.URL); err != nil {
		t.Fatal(err)
	}
	prepareEmbeddedSheets(page, "Cronograma General")
	raw, err := page.Locator(embeddedSheetSelector).First().Evaluate(`(frame) => frame.getBoundingClientRect().height`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if height := evaluatedNumber(raw); height < 4200 || height > embeddedSheetMaxHeight {
		t.Fatalf("the widget must grow to its nested sheet (4200 px + tabs), got %d", height)
	}
}

// moodleCourseHTML is a Moodle 4 course page: sections whose content is
// rendered but collapsed, with toggles that save the state as a user
// preference (here, a request to /pref) when clicked.
const moodleCourseHTML = `<html><head><style>.collapse:not(.show){display:none}</style></head><body><div id="region-main"><ul class="course-content">
<li class="section" id="section-0"><div class="course-section-header"><h3 class="sectionname">ANUNCIOS</h3></div>
<div class="content course-content-item-content collapse show" id="c0"><div id="banner">Animación 3D</div></div></li>
<li class="section" id="s1"><div class="course-section-header"><h3 class="sectionname">INDUCCION</h3>
<a data-toggle="collapse" href="#c1" aria-expanded="false" onclick="fetch('/pref')">t</a></div>
<div class="content collapse" id="c1"><ul>
<li class="section" id="s1a"><div class="course-section-header"><h3 class="sectionname">Evidencias</h3>
<a data-toggle="collapse" href="#c1a" aria-expanded="false" onclick="fetch('/pref')">t</a></div>
<div class="content collapse" id="c1a"><div class="activity-item" id="link">Entrega AA1-EV01</div></div></li>
</ul></div></li>
<li class="section" id="s2"><div class="course-section-header"><h3 class="sectionname">Fase 1 Planear</h3>
<a data-toggle="collapse" href="#c2" aria-expanded="true" onclick="fetch('/pref')">t</a></div>
<div class="content collapse show" id="c2"><div class="activity-item" id="planear">Actividad</div></div></li>
</ul></div></body></html>`

func TestCourseLayoutNeverClicksMoodleTogglesSmoke(t *testing.T) {
	if os.Getenv("ZAJUNA_RUN_BROWSER_SMOKE") != "1" {
		t.Skip("browser smoke disabled")
	}
	var preferenceWrites atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/pref", func(w http.ResponseWriter, _ *http.Request) { preferenceWrites.Add(1) })
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(moodleCourseHTML))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	pw, err := Resolve("").Start()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	visible := func(page playwright.Page, id string) bool {
		value, _ := page.Locator("#" + id).IsVisible()
		return value
	}
	for _, tc := range []struct {
		layout            string
		link, planearOpen bool
	}{
		// 3.1: the first section after the course header (section 0, which
		// also has a panel) with its whole subtree, the rest closed.
		{CourseLayoutFirstSection, true, false},
		// 4.1: only the menu.
		{CourseLayoutMenu, false, false},
		// Other items keep what Zajuna rendered.
		{"", false, true},
	} {
		page, err := browser.NewPage()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := page.Goto(server.URL); err != nil {
			t.Fatal(err)
		}
		prepareCourseMenu(page, CaptureOptions{Selector: "#region-main .course-content", CourseLayout: tc.layout})
		if visible(page, "link") != tc.link || visible(page, "planear") != tc.planearOpen {
			t.Fatalf("layout %q: evidence link visible=%v, Fase 1 open=%v", tc.layout, visible(page, "link"), visible(page, "planear"))
		}
		_ = page.Close()
	}
	if writes := preferenceWrites.Load(); writes != 0 {
		t.Fatalf("preparing the course page must not save Moodle preferences, got %d writes", writes)
	}
}

// TestLoginNoticeDoesNotBlockSubmitSmoke reproduces Zajuna's Connection
// Quality Guard: an informative modal shown shortly after load over the whole
// login page. The submit click must still reach the button.
func TestLoginNoticeDoesNotBlockSubmitSmoke(t *testing.T) {
	if os.Getenv("ZAJUNA_RUN_BROWSER_SMOKE") != "1" {
		t.Skip("browser smoke disabled")
	}
	body := `<html><body><form action="login_user" onsubmit="event.preventDefault(); window.document.body.dataset.sent = '1'">
<input name="document"><button type="submit">Iniciar sesión</button></form>
<div id="connection-guard-modal" role="dialog" style="display:none; position:fixed; inset:0; background:rgba(0,0,0,.5)"></div>
<script>setTimeout(() => { document.getElementById('connection-guard-modal').style.display = 'flex'; }, 300);</script></body></html>`
	pw, err := Resolve("").Start()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(serveHTML(t, body).URL); err != nil {
		t.Fatal(err)
	}
	dismissLoginNotices(page)
	page.WaitForTimeout(600) // the notice is shown after the style was added
	if err := learnerLoginForm(page).Locator(`button`).First().Click(playwright.LocatorClickOptions{Timeout: playwright.Float(3000)}); err != nil {
		t.Fatalf("the login notice must not block the submit button: %v", err)
	}
	if sent, _ := page.Locator("body").GetAttribute("data-sent"); sent != "1" {
		t.Fatal("the login form was not submitted")
	}
}
