package checklist

// guideContent is the hand-written guide of one checklist item. Every item of
// the catalog has one (guides_test.go enforces it): the instructor is guided
// up to the point where Zajuna Sync only needs to capture again or receive the
// evidence.
type guideContent struct {
	headline string
	// requirement is what the guideline asks for, in one sentence.
	requirement string
	// location is the navigation path inside the course.
	location string
	// steps fix the content in Zajuna (content absent, and empty section
	// unless emptySteps says otherwise).
	steps      []string
	emptySteps []string
	// create names, with an article, what must exist so route discovery finds
	// the item ("el foro «Foro de Dudas e Inquietudes»").
	create string
	// evidenceHint is exactly what to upload when the instructor hands the
	// evidence over instead of waiting for a capture.
	evidenceHint string
	template     *GuideTemplate
}

const (
	locSeguimientoForm = "Curso → Seguimiento y Evaluación → Seguimiento a la Formación"
	locComitesActas    = "Curso → Seguimiento y Evaluación → Seguimiento a la Formación → Comités evaluativos – Actas"
	locReporteCurso    = "Curso → Seguimiento y Evaluación → Reporte de Curso"
	locSesiones        = "Curso → Sesiones en Línea"
	locForoDudas       = "Curso → Foro de Dudas e Inquietudes"
	locForoTematico    = "Curso → sección de la fase → actividad → Foro Temático"
	locAnuncios        = "Curso → Anuncios (foro de anuncios del curso)"
	locEntregas        = "Curso → actividad de evidencia → Ver todas las entregas"

	stepActivarEdicion = "Activa la edición del curso («Activar edición», arriba a la derecha)."
	stepGuardar        = "Pulsa «Guardar cambios y regresar al curso»."
	stepAnuncioNuevo   = "Abre el foro de Anuncios del curso y pulsa «Añadir un nuevo tema»."
	stepEnviarForo     = "Pulsa «Enviar al foro» y comprueba que el mensaje se vea en la lista."
)

