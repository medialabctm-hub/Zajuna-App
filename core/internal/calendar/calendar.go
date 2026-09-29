// Package calendar calcula los festivos legales de Colombia y decide qué días
// son laborales. Es determinista y no consulta ningún servicio externo.
//
// Todas las fechas son fechas de calendario: solo importan año, mes y día. Se
// devuelven a medianoche UTC y las entradas se normalizan del mismo modo.
package calendar

import (
	"sort"
	"time"
)

type Holiday struct {
	Date time.Time
	Name string
}

// Calendar combina los festivos legales con días no laborales que el
// instructor agrega a mano (cierres institucionales, paros, imprevistos).
type Calendar struct {
	extra map[time.Time]string
}

func New() *Calendar {
	return &Calendar{extra: map[time.Time]string{}}
}

// AddNonWorking marca un día como no laboral. Si ya había un motivo para ese
// día, se reemplaza.
func (c *Calendar) AddNonWorking(day time.Time, reason string) {
	c.extra[Day(day)] = reason
}

// NonWorkingReason explica por qué un día no es laboral: domingo, festivo o
// día agregado a mano. El sábado no cuenta como no laboral: hay entregas en
// sábado, aunque no sea un día hábil (ver IsBusinessDay).
func (c *Calendar) NonWorkingReason(day time.Time) (string, bool) {
	day = Day(day)
	if reason, ok := c.extra[day]; ok {
		return reason, true
	}
	if name, ok := holidayName(day); ok {
		return name, true
	}
	if day.Weekday() == time.Sunday {
		return "domingo", true
	}
	return "", false
}

// IsBusinessDay es verdadero de lunes a viernes cuando el día no es festivo ni
// está marcado como no laboral. Es la regla de los "días hábiles" del checklist.
func (c *Calendar) IsBusinessDay(day time.Time) bool {
	day = Day(day)
	if day.Weekday() == time.Saturday {
		return false
	}
	_, nonWorking := c.NonWorkingReason(day)
	return !nonWorking
}

// AddBusinessDays avanza (n > 0) o retrocede (n < 0) n días hábiles. Con n == 0
// devuelve el mismo día si es hábil, o el siguiente hábil si no lo es.
func (c *Calendar) AddBusinessDays(day time.Time, n int) time.Time {
	day = Day(day)
	step := 1
	if n < 0 {
		step, n = -1, -n
	}
	if n == 0 {
		for !c.IsBusinessDay(day) {
			day = day.AddDate(0, 0, 1)
		}
		return day
	}
	for n > 0 {
		day = day.AddDate(0, 0, step)
		if c.IsBusinessDay(day) {
			n--
		}
	}
	return day
}

// Day normaliza un instante a su fecha de calendario (medianoche UTC).
func Day(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// Holidays devuelve los festivos legales del año ordenados por fecha. Si dos
// festivos coinciden en el mismo día se listan ambos.
func Holidays(year int) []Holiday {
	date := func(month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	easterDay := easter(year)
	offset := func(days int) time.Time { return easterDay.AddDate(0, 0, days) }

	holidays := []Holiday{
		// Fijos.
		{date(time.January, 1), "Año Nuevo"},
		{date(time.May, 1), "Día del Trabajo"},
		{date(time.July, 20), "Día de la Independencia"},
		{date(time.August, 7), "Batalla de Boyacá"},
		{date(time.December, 8), "Inmaculada Concepción"},
		{date(time.December, 25), "Navidad"},
		// Semana Santa: no se trasladan.
		{offset(-3), "Jueves Santo"},
		{offset(-2), "Viernes Santo"},
		// Ley Emiliani: pasan al lunes siguiente si no caen en lunes.
		{mondayOnOrAfter(date(time.January, 6)), "Reyes Magos"},
		{mondayOnOrAfter(date(time.March, 19)), "San José"},
		{mondayOnOrAfter(offset(39)), "Ascensión del Señor"},
		{mondayOnOrAfter(offset(60)), "Corpus Christi"},
		{mondayOnOrAfter(offset(68)), "Sagrado Corazón de Jesús"},
		{mondayOnOrAfter(date(time.June, 29)), "San Pedro y San Pablo"},
		{mondayOnOrAfter(date(time.August, 15)), "Asunción de la Virgen"},
		{mondayOnOrAfter(date(time.October, 12)), "Día de la Raza"},
		{mondayOnOrAfter(date(time.November, 1)), "Todos los Santos"},
		{mondayOnOrAfter(date(time.November, 11)), "Independencia de Cartagena"},
	}
	sort.SliceStable(holidays, func(i, j int) bool { return holidays[i].Date.Before(holidays[j].Date) })
	return holidays
}

func holidayName(day time.Time) (string, bool) {
	name := ""
	for _, holiday := range Holidays(day.Year()) {
		if holiday.Date.Equal(day) {
			if name != "" {
				name += " / "
			}
			name += holiday.Name
		}
	}
	return name, name != ""
}

// mondayOnOrAfter aplica la Ley Emiliani: un lunes se queda donde está y
// cualquier otro día pasa al lunes siguiente.
func mondayOnOrAfter(day time.Time) time.Time {
	return day.AddDate(0, 0, (8-int(day.Weekday()))%7)
}

// easter calcula el Domingo de Pascua gregoriano (algoritmo de Meeus/Jones/Butcher).
func easter(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
