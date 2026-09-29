package timeline

import (
	"strings"
	"testing"
	"time"

	"github.com/zajuna-app/core/internal/calendar"
)

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// phaseCSV imita una pestaña de fase: celdas combinadas (columnas vacías bajo
// la primera fila del grupo), fechas sin ceros, una evidencia sin código, otra
// sin fechas y otra con fecha ilegible. Los datos son inventados.
const phaseCSV = `"CRONOGRAMA FASE 2 HACER

PROGRAMA DE FORMACIÓN TITULADA:",,,,,,,
,,,,,,,
,FECHA DE INICIO FASE:,,,14/04/2025,,,
,,,,,,,
NOMBRE DE LA FASE,ACTIVIDAD DEL PROYECTO,ACTIVIDAD DE APRENDIZAJE,RESULTADO DE APRENDIZAJE,EVIDENCIAS DE APRENDIZAJE,AREA,"FECHAS DE ENTREGA
INICIA  -  FINALIZA",
HACER,AP3. Construir el personaje,GA3-220501083-AA1. Construir la ilustración,220501083-01. Ilustrar referencias,"Evidencia de conocimiento:
Cuestionario GA3-220501083-AA1-EV01",TÉCNICA,14/4/2025,11/05/2025
,,,,"Evidencia de producto:
Bitácora GA3-220501083-AA1-EV02",,28/04/2025,12/05/2025
,,GA2-240202501.-AA1 Reportar opiniones,240202501-02 Intercambiar opiniones,"Conocimiento
Cuestionario GA2-240202501-AA1-EV01",INGLÉS,30/04/2025,1 de mayo de 2025
,,,,Producto sin código,,05/05/2025,20/04/2025
,,,,Evidencia sin fechas,,,
,,,,Evidencia con fecha rota GA2-240202501-AA1-EV09,,#REF!,31/02/2025
`

func TestParseSheetCSVPhaseSheet(t *testing.T) {
	sheet, err := ParseSheetCSV("FASE 2 HACER", strings.NewReader(phaseCSV))
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Events) != 6 {
		t.Fatalf("eventos = %d, quiero 6: %+v", len(sheet.Events), sheet.Events)
	}

	first := sheet.Events[0]
	if first.Key != "GA3-220501083-AA1-EV01" || first.Kind != KindEvidence {
		t.Errorf("primer evento: key=%q kind=%q", first.Key, first.Kind)
	}
	if !first.Start.Equal(date(2025, time.April, 14)) || !first.End.Equal(date(2025, time.May, 11)) {
		t.Errorf("fechas del primer evento: %s → %s", first.Start, first.End)
	}
	if first.Row != 6 {
		t.Errorf("fila = %d, quiero 6", first.Row)
	}
	if first.Title != "Evidencia de conocimiento: Cuestionario GA3-220501083-AA1-EV01" {
		t.Errorf("título = %q", first.Title)
	}

	// La segunda evidencia hereda fase, proyecto, actividad, resultado y área.
	second := sheet.Events[1]
	if second.Phase != "HACER" || second.Project != "AP3. Construir el personaje" ||
		second.Activity != "GA3-220501083-AA1. Construir la ilustración" ||
		second.Outcome != "220501083-01. Ilustrar referencias" || second.Area != "TÉCNICA" {
		t.Errorf("no se arrastraron las celdas combinadas: %+v", second)
	}
	if second.Key != "GA3-220501083-AA1-EV02" {
		t.Errorf("key = %q", second.Key)
	}

	// Un área nueva reemplaza a la arrastrada, y fecha en texto largo se entiende.
	third := sheet.Events[2]
	if third.Area != "INGLÉS" || third.Key != "GA2-240202501-AA1-EV01" {
		t.Errorf("tercer evento: %+v", third)
	}
	if !third.End.Equal(date(2025, time.May, 1)) {
		t.Errorf("fin en texto largo = %s", third.End)
	}
	// El punto suelto del código de actividad no debe afectar al de la evidencia.
	if third.Activity != "GA2-240202501.-AA1 Reportar opiniones" {
		t.Errorf("actividad = %q", third.Activity)
	}

	// Sin código -EV no hay Key: el ID cae a hoja y fila, no al de la actividad.
	noCode := sheet.Events[3]
	if noCode.Key != "" || noCode.ID() != "FASE 2 HACER#9" {
		t.Errorf("evidencia sin código: key=%q id=%q", noCode.Key, noCode.ID())
	}
}

func TestParseSheetCSVReportsBrokenData(t *testing.T) {
	sheet, err := ParseSheetCSV("FASE 2 HACER", strings.NewReader(phaseCSV))
	if err != nil {
		t.Fatal(err)
	}
	got := map[IssueCode][]int{}
	for _, issue := range sheet.Issues {
		got[issue.Code] = append(got[issue.Code], issue.Row)
	}
	// Fila 9: fin (20/04) antes de inicio (05/05). Fila 10: sin fechas.
	// Fila 11: #REF! y 31/02 son dos fechas ilegibles.
	if rows := got[IssueEndBeforeStart]; len(rows) != 1 || rows[0] != 9 {
		t.Errorf("fin_antes_de_inicio = %v", rows)
	}
	if rows := got[IssueMissingDate]; len(rows) != 1 || rows[0] != 10 {
		t.Errorf("sin_fecha = %v", rows)
	}
	if rows := got[IssueInvalidDate]; len(rows) != 2 || rows[0] != 11 || rows[1] != 11 {
		t.Errorf("fecha_invalida = %v", rows)
	}
	// Las fechas rotas no se inventan: el evento sigue con fecha cero.
	broken := sheet.Events[5]
	if !broken.Start.IsZero() || !broken.End.IsZero() {
		t.Errorf("fechas rotas deberían quedar en cero: %s, %s", broken.Start, broken.End)
	}
}

