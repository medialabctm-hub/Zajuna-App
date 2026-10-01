package workers

import (
	"context"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/zajuna"
)

// fullFakeClient offers every optional session-only source.
type fullFakeClient struct {
	fakeAJAXClient
	gradeItems int
	pending    []zajuna.PendingGrading
	history    []zajuna.GradeHistoryEntry
	exported   []zajuna.ForumPostRecord
	page       string
	noForumID  bool
	index      []zajuna.ForumIndexEntry
}

func (f *fullFakeClient) ForumIndex(context.Context, zajuna.Session, int) ([]zajuna.ForumIndexEntry, error) {
	return f.index, nil
}

func (f *fullFakeClient) GradeItemCount(context.Context, zajuna.Session, int) (int, error) {
	return f.gradeItems, nil
}

func (f *fullFakeClient) PendingGradings(context.Context, zajuna.Session, int) ([]zajuna.PendingGrading, error) {
	return f.pending, nil
}

func (f *fullFakeClient) GradeHistory(context.Context, zajuna.Session, int) ([]zajuna.GradeHistoryEntry, error) {
	return f.history, nil
}

func (f *fullFakeClient) ForumInstanceID(context.Context, zajuna.Session, int) (int, error) {
	if f.noForumID {
		return 0, zajuna.ErrExportUnavailable
	}
	return 282641, nil
}

func (f *fullFakeClient) ExportForum(context.Context, zajuna.Session, int) ([]zajuna.ForumPostRecord, error) {
	return f.exported, nil
}

func (f *fullFakeClient) GetPage(context.Context, zajuna.Session, string) (string, error) {
	return f.page, nil
}

type fichaStoreStub struct{ courseID string }

func (s fichaStoreStub) GetFicha(context.Context, string) (sqlite.FichaRecord, error) {
	return sqlite.FichaRecord{ID: "f1", CourseID: s.courseID}, nil
}

func TestGradebookWithoutItemsIsAnAbsence(t *testing.T) {
	gradebook := "https://zajuna.sena.edu.co/zajuna/grade/edit/tree/index.php?id=41080"
	worker := &CaptureChecklistWorker{client: &fullFakeClient{gradeItems: 0}}
	if detail, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("5.1", gradebook, "g-0")); !absent || !strings.Contains(detail, "calificador") {
		t.Fatalf("5.1 without grade items: %q %v", detail, absent)
	}
	worker = &CaptureChecklistWorker{client: &fullFakeClient{gradeItems: 136}}
	if _, absent := worker.ajaxContentAbsence(context.Background(), ajaxParams("5.1", gradebook, "g-1")); absent {
		t.Fatal("a gradebook with items is not an absence")
	}
}

func TestActivityDatesAreNeverAnAJAXAbsence(t *testing.T) {
	// Course cards only print dates when the course shows activity dates.
	params := ajaxParams("6.1", courseURL, "a-0")
	params.Target.ActivityID = "3010232"
	worker := &CaptureChecklistWorker{client: &fullFakeClient{fakeAJAXClient: fakeAJAXClient{card: `<div class="activityname">Taller</div>`}}}
	if _, absent := worker.ajaxContentAbsence(context.Background(), params); absent {
		t.Fatal("6.1 is decided by the browser capture")
	}
}

