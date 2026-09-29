package timeline

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// maxSheetRows acota la lectura: un cronograma real tiene decenas de filas y
// el CSV viene de un enlace externo que no controlamos.
const maxSheetRows = 5000

// columns guarda el índice (base 0) de cada columna reconocida en la cabecera.
// -1 significa que la hoja no la trae.
type columns struct {
	phase, project, activity, outcome, evidence, area int
	start, end                                        int
}

var (
	evidenceCode = regexp.MustCompile(`(?i)\bGA\d+-\d+\.?-AA\d+-EV\d+\b`)
	activityCode = regexp.MustCompile(`(?i)\bGA\d+-\d+\.?-AA\d+\b`)
	spaces       = regexp.MustCompile(`\s+`)
	numericDate  = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})/(\d{4})$`)
	isoDate      = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
	longDate     = regexp.MustCompile(`^(\d{1,2}) de ([a-z]+) de (\d{4})$`)
)

var spanishMonths = map[string]time.Month{
	"enero": time.January, "febrero": time.February, "marzo": time.March, "abril": time.April,
	"mayo": time.May, "junio": time.June, "julio": time.July, "agosto": time.August,
	"septiembre": time.September, "setiembre": time.September, "octubre": time.October,
	"noviembre": time.November, "diciembre": time.December,
}

var accents = strings.NewReplacer("Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N")

// ParseSheetCSV lee una pestaña de cronograma exportada como CSV.
//
// Las columnas se reconocen por el texto de su cabecera, no por posición,
// porque cada pestaña las ordena distinto (la de cronograma general no trae
// evidencias). Las celdas combinadas llegan vacías en el CSV, así que fase,
// actividades, resultado y área se arrastran hacia abajo. Los datos rotos
// (#REF!, fin antes que inicio, sin fecha) no abortan: quedan como Issue.
func ParseSheetCSV(name string, r io.Reader) (Sheet, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	var rows [][]string
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Sheet{}, fmt.Errorf("leer el CSV de %q: %w", name, err)
		}
		if len(rows) == 0 && len(record) > 0 {
			record[0] = strings.TrimPrefix(record[0], "\ufeff")
		}
		rows = append(rows, record)
		if len(rows) > maxSheetRows {
			return Sheet{}, fmt.Errorf("la hoja %q tiene más de %d filas", name, maxSheetRows)
		}
	}

	headerRow, cols, ok := findHeader(rows)
	if !ok {
		return Sheet{}, fmt.Errorf("no se reconoció la cabecera del cronograma en %q: se esperaba «ACTIVIDAD DE APRENDIZAJE» y «FECHAS DE ENTREGA»", name)
	}

	sheet := Sheet{Name: name}
	var phase, project, activity, outcome, area string
	for index := headerRow + 1; index < len(rows); index++ {
		row := rows[index]
		lineNumber := index + 1

		if value := cell(row, cols.phase); value != "" {
			phase = value
		}
		if value := cell(row, cols.project); value != "" {
			project = value
		}
		if value := cell(row, cols.activity); value != "" {
			activity = value
		}
		if value := cell(row, cols.outcome); value != "" {
			outcome = value
		}
		if value := cell(row, cols.area); value != "" {
			area = value
		}

		evidence := cell(row, cols.evidence)
		rawStart, rawEnd := dateCell(row, cols.start), dateCell(row, cols.end)
		hasEvidenceColumn := cols.evidence >= 0

		var kind Kind
		var title string
		switch {
		case hasEvidenceColumn && evidence != "":
			kind, title = KindEvidence, evidence
		case !hasEvidenceColumn && cell(row, cols.activity) != "" && (rawStart != "" || rawEnd != ""):
			kind, title = KindActivity, activity
		case hasEvidenceColumn && evidence == "" && (rawStart != "" || rawEnd != ""):
			// Fechas sueltas bajo una evidencia sin texto: se conservan.
			kind, title = KindEvidence, activity
		default:
			continue
		}

		event := Event{
			Sheet:    name,
			Row:      lineNumber,
			Kind:     kind,
			Phase:    phase,
			Project:  project,
			Activity: activity,
			Outcome:  outcome,
			Area:     area,
			Title:    truncate(collapse(title), 200),
			Key:      eventKey(kind, evidence, activity),
		}

		var startOK, endOK bool
		event.Start, startOK = parseDate(rawStart)
		event.End, endOK = parseDate(rawEnd)
		id := event.ID()
		report := func(code IssueCode, message string) {
			sheet.Issues = append(sheet.Issues, Issue{Code: code, Sheet: name, Row: lineNumber, EventID: id, Message: message})
		}
		if rawStart == "" && rawEnd == "" {
			report(IssueMissingDate, "La fila no tiene fechas de entrega.")
		}
		if rawStart != "" && !startOK {
			report(IssueInvalidDate, fmt.Sprintf("La fecha de inicio no se pudo leer: %q.", rawStart))
		}
		if rawEnd != "" && !endOK {
			report(IssueInvalidDate, fmt.Sprintf("La fecha de fin no se pudo leer: %q.", rawEnd))
		}
		if startOK && endOK && event.End.Before(event.Start) {
			report(IssueEndBeforeStart, fmt.Sprintf("La fecha de fin (%s) es anterior a la de inicio (%s).", event.End.Format("02/01/2006"), event.Start.Format("02/01/2006")))
		}
		sheet.Events = append(sheet.Events, event)
	}
	return sheet, nil
}

// findHeader busca la fila de cabecera entre las primeras: la que trae la
// actividad de aprendizaje y las fechas de entrega. Devuelve el índice de esa
// fila. Si la columna de fechas es solo una, la de fin es la siguiente.
func findHeader(rows [][]string) (int, columns, bool) {
	limit := len(rows)
	if limit > 40 {
		limit = 40
	}
	for index := 0; index < limit; index++ {
		cols := columns{phase: -1, project: -1, activity: -1, outcome: -1, evidence: -1, area: -1, start: -1, end: -1}
		for col, raw := range rows[index] {
			text := normalize(raw)
			switch {
			case strings.HasPrefix(text, "FECHAS DE ENTREGA"):
				cols.start, cols.end = col, col+1
			case strings.HasPrefix(text, "ACTIVIDAD DE APRENDIZAJE"):
				cols.activity = col
			case strings.HasPrefix(text, "ACTIVIDAD DEL PROYECTO"):
				cols.project = col
			case strings.HasPrefix(text, "RESULTADO DE APRENDIZAJE"):
				cols.outcome = col
			case strings.HasPrefix(text, "EVIDENCIAS"):
				cols.evidence = col
			case text == "AREA":
				cols.area = col
			case text == "NOMBRE DE LA FASE" || text == "FASE DEL PROYECTO":
				cols.phase = col
			}
		}
		if cols.activity >= 0 && cols.start >= 0 {
			return index, cols, true
		}
	}
	return 0, columns{}, false
}

// eventKey extrae el código estable de la fila. Una evidencia usa solo su
// código -EV: si no lo trae queda sin Key en vez de heredar el de la actividad,
// que comparten todas las evidencias de esa actividad. Un punto suelto dentro
// del código (GA2-240202501.-AA1, como aparece en hojas reales) se quita.
func eventKey(kind Kind, evidence, activity string) string {
	if kind == KindEvidence {
		return cleanCode(evidenceCode.FindString(evidence))
	}
	return cleanCode(activityCode.FindString(activity))
}

func cleanCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(code, ".", ""))
}

func cell(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}

// dateCell devuelve el texto de una celda de fecha. Los rótulos de subcabecera
// ("INICIA", "FINALIZA") no son fechas ni errores.
func dateCell(row []string, col int) string {
	value := cell(row, col)
	switch normalize(value) {
	case "INICIA", "FINALIZA", "INICIA - FINALIZA":
		return ""
	}
	return value
}

// parseDate acepta dd/mm/aaaa (con o sin ceros), aaaa-mm-dd, "12 de marzo de
// 2025" y números de serie de Excel. Devuelve una fecha de calendario en UTC.
func parseDate(raw string) (time.Time, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return time.Time{}, false
	}
	build := func(year, month, day int) (time.Time, bool) {
		parsed := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		// time.Date normaliza fechas imposibles (31/02): se rechazan.
		if parsed.Year() != year || int(parsed.Month()) != month || parsed.Day() != day {
			return time.Time{}, false
		}
		return parsed, true
	}
	if m := numericDate.FindStringSubmatch(text); m != nil {
		return build(atoi(m[3]), atoi(m[2]), atoi(m[1]))
	}
	if m := isoDate.FindStringSubmatch(text); m != nil {
		return build(atoi(m[1]), atoi(m[2]), atoi(m[3]))
	}
	if m := longDate.FindStringSubmatch(strings.ToLower(spaces.ReplaceAllString(text, " "))); m != nil {
		if month, ok := spanishMonths[m[2]]; ok {
			return build(atoi(m[3]), int(month), atoi(m[1]))
		}
		return time.Time{}, false
	}
	// Número de serie de Excel (días desde 1899-12-30). El rango evita tomar
	// por fecha un número cualquiera, como las horas de una fase.
	if serial, err := strconv.ParseFloat(text, 64); err == nil && serial >= 36526 && serial <= 73415 {
		base := time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC)
		return base.AddDate(0, 0, int(serial)), true
	}
	return time.Time{}, false
}

func atoi(text string) int {
	value, _ := strconv.Atoi(text)
	return value
}

// normalize deja un texto en mayúsculas, sin tildes y con espacios simples,
// para comparar cabeceras sin depender de cómo las escribió cada instructor.
func normalize(text string) string {
	return spaces.ReplaceAllString(strings.TrimSpace(accents.Replace(strings.ToUpper(text))), " ")
}

func collapse(text string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(text, " "))
}

func truncate(text string, maxRunes int) string {
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxRunes-1]) + "…"
}