var (
	conclusionTemplate = &GuideTemplate{
		Label: "Texto sugerido para la conclusión",
		Title: "Conclusión del foro temático: [nombre del foro]",
		Body: "Apreciados aprendices:\n\n" +
			"Agradezco sus aportes en este foro. A partir de sus participaciones destaco:\n" +
			"1. [Idea principal 1]\n2. [Idea principal 2]\n3. [Idea principal 3]\n\n" +
			"Conclusión: [síntesis del tema y relación con la actividad de proyecto].\n\n" +
			"Cordialmente,\n[Nombre del instructor]",
	}
	forumFeedbackTemplate = &GuideTemplate{
		Label: "Texto sugerido para la retroalimentación",
		Body: "Hola, [nombre del aprendiz]:\n\n" +
			"Gracias por tu aporte. Destaco [fortaleza concreta del aporte].\n" +
			"Para mejorar, te sugiero [aspecto a fortalecer y cómo hacerlo].\n\n" +
			"Cordialmente,\n[Nombre del instructor]",
	}
	doubtReplyTemplate = &GuideTemplate{
		Label: "Texto sugerido para responder una duda",
		Body: "Hola, [nombre del aprendiz]:\n\n" +
			"Gracias por tu pregunta. [Respuesta clara a la duda].\n" +
			"Si necesitas más apoyo, puedes consultarme en el horario de atención: [día y hora].\n\n" +
			"Cordialmente,\n[Nombre del instructor]",
	}
	phaseStartTemplate = &GuideTemplate{
		Label: "Texto sugerido para el anuncio de inicio de fase",
		Title: "Inicio de la fase [nombre de la fase]",
		Body: "Apreciados aprendices:\n\n" +
			"Hoy iniciamos la fase [nombre de la fase] del proyecto, que va del [fecha de inicio] al [fecha de finalización].\n\n" +
			"Qué consultar: [materiales, guías y recursos de la fase].\n" +
			"Pasos a seguir:\n1. [Paso 1]\n2. [Paso 2]\n3. [Paso 3]\n\n" +
			"Cordialmente,\n[Nombre del instructor]",
	}
	activityStartTemplate = &GuideTemplate{
		Label: "Texto sugerido para el anuncio de inicio de actividad",
		Title: "Inicio de la actividad de proyecto [nombre]",
		Body: "Apreciados aprendices:\n\n" +
			"Iniciamos la actividad de proyecto [nombre], del [fecha de inicio] al [fecha de cierre].\n" +
			"Evidencias a entregar: [evidencias y enlace de envío].\n\n" +
			"Cordialmente,\n[Nombre del instructor]",
	}
	activityCloseTemplate = &GuideTemplate{
		Label: "Texto sugerido para el anuncio de cierre de actividad",
		Title: "Cierre de la actividad de proyecto [nombre]",
		Body: "Apreciados aprendices:\n\n" +
			"El [fecha] cierra la actividad de proyecto [nombre]. Recuerden entregar [evidencias] en [enlace de envío].\n" +
			"La siguiente actividad es [nombre de la siguiente actividad].\n\n" +
			"Cordialmente,\n[Nombre del instructor]",
	}
	weeklySessionTemplate = &GuideTemplate{
		Label: "Texto sugerido para el anuncio",
		Title: "Sesión en línea de la semana — [fecha]",
		Body: "Apreciados aprendices:\n\nLos invito a la sesión en línea de esta semana.\n" +
			"Fecha: [día y fecha]\nHora: [hora]\nEnlace: [enlace de la sesión]\nTema: [tema de la sesión]\n\n" +
			"Cordialmente,\n[Nombre del instructor]",
	}
	approvedTemplate = &GuideTemplate{
		Label: "Texto sugerido para el anuncio",
		Title: "Aprendices que aprobaron la fase [nombre de la fase]",
		Body: "Apreciados aprendices:\n\nFelicito a quienes aprobaron la fase [nombre de la fase]:\n" +
			"- [Nombre del aprendiz]\n- [Nombre del aprendiz]\n\n" +
			"Quienes tienen pendientes, por favor revisen [plan de mejoramiento o indicación].\n\n" +
			"Cordialmente,\n[Nombre del instructor]",
	}
	sessionSummaryTemplate = &GuideTemplate{
		Label: "Texto sugerido para el resumen de la sesión",
		Title: "Resumen de la sesión en línea del [fecha]",
		Body: "Tema: [tema]\nAsistentes: [número]\n\nPuntos tratados:\n1. [Punto 1]\n2. [Punto 2]\n\n" +
			"Compromisos: [compromisos]\nGrabación: [enlace]",
	}
)

func seguimientoDoc(headline, requirement, location, subsection, document string) guideContent {
	return guideContent{
		headline:    headline,
		requirement: requirement,
		location:    location,
		steps: []string{
			stepActivarEdicion,
			"Abre «Seguimiento y Evaluación» (está oculta para los aprendices; con la edición activada sí la ves) y entra en la subsección «" + subsection + "».",
			"Pulsa «Añadir una actividad o un recurso», elige «Archivo» y arrastra " + document + " en PDF.",
			"Ponle un nombre que diga tipo y fecha, por ejemplo «" + subsection + " – 2026-09-15».",
			stepGuardar,
			"Comprueba que el archivo aparezca dentro de la subsección: tenerlo en tu equipo o en Drive no cuenta como publicado.",
		},
		emptySteps: []string{
			"La subsección «" + subsection + "» ya existe, pero solo tiene su título.",
			stepActivarEdicion,
			"Entra en «" + subsection + "», pulsa «Añadir una actividad o un recurso» y elige «Archivo».",
			"Arrastra " + document + " en PDF y ponle un nombre con tipo y fecha.",
			stepGuardar,
			"Comprueba que el archivo aparezca en la subsección y que siga oculta para los aprendices.",
		},
		create:       "la subsección «" + subsection + "» dentro de «Seguimiento y Evaluación»",
		evidenceHint: "una captura de la subsección «" + subsection + "» donde se vea " + document + " publicado con su nombre, o el PDF del documento.",
	}
}