func TestScheduleSheetIssuesComeFromThePublishedSheet(t *testing.T) {
	previous := fetchSheet
	defer func() { fetchSheet = previous }()
	fetchSheet = func(context.Context, string) ([][]string, error) {
		return [][]string{{"Fase", "Fecha fin fase"}, {"Planear", "#¡REF!"}}, nil
	}
	client := &fullFakeClient{page: `<iframe src="https://docs.google.com/spreadsheets/d/e/2PACX-x/pubhtml?gid=1"></iframe>`}
	target := checklist.CaptureTarget{ItemCode: "1.2.1", URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=4268113"}
	issues := scheduleSheetIssues(context.Background(), client, zajuna.Session{Sesskey: "sheet-1"}, target)
	if len(issues) != 1 || !strings.Contains(issues[0], "#¡REF!") {
		t.Fatalf("issues = %#v", issues)
	}
	target.ItemCode = "4.1"
	if got := scheduleSheetIssues(context.Background(), client, zajuna.Session{Sesskey: "sheet-2"}, target); got != nil {
		t.Fatalf("only schedule items read sheets: %#v", got)
	}
	target.ItemCode, target.URL = "1.2.1", "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080"
	if got := scheduleSheetIssues(context.Background(), client, zajuna.Session{Sesskey: "sheet-3"}, target); got != nil {
		t.Fatalf("the course page mixes every sheet: %#v", got)
	}
}

func TestAbsenceInsightExplainsWhatIsMissing(t *testing.T) {
	forum := ajaxParams("9.1.6", "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=3010216", "i-0")
	worker := &CaptureChecklistWorker{client: &fullFakeClient{exported: []zajuna.ForumPostRecord{
		{ID: 1, UserID: 42, Created: 1735900000}, {ID: 2, UserID: 43, Created: 1736000000},
	}}}
	if got := worker.absenceInsight(context.Background(), forum); !strings.Contains(got, "2 mensajes sin respuesta tuya") {
		t.Fatalf("forum insight = %q", got)
	}
	grading := ajaxParams("10.1.1", "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=3010232&action=grading", "i-1")
	grading.Target.ActivityID, grading.Target.ActivityTitle = "3010232", "Taller. AA4-EV02"
	worker = &CaptureChecklistWorker{
		client: &fullFakeClient{
			pending: []zajuna.PendingGrading{{CMID: 3010232, Count: 3}},
			history: []zajuna.GradeHistoryEntry{{Item: "Taller. AA4-EV02", Graded: true, HasFeedback: false}, {Item: "Otra", Graded: true}},
		},
		fichaStore: fichaStoreStub{courseID: "41080"},
	}
	got := worker.absenceInsight(context.Background(), grading)
	if !strings.Contains(got, "3 entregas por calificar") || !strings.Contains(got, "1 calificaciones y 0 retroalimentaciones") {
		t.Fatalf("grading insight = %q", got)
	}
	// Other items get no insight.
	if got := worker.absenceInsight(context.Background(), ajaxParams("4.1", courseURL, "i-2")); got != "" {
		t.Fatalf("4.1 insight = %q", got)
	}
}

func TestForumIDFallsBackToTheCourseForumIndex(t *testing.T) {
	state, err := zajuna.ParseCourseState(`{"section":[{"id":"1","title":"General","hassummary":false,"cmlist":["3010216"]}],
		"cm":[{"id":"3010216","name":"Foro temático. AA2-EV01","module":"forum","sectionid":"1","visible":true}]}`)
	if err != nil {
		t.Fatal(err)
	}
	client := &fullFakeClient{
		fakeAJAXClient: fakeAJAXClient{state: state},
		noForumID:      true,
		index:          []zajuna.ForumIndexEntry{{ForumID: 282639, Name: "ANUNCIOS"}, {ForumID: 282641, Name: "Foro temático. AA2-EV01"}},
		exported:       []zajuna.ForumPostRecord{{ID: 1, UserID: 42, Created: 1735900000}},
	}
	worker := &CaptureChecklistWorker{client: client, fichaStore: fichaStoreStub{courseID: "41080"}}
	params := ajaxParams("9.1.6", "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=3010216", "idx-0")
	if got := worker.forumIDFromIndex(context.Background(), params, 3010216); got != 282641 {
		t.Fatalf("forum id from index = %d", got)
	}
	if got := worker.absenceInsight(context.Background(), params); !strings.Contains(got, "1 mensajes") {
		t.Fatalf("insight through the index = %q", got)
	}
}
