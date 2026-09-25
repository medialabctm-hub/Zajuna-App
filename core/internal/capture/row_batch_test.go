package capture

import (
	"strings"
	"testing"
)

func TestRowBatchWindow(t *testing.T) {
	cases := []struct {
		name                  string
		total, perShot, batch int
		start, end            int
		ok                    bool
	}{
		{"first full batch", 25, 10, 0, 0, 10, true},
		{"middle batch", 25, 10, 1, 10, 20, true},
		{"last partial batch", 25, 10, 2, 20, 25, true},
		{"past the end", 25, 10, 3, 0, 0, false},
		{"exact multiple end", 20, 10, 2, 0, 0, false},
		{"single row", 1, 10, 0, 0, 1, true},
		{"no rows", 0, 10, 0, 0, 0, false},
		{"zero per shot", 5, 0, 0, 0, 0, false},
		{"negative batch", 5, 2, -1, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := rowBatchWindow(tc.total, tc.perShot, tc.batch)
			if start != tc.start || end != tc.end || ok != tc.ok {
				t.Fatalf("rowBatchWindow(%d,%d,%d) = (%d,%d,%v), want (%d,%d,%v)", tc.total, tc.perShot, tc.batch, start, end, ok, tc.start, tc.end, tc.ok)
			}
		})
	}
}

func TestEvaluatedInt(t *testing.T) {
	if got := evaluatedInt(map[string]any{"total": float64(7)}, "total"); got != 7 {
		t.Fatalf("float64 total = %d", got)
	}
	if got := evaluatedInt(map[string]any{"total": 3}, "total"); got != 3 {
		t.Fatalf("int total = %d", got)
	}
	if got := evaluatedInt(nil, "total"); got != 0 {
		t.Fatalf("nil total = %d", got)
	}
}

func TestMoodleNotificationSelectors(t *testing.T) {
	want := []string{"#user-notifications", "#region-main > .alert-dismissible", "#page-content .alert-block.alert-dismissible"}
	if strings.Join(moodleNotificationSelectors, "|") != strings.Join(want, "|") {
		t.Fatalf("moodleNotificationSelectors = %v", moodleNotificationSelectors)
	}
}

func TestSameCapturePageIgnoresFragmentsOnly(t *testing.T) {
	course := "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080"
	if !sameCapturePage(course+"#section-12", course) {
		t.Fatal("a fragment change is the same page")
	}
	if sameCapturePage("https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=3010176", course) {
		t.Fatal("an activity page is not the course page")
	}
}

func TestSheetTabForTitle(t *testing.T) {
	cases := map[string]string{
		"P_524703_V_3135429_R_5_C_9205: Cronograma Fase - Planear | Zajuna": "planear",
		"P_524703_V_3135429_R_5_C_9205: Cronograma Fase - Hacer | Zajuna":   "hacer",
		"Cronograma General | Zajuna":                                       "general",
		"Foro temático | Zajuna":                                            "",
	}
	for title, want := range cases {
		if got := sheetTabForTitle(title); got != want {
			t.Fatalf("sheetTabForTitle(%q) = %q, want %q", title, got, want)
		}
	}
}