func foroAnuncio(headline, requirement string, steps []string, evidence string, template *GuideTemplate) guideContent {
	return guideContent{
		headline:     headline,
		requirement:  requirement,
		location:     locAnuncios,
		steps:        steps,
		create:       "el foro «Anuncios» del curso (normalmente en la sección inicial)",
		evidenceHint: evidence,
		template:     template,
	}
}

var itemGuides = map[string]guideContent{

	"7.3.1": seguimientoDoc("Publica las actas de los comités evaluativos", "La subsección «Comités evaluativos – Actas» tiene las actas de comité.", locSeguimientoForm, "Comités evaluativos – Actas", "el acta del comité evaluativo"),
	"7.3.2": seguimientoDoc("Sube un documento de retención", "La subsección «Documentos de retención» tiene al menos un documento.", locSeguimientoForm, "Documentos de retención", "el documento de retención"),
	"7.3.3": seguimientoDoc("Sube las actas de las reuniones EEF", "La subsección «Reuniones EEF – Actas» tiene las actas de las reuniones del equipo ejecutor.", locSeguimientoForm, "Reuniones EEF – Actas", "el acta de la reunión EEF"),
	"7.4.1": seguimientoDoc("Publica las actas de comité", "«Comités evaluativos – Actas» tiene la carpeta o los archivos de Actas de Comité.", locComitesActas, "Actas de Comité", "el acta de comité"),
	"7.4.2": seguimientoDoc("Publica los planes de mejoramiento", "«Comités evaluativos – Actas» tiene los planes de mejoramiento.", locComitesActas, "Planes de Mejoramiento", "el plan de mejoramiento"),
	"7.4.3": seguimientoDoc("Publica el registro de novedades", "«Comités evaluativos – Actas» tiene el registro de novedades.", locComitesActas, "Registro de Novedades", "el registro de novedades"),
	"7.4.4": seguimientoDoc("Publica los llamados de atención", "«Comités evaluativos – Actas» tiene los llamados de atención.", locComitesActas, "Llamados de Atención", "el llamado de atención (o un documento que indique que no hubo)"),

	"9.1.1": {
		headline:    "Nombra el foro de dudas según el lineamiento",
		requirement: "El foro de dudas se llama «Foro de Dudas e Inquietudes».",
		location:    locForoDudas,
		steps: []string{
			"Abre el foro de dudas del curso y pulsa «Editar ajustes».",
			"Cambia el nombre a «Foro de Dudas e Inquietudes».",
			stepGuardar,
		},
		create:       "el foro «Foro de Dudas e Inquietudes»",
		evidenceHint: "una captura de la página del foro donde se vea su nombre «Foro de Dudas e Inquietudes».",
	},
	"9.1.2": {
		headline:    "Crea al menos un Foro de Dudas e Inquietudes",
		requirement: "El curso tiene disponible al menos un «Foro de Dudas e Inquietudes».",
		location:    locForoDudas,
		steps: []string{
			stepActivarEdicion,
			"En la sección inicial, pulsa «Añadir una actividad o un recurso» y elige «Foro».",
			"Nómbralo «Foro de Dudas e Inquietudes», tipo «Foro para uso general», y guarda.",
		},
		create:       "el foro «Foro de Dudas e Inquietudes»",
		evidenceHint: "una captura del curso o del foro donde se vea el «Foro de Dudas e Inquietudes» disponible.",
	},
	"9.1.3": {
		headline:    "Configura las fechas del foro temático",
		requirement: "El foro temático tiene configurada su fecha de inicio y de fin (vencimiento y fecha límite).",
		location:    locForoTematico + " → Editar ajustes → Disponibilidad",
		steps: []string{
			"Abre el foro temático y pulsa «Editar ajustes».",
			"En «Disponibilidad», activa y completa «Fecha de entrega» y «Fecha límite».",
			stepGuardar,
			"Comprueba que las fechas se vean arriba en la página del foro.",
		},
		create:       "el foro temático de la actividad",
		evidenceHint: "una captura de la página del foro temático donde se vean sus fechas de apertura y cierre.",
	},
	"9.1.4": {
		headline:    "Abre el foro temático en las fechas del cronograma",
		requirement: "El foro temático está abierto en las fechas establecidas en el cronograma.",
		location:    locForoTematico + " → Editar ajustes → Disponibilidad",
		steps: []string{
			"Revisa en el cronograma de la fase las fechas del foro temático.",
			"En «Editar ajustes» del foro, ajusta la disponibilidad a esas fechas y deja el foro visible.",
			stepGuardar,
		},
		create:       "el foro temático de la actividad",
		evidenceHint: "una captura del foro temático donde se vean las fechas, que deben coincidir con el cronograma.",
	},
	"9.1.5": {
		headline:    "Responde las dudas del foro de dudas",
		requirement: "Cada duda del Foro de Dudas e Inquietudes tiene respuesta del instructor en máximo un día hábil.",
		location:    locForoDudas,
		steps: []string{
			"Abre el Foro de Dudas e Inquietudes y entra en cada debate sin respuesta.",
			"Pulsa «Responder» y contesta la duda. Tu respuesta debe quedar como el último mensaje del debate.",
			"Hazlo en máximo un día hábil desde que el aprendiz publica.",
		},
		create:       "el foro «Foro de Dudas e Inquietudes»",
		evidenceHint: "una captura de la lista de debates del foro donde se vea que el último mensaje de cada debate es tuyo, con su fecha.",
		template:     doubtReplyTemplate,
	},
	"9.1.6": {
		headline:    "Responde a los aprendices en los foros temáticos",
		requirement: "El instructor responde las participaciones de los foros temáticos en máximo un día hábil.",
		location:    locForoTematico,
		steps: []string{
			"Abre el foro temático de la actividad y entra en cada debate de los aprendices.",
			"Pulsa «Responder» y contesta. Tu mensaje debe quedar como el último de cada debate.",
			"Hazlo en máximo un día hábil.",
		},
		create:       "el foro temático de la actividad",
		evidenceHint: "una captura de la lista de debates del foro temático donde se vea que el último mensaje es tuyo, con su fecha.",
		template:     forumFeedbackTemplate,
	},
	"9.1.7": {
		headline:    "Publica la retroalimentación de los foros temáticos",
		requirement: "Los foros temáticos tienen la retroalimentación del instructor a los aportes de los aprendices.",
		location:    locForoTematico,
		steps: []string{
			"Abre el foro temático y entra en cada debate de los aprendices.",
			"Pulsa «Responder» y escribe la retroalimentación: qué hizo bien y qué puede mejorar.",
			"Comprueba que tu respuesta quede como último mensaje del debate.",
		},
		create:       "el foro temático de la actividad",
		evidenceHint: "una captura de un debate del foro temático donde se vea tu retroalimentación al aprendiz.",
		template:     forumFeedbackTemplate,
	},

	"10.1.1": {
		headline:    "Califica y retroalimenta las evidencias entregadas",
		requirement: "Las evidencias entregadas tienen calificación y comentario de retroalimentación.",
		location:    locEntregas,
		steps: []string{
			"Abre la actividad de evidencia y pulsa «Ver todas las entregas».",
			"Pulsa «Calificar» en cada entrega, asigna la calificación y escribe el comentario de retroalimentación.",
			"Guarda. La entrega debe quedar en estado «Calificado».",
		},
		create:       "la actividad de evidencia que seleccionaste en Actividades",
		evidenceHint: "una captura de «Ver todas las entregas» donde se vean las entregas en estado «Calificado» con su comentario.",
	},
	"10.1.2": {
		headline:    "Retroalimenta las evidencias en máximo tres días hábiles",
		requirement: "La retroalimentación de cada evidencia se hace en máximo tres días hábiles desde la entrega.",
		location:    locEntregas,
		steps: []string{
			"Abre la actividad y ordena las entregas por fecha.",
			"Califica y retroalimenta las que llevan más tiempo sin calificar, en máximo tres días hábiles desde la entrega.",
		},
		create:       "la actividad de evidencia que seleccionaste en Actividades",
		evidenceHint: "una captura de «Ver todas las entregas» donde se vean la fecha de entrega y la fecha de calificación.",
	},

	"11.1.1": foroAnuncio("Nombra la fase en el anuncio de inicio", "El anuncio de inicio de fase indica el nombre de la fase del proyecto.",
		[]string{stepAnuncioNuevo, "Escribe el anuncio de inicio de fase con el nombre de la fase en el asunto y en el mensaje.", stepEnviarForo},
		"una captura del anuncio de inicio de fase donde se vea el nombre de la fase.", phaseStartTemplate),
	"11.1.2": foroAnuncio("Agrega las fechas de la fase al anuncio de inicio", "El anuncio de inicio de fase indica la fecha de inicio y finalización de la fase.",
		[]string{stepAnuncioNuevo, "Incluye en el anuncio de inicio de fase la fecha de inicio y la de finalización.", stepEnviarForo},
		"una captura del anuncio de inicio de fase donde se vean las fechas.", phaseStartTemplate),
	"11.1.3": foroAnuncio("Indica qué consultar en el anuncio de inicio", "El anuncio de inicio de fase indica qué materiales y recursos consultar.",
		[]string{stepAnuncioNuevo, "Incluye en el anuncio qué materiales, guías y recursos deben consultar los aprendices.", stepEnviarForo},
		"una captura del anuncio de inicio de fase donde se vea qué consultar.", phaseStartTemplate),
	"11.1.4": foroAnuncio("Agrega los pasos a seguir al anuncio de inicio", "El anuncio de inicio de fase indica los pasos a seguir para el desarrollo de la fase.",
		[]string{stepAnuncioNuevo, "Incluye en el anuncio los pasos a seguir, numerados.", stepEnviarForo},
		"una captura del anuncio de inicio de fase donde se vean los pasos a seguir.", phaseStartTemplate),
	"11.2.1": foroAnuncio("Publica el anuncio de inicio de la actividad de proyecto", "Hay un anuncio de inicio por cada actividad de proyecto.",
		[]string{stepAnuncioNuevo, "Escribe el anuncio de inicio de la actividad de proyecto con su nombre, fechas y evidencias.", stepEnviarForo},
		"una captura del anuncio de inicio de la actividad de proyecto.", activityStartTemplate),
	"11.2.2": foroAnuncio("Publica el anuncio de cierre de la actividad de proyecto", "Hay un anuncio de cierre por cada actividad de proyecto.",
		[]string{stepAnuncioNuevo, "Escribe el anuncio de cierre con la fecha de cierre y las evidencias pendientes.", stepEnviarForo},
		"una captura del anuncio de cierre de la actividad de proyecto.", activityCloseTemplate),
	"11.2.3": foroAnuncio("Publica el anuncio semanal de la sesión en línea", "Cada semana hay un anuncio de invitación a la sesión en línea.",
		[]string{stepAnuncioNuevo, "Indica fecha, hora, enlace y tema de la sesión en línea de la semana.", stepEnviarForo},
		"una captura del anuncio de invitación a la sesión en línea de la semana.", weeklySessionTemplate),
	"11.3": foroAnuncio("Publica el anuncio de aprendices aprobados de la fase", "Al terminar la fase se publica el anuncio con los aprendices aprobados.",
		[]string{stepAnuncioNuevo, "Publica la lista de aprendices que aprobaron la fase.", stepEnviarForo},
		"una captura del anuncio de aprendices aprobados de la fase.", approvedTemplate),
	"11.4": foroAnuncio("Da formato comunicativo a los anuncios", "Los anuncios tienen función comunicativa: saludo, propósito claro, información completa y firma.",
		[]string{"Abre el foro de Anuncios y revisa los anuncios publicados.", "Edita los que no tengan saludo, propósito claro, información completa o firma (pulsa «Editar» en el mensaje).", "Guarda los cambios."},
		"una captura de la lista del foro de Anuncios donde se vean anuncios con su asunto y formato.", nil),

	"12.1.1": {
		headline:    "Publica la grabación de la sesión semanal",
		requirement: "La grabación de cada sesión semanal está publicada en la subsección del mes.",
		location:    locSesiones + " → fase → mes",
		steps: []string{
			stepActivarEdicion,
			"Abre la subsección del mes en «Sesiones en Línea».",
			"Pulsa «Añadir una actividad o un recurso», elige «URL» y pega el enlace de la grabación de la semana que falta (nómbralo «Grabación sesión [fecha]»).",
			stepGuardar,
		},
		create:       "la subsección del mes dentro de «Sesiones en Línea»",
		evidenceHint: "una captura de la subsección del mes donde se vea la grabación de cada semana publicada.",
	},
	"12.1.2": {
		headline:    "Publica el resumen de la sesión semanal",
		requirement: "El resumen de cada sesión semanal está publicado en la subsección del mes.",
		location:    locSesiones + " → fase → mes",
		steps: []string{
			stepActivarEdicion,
			"Abre la subsección del mes en «Sesiones en Línea».",
			"Añade un recurso «Página» (o «Archivo») con el resumen de la sesión que falta, llamado «Resumen sesión [fecha]».",
			stepGuardar,
		},
		create:       "la subsección del mes dentro de «Sesiones en Línea»",
		evidenceHint: "una captura de la subsección del mes donde se vea el resumen de cada sesión publicado.",
		template:     sessionSummaryTemplate,
	},

	"13.1.1": seguimientoDoc("Publica 2 actas mensuales de reuniones EEF", "La subsección «Reuniones EEF – Actas» tiene 2 actas por mes.", locSeguimientoForm, "Reuniones EEF – Actas", "las 2 actas EEF del mes"),
	"13.1.2": seguimientoDoc("Publica el acta de comité al terminar la fase", "La subsección «Comités evaluativos – Actas» tiene mínimo un acta al terminar cada fase.", locSeguimientoForm, "Comités evaluativos – Actas", "el acta de comité de la fase terminada"),
	"13.1.3": seguimientoDoc("Publica un documento en Documentos de retención", "La subsección «Documentos de retención» tiene al menos un documento.", locSeguimientoForm, "Documentos de retención", "un documento de retención"),
	"13.2.1": seguimientoDoc("Publica la copia de calificaciones al 100 %", "Al final de la fase, el «Reporte de Curso» tiene la copia de calificaciones al 100 %.", locReporteCurso, "Reporte de Curso", "la copia de las calificaciones de la fase al 100 % (exporta el calificador en Calificaciones → Exportar)"),
	"13.2.2": seguimientoDoc("Publica los formatos de cierre de la fase", "Al final de la fase, el «Reporte de Curso» tiene los formatos de cierre.", locReporteCurso, "Reporte de Curso", "los formatos de cierre de la fase"),

	"14.1.1": {
		headline:    "Publica la conclusión del foro temático",
		requirement: "El instructor publica la conclusión del foro temático en la fecha del cronograma o al día siguiente.",
		location:    locForoTematico,
		steps: []string{
			"Abre el foro temático cuando termine según el cronograma (o al día siguiente).",
			"Pulsa «Añadir un nuevo tema de debate».",
			"Pon un asunto que contenga la palabra «Conclusión» y pega el texto sugerido completado.",
			"Pulsa «Enviar al foro».",
		},
		create:       "el foro temático de la actividad",
		evidenceHint: "una captura de la lista de debates del foro temático donde se vea tu debate «Conclusión…» con su fecha.",
		template:     conclusionTemplate,
	},
	"14.1.2": {
		headline:    "Publica la conclusión como un debate nuevo",
		requirement: "La conclusión del foro temático se publica como un tema de debate nuevo, no como respuesta.",
		location:    locForoTematico,
		steps: []string{
			"Abre el foro temático y pulsa «Añadir un nuevo tema de debate». No la publiques como respuesta dentro de otro debate.",
			"El asunto debe contener la palabra «Conclusión», por ejemplo «Conclusión del foro temático».",
			"Pega el texto sugerido completado y pulsa «Enviar al foro».",
		},
		create:       "el foro temático de la actividad",
		evidenceHint: "una captura de la lista de debates del foro temático donde se vea la conclusión como tema propio.",
		template:     conclusionTemplate,
	},
}
