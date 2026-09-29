// Package timeline modela el cronograma de una ficha como una lista de
// eventos normalizados, sin importar de qué formato provenga. Hoy lee el CSV
// de un Google Sheets publicado en la web; PDF, Word y HTML entrarían como
// otros adaptadores que producen los mismos Event.
package timeline

import (
	"fmt"
	"time"

	"github.com/zajuna-app/core/internal/calendar"
)

type Kind string

const (
	// KindEvidence es una entrega concreta (una evidencia de aprendizaje).
	KindEvidence Kind = "evidence"
	// KindActivity es una actividad de aprendizaje con fechas propias, en las
	// hojas que no detallan evidencias (cronograma general).
	KindActivity Kind = "activity"
)

// Event es una fecha de entrega y el lugar exacto del origen del que salió.
type Event struct {
	// Key es el código estable del origen (p. ej. GA1-250201022-AA1-EV01). Puede
	// estar vacío si la fila no trae código; usa ID para identificar el evento.
	Key      string
	Sheet    string
	Row      int // fila del CSV, empezando en 1
	Kind     Kind
	Phase    string
	Project  string
	Activity string
	Outcome  string
	Area     string
	Title    string
	// Start y End son fechas de calendario (medianoche UTC). Cero si el origen
	// no las trae o no se pudieron leer; en ese caso hay un Issue.
	Start time.Time
	End   time.Time
}

// ID identifica el evento: su código si lo tiene, o la hoja y fila.
func (e Event) ID() string {
	if e.Key != "" {
		return e.Key
	}
	return fmt.Sprintf("%s#%d", e.Sheet, e.Row)
}

type IssueCode string

const (
	IssueInvalidDate    IssueCode = "fecha_invalida"
	IssueMissingDate    IssueCode = "sin_fecha"
	IssueEndBeforeStart IssueCode = "fin_antes_de_inicio"
	IssueNonWorkingDay  IssueCode = "dia_no_laboral"
)

// Issue es algo que el instructor debería revisar en el cronograma. No detiene
// la lectura: el resto de eventos sigue siendo válido.
type Issue struct {
	Code    IssueCode
	Sheet   string
	Row     int
	EventID string
	Message string
}

// Sheet es el resultado de leer una pestaña.
type Sheet struct {
	Name   string
	Events []Event
	Issues []Issue
}

// CheckCalendar avisa de los eventos cuyo inicio o fin cae en domingo, festivo
// o un día marcado como no laboral. Las fechas que ya estaban rotas se ignoran
// porque ya tienen su propio Issue.
func CheckCalendar(events []Event, cal *calendar.Calendar) []Issue {
	var issues []Issue
	check := func(event Event, label string, day time.Time) {
		if day.IsZero() {
			return
		}
		reason, nonWorking := cal.NonWorkingReason(day)
		if !nonWorking {
			return
		}
		issues = append(issues, Issue{
			Code:    IssueNonWorkingDay,
			Sheet:   event.Sheet,
			Row:     event.Row,
			EventID: event.ID(),
			Message: fmt.Sprintf("La fecha %s (%s) cae en un día no laboral: %s.", label, day.Format("02/01/2006"), reason),
		})
	}
	for _, event := range events {
		check(event, "de inicio", event.Start)
		check(event, "de fin", event.End)
	}
	return issues
}