// generalCSV imita "CRONOGRAMA GENERAL": sin columna de evidencias, con
// cabecera de dos filas (INICIA / FINALIZA) y filas de actividades sin fechas.
const generalCSV = `CRONOGRAMA GENERAL DE ACTIVIDADES,,,,,,
,,,,,,
,Para acceder al documento,,,,,
,FECHA DE INICIO DE FORMACIÓN:,,,,,
,,,,,,
FASE DEL PROYECTO,ACTIVIDAD DEL PROYECTO,HORAS DURACIÓN DE LA FASE,ACTIVIDAD DE APRENDIZAJE,ÁREA,FECHAS DE ENTREGA,
,,,,,INICIA,FINALIZA
INDUCCIÓN,N/A,48,Actividades de reflexión inicial.,TÉCNICA,29/9/2025,3/03/2025
,,,,,,
,N/A,,Actividad de Aprendizaje 1. Contexto,,,
PLANEAR,AP1. Analizar el guion,576,GA1-250201022-AA1. Estructurar conceptos,TÉCNICA,5/02/2025,#REF!
`

func TestParseSheetCSVGeneralSheetWithoutEvidenceColumn(t *testing.T) {
	sheet, err := ParseSheetCSV("CRONOGRAMA GENERAL", strings.NewReader(generalCSV))
	if err != nil {
		t.Fatal(err)
	}
	// Las filas de actividad sin fechas y la subcabecera no generan eventos.
	if len(sheet.Events) != 2 {
		t.Fatalf("eventos = %d, quiero 2: %+v", len(sheet.Events), sheet.Events)
	}
	induction := sheet.Events[0]
	if induction.Kind != KindActivity || induction.Phase != "INDUCCIÓN" || induction.Key != "" {
		t.Errorf("inducción: %+v", induction)
	}
	planear := sheet.Events[1]
	if planear.Key != "GA1-250201022-AA1" || planear.Phase != "PLANEAR" || !planear.Start.Equal(date(2025, time.February, 5)) {
		t.Errorf("planear: %+v", planear)
	}

	codes := map[IssueCode]int{}
	for _, issue := range sheet.Issues {
		codes[issue.Code]++
	}
	// Inducción termina antes de empezar; Planear tiene #REF! en el fin.
	if codes[IssueEndBeforeStart] != 1 || codes[IssueInvalidDate] != 1 || len(sheet.Issues) != 2 {
		t.Errorf("issues = %+v", sheet.Issues)
	}
}

func TestParseSheetCSVRejectsUnknownLayout(t *testing.T) {
	_, err := ParseSheetCSV("otra", strings.NewReader("a,b,c\n1,2,3\n"))
	if err == nil || !strings.Contains(err.Error(), "cabecera") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSheetCSVIgnoresByteOrderMark(t *testing.T) {
	csv := "\ufeff" + generalCSV
	sheet, err := ParseSheetCSV("CRONOGRAMA GENERAL", strings.NewReader(csv))
	if err != nil || len(sheet.Events) != 2 {
		t.Fatalf("err=%v eventos=%d", err, len(sheet.Events))
	}
}

func TestParseDate(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Time
		ok   bool
	}{
		{"14/4/2025", date(2025, time.April, 14), true},
		{"05/05/2025", date(2025, time.May, 5), true},
		{"2025-05-05", date(2025, time.May, 5), true},
		{"12 de marzo de 2025", date(2025, time.March, 12), true},
		{"12  DE  Marzo de 2025", date(2025, time.March, 12), true},
		{"45762", date(2025, time.April, 15), true}, // serie de Excel
		{"31/02/2025", time.Time{}, false},
		{"12 de marcia de 2025", time.Time{}, false},
		{"#REF!", time.Time{}, false},
		{"576", time.Time{}, false}, // horas de una fase, no una fecha
		{"", time.Time{}, false},
	}
	for _, tc := range cases {
		got, ok := parseDate(tc.raw)
		if ok != tc.ok || !got.Equal(tc.want) {
			t.Errorf("parseDate(%q) = %s, %v; quiero %s, %v", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCheckCalendarFlagsNonWorkingDays(t *testing.T) {
	cal := calendar.New()
	cal.AddNonWorking(date(2025, time.May, 7), "Paro")
	events := []Event{
		{Key: "A", Start: date(2025, time.April, 13), End: date(2025, time.May, 2)}, // domingo → viernes
		{Key: "B", Start: date(2025, time.May, 1), End: date(2025, time.May, 7)},    // festivo, día agregado
		{Key: "C"}, // fechas rotas: sin aviso de calendario
	}
	issues := CheckCalendar(events, cal)
	if len(issues) != 3 {
		t.Fatalf("issues = %d, quiero 3: %+v", len(issues), issues)
	}
	for _, issue := range issues {
		if issue.Code != IssueNonWorkingDay || issue.EventID == "C" {
			t.Errorf("issue inesperado: %+v", issue)
		}
	}
	if !strings.Contains(issues[0].Message, "domingo") || !strings.Contains(issues[1].Message, "Día del Trabajo") || !strings.Contains(issues[2].Message, "Paro") {
		t.Errorf("mensajes: %+v", issues)
	}
}
