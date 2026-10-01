package workers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/zajuna"
)

type fakeAJAXClient struct {
	state    zajuna.CourseState
	stateErr error
	posts    []zajuna.ForumUserPost
	card     string
	calls    int
}

func (f *fakeAJAXClient) ModuleCard(context.Context, zajuna.Session, int) (string, error) {
	f.calls++
	return f.card, nil
}

func (f *fakeAJAXClient) Login(context.Context, zajuna.Credentials) (zajuna.Session, error) {
	return zajuna.Session{}, nil
}

func (f *fakeAJAXClient) CourseState(context.Context, zajuna.Session, int) (zajuna.CourseState, error) {
	f.calls++
	return f.state, f.stateErr
}

func (f *fakeAJAXClient) ForumUserPosts(context.Context, zajuna.Session, int) ([]zajuna.ForumUserPost, error) {
	f.calls++
	return f.posts, nil
}

func ajaxParams(itemCode, targetURL, sesskey string) checklistTargetParams {
	return checklistTargetParams{
		Target:  checklist.CaptureTarget{ItemCode: itemCode, URL: targetURL},
		Session: zajuna.Session{Client: &http.Client{}, Sesskey: sesskey, UserID: 7},
	}
}

const courseURL = "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080"

func TestAJAXConfirmsAnEmptySubsection(t *testing.T) {
	state, err := zajuna.ParseCourseState(`{"section":[
		{"id":"149","title":"Seguimiento a la Formación","hassummary":false,"cmlist":[],"children":[{"id":"154"},{"id":"150"}]},
		{"id":"154","title":"Documentos de retención de aprendices","hassummary":false,"cmlist":[],"children":[]},
		{"id":"150","title":"Comités evaluativos - Actas","hassummary":false,"cmlist":["20"],"children":[]},
		{"id":"160","title":"Reuniones EEF - Actas","hassummary":true,"cmlist":[],"children":[]}],"cm":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeAJAXClient{state: state}
	worker := &CaptureChecklistWorker{client: fake}
	detail, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("7.3.2", courseURL, "s-empty"))
	if !absent || detail == "" {
		t.Fatalf("7.3.2 must be absent: %q", detail)
	}
	if _, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("7.3.1", courseURL, "s-empty")); absent {
		t.Fatal("7.3.1 (Comités) has a file")
	}
	if fake.calls != 1 {
		t.Fatalf("the course state is read once per session, got %d calls", fake.calls)
	}
	// A summary (a link, an embedded file) is content: not an absence.
	if _, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("7.3.3", courseURL, "s-empty")); absent {
		t.Fatal("7.3.3 has a summary")
	}
	// Parent sections are left to the browser capture.
	if _, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("7.1.1", courseURL, "s-empty")); absent {
		t.Fatal("7.1.1 is not a subsection check")
	}
}

func TestAJAXFailuresAndUnknownTitlesFallBackToTheBrowser(t *testing.T) {
	worker := &CaptureChecklistWorker{client: &fakeAJAXClient{stateErr: errors.New("boom")}}
	if _, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("13.1.3", courseURL, "s-fail")); absent {
		t.Fatal("an API failure must not decide an absence")
	}
	state, _ := zajuna.ParseCourseState(`{"section":[{"id":"1","title":"General","cmlist":[],"children":[]}],"cm":[]}`)
	worker = &CaptureChecklistWorker{client: &fakeAJAXClient{state: state}}
	if _, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("13.1.3", courseURL, "s-missing")); absent {
		t.Fatal("a subsection that does not exist is a route problem, not an absence")
	}
	if _, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("13.1.3", courseURL, "")); absent {
		t.Fatal("without sesskey the API is not used")
	}
}

func TestAJAXForumRules(t *testing.T) {
	forum := "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=3010216"
	cases := []struct {
		name, item string
		posts      []zajuna.ForumUserPost
		absent     bool
	}{
		{"sin respuestas", "9.1.6", []zajuna.ForumUserPost{{Subject: "Apertura del FORO"}}, true},
		{"con respuestas", "9.1.6", []zajuna.ForumUserPost{{Subject: "Re: aporte", IsReply: true}}, false},
		{"sin conclusión", "14.1.1", []zajuna.ForumUserPost{{Subject: "Apertura del FORO"}}, true},
		{"conclusión como debate", "14.1.1", []zajuna.ForumUserPost{{Subject: "Conclusión del foro temático"}}, false},
		{"conclusión como respuesta", "14.1.2", []zajuna.ForumUserPost{{Subject: "Re: CONCLUSION", IsReply: true}}, false},
	}
	for index, entry := range cases {
		worker := &CaptureChecklistWorker{client: &fakeAJAXClient{posts: entry.posts}}
		_, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams(entry.item, forum, fmt.Sprintf("forum-%d", index)))
		if absent != entry.absent {
			t.Fatalf("%s: absent = %v", entry.name, absent)
		}
	}
}

func TestAJAXForumDates(t *testing.T) {
	forum := "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=4355446"
	// Text of the two real cards (ficha 3135429): with and without dates.
	withDates := `<div class="activity-dates"><strong>Vencimiento:</strong> domingo, 9 de febrero de 2025, 22:57</div>`
	without := `<div class="activityname">Foro Temático</div>`
	worker := &CaptureChecklistWorker{client: &fakeAJAXClient{card: without}}
	if _, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("9.1.3", forum, "dates-1")); !absent {
		t.Fatal("a forum card without dates is an absence")
	}
	// Whatever the browser rule accepts is not an absence.
	for index, card := range []string{withDates, `Vence: lunes`, `Abrió: martes`, `Fecha de corte: hoy`, `<div data-region="activity-dates"></div>`} {
		worker = &CaptureChecklistWorker{client: &fakeAJAXClient{card: card}}
		if _, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("9.1.4", forum, fmt.Sprintf("dates-ok-%d", index))); absent {
			t.Fatalf("card %q is not an absence", card)
		}
	}
}
