package calendar

import (
	"strings"
	"testing"
	"time"
)

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestEasterKnownDates(t *testing.T) {
	cases := map[int]time.Time{
		2024: date(2024, time.March, 31),
		2025: date(2025, time.April, 20),
		2026: date(2026, time.April, 5),
		2027: date(2027, time.March, 28),
	}
	for year, want := range cases {
		if got := easter(year); !got.Equal(want) {
			t.Errorf("easter(%d) = %s, quiero %s", year, got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}
}

// Las listas son los festivos oficiales publicados para Colombia.
func TestHolidaysMatchOfficialLists(t *testing.T) {
	cases := map[int][]string{
		2025: {
			"2025-01-01", "2025-01-06", "2025-03-24", "2025-04-17", "2025-04-18", "2025-05-01",
			"2025-06-02", "2025-06-23", "2025-06-30", "2025-07-20", "2025-08-07", "2025-08-18",
			"2025-10-13", "2025-11-03", "2025-11-17", "2025-12-08", "2025-12-25",
		},
		2026: {
			"2026-01-01", "2026-01-12", "2026-03-23", "2026-04-02", "2026-04-03", "2026-05-01",
			"2026-05-18", "2026-06-08", "2026-06-15", "2026-06-29", "2026-07-20", "2026-08-07",
			"2026-08-17", "2026-10-12", "2026-11-02", "2026-11-16", "2026-12-08", "2026-12-25",
		},
	}
	for year, want := range cases {
		seen := map[string]bool{}
		for _, holiday := range Holidays(year) {
			seen[holiday.Date.Format("2006-01-02")] = true
		}
		if len(seen) != len(want) {
			t.Errorf("%d: %d fechas distintas, quiero %d", year, len(seen), len(want))
		}
		for _, day := range want {
			if !seen[day] {
				t.Errorf("%d: falta el festivo %s", year, day)
			}
		}
	}
}

func TestHolidaysAreSorted(t *testing.T) {
	holidays := Holidays(2026)
	for i := 1; i < len(holidays); i++ {
		if holidays[i].Date.Before(holidays[i-1].Date) {
			t.Fatalf("festivos desordenados: %s antes de %s", holidays[i].Date, holidays[i-1].Date)
		}
	}
}

func TestCoincidingHolidaysKeepBothNames(t *testing.T) {
	// En 2025 el Sagrado Corazón y San Pedro y San Pablo caen el mismo lunes.
	reason, ok := New().NonWorkingReason(date(2025, time.June, 30))
	if !ok || !strings.Contains(reason, "Sagrado Corazón") || !strings.Contains(reason, "San Pedro") {
		t.Fatalf("motivo = %q, %v", reason, ok)
	}
}

func TestNonWorkingReason(t *testing.T) {
	cal := New()
	cal.AddNonWorking(date(2025, time.September, 10), "Cierre institucional")
	cases := []struct {
		day    time.Time
		reason string
		ok     bool
	}{
		{date(2025, time.April, 13), "domingo", true},
		{date(2025, time.April, 17), "Jueves Santo", true},
		{date(2025, time.September, 10), "Cierre institucional", true},
		{date(2025, time.April, 12), "", false}, // sábado
		{date(2025, time.April, 14), "", false}, // lunes normal
	}
	for _, tc := range cases {
		reason, ok := cal.NonWorkingReason(tc.day)
		if reason != tc.reason || ok != tc.ok {
			t.Errorf("%s: (%q, %v), quiero (%q, %v)", tc.day.Format("2006-01-02"), reason, ok, tc.reason, tc.ok)
		}
	}
}

func TestBusinessDays(t *testing.T) {
	cal := New()
	if cal.IsBusinessDay(date(2025, time.April, 12)) {
		t.Error("el sábado no es día hábil")
	}
	if cal.IsBusinessDay(date(2025, time.May, 1)) {
		t.Error("el 1 de mayo es festivo")
	}
	if !cal.IsBusinessDay(date(2025, time.May, 2)) {
		t.Error("el 2 de mayo de 2025 es viernes hábil")
	}
}

func TestAddBusinessDaysSkipsWeekendsAndHolidays(t *testing.T) {
	cal := New()
	// Miércoles 30 abr 2025 + 2 hábiles: salta el 1 de mayo (festivo) → viernes 2 y lunes 5.
	if got, want := cal.AddBusinessDays(date(2025, time.April, 30), 2), date(2025, time.May, 5); !got.Equal(want) {
		t.Errorf("+2 = %s, quiero %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
	if got, want := cal.AddBusinessDays(date(2025, time.May, 5), -1), date(2025, time.May, 2); !got.Equal(want) {
		t.Errorf("-1 = %s, quiero %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
	// Con 0 un día no hábil pasa al siguiente hábil.
	if got, want := cal.AddBusinessDays(date(2025, time.May, 1), 0), date(2025, time.May, 2); !got.Equal(want) {
		t.Errorf("0 = %s, quiero %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestDayNormalizesTimeOfDay(t *testing.T) {
	late := time.Date(2025, time.May, 1, 23, 30, 0, 0, time.FixedZone("COT", -5*3600))
	if !Day(late).Equal(date(2025, time.May, 1)) {
		t.Errorf("Day(%s) = %s", late, Day(late))
	}
}
