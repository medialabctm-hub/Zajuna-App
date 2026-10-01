# API AJAX de Zajuna (2026-10-01)

Zajuna es Moodle 4.3.x bajo `/zajuna/`. Hay dos APIs y la app solo puede usar
una:

- **REST con token: no se puede usar.** `/zajuna/login/token.php` y
  `/webservice/rest/server.php` existen, pero el servicio móvil está apagado
  (`enablemobilewebservice=0`, leído de `tool_mobile_get_public_config`). Sin
  ese servicio, Zajuna no entrega tokens.
- **AJAX interno: sí está activo.** Es `/zajuna/lib/ajax/service.php?sesskey=…`,
  el mismo que usan las páginas de Zajuna. Funciona con la cookie de sesión
  más el `sesskey`, que el core lee de `M.cfg` en `my/courses.php` al iniciar
  sesión. Solo responde las funciones marcadas `ajax => true` y con los
  permisos del instructor.

No hay documentación pública del SENA sobre web services ni una app móvil
oficial de Zajuna.

## Implementación

- `zajuna.Session` guarda `Sesskey` y `UserID`, solo en memoria, igual que las
  cookies.
- El `UserID` no viene en `M.cfg` de Zajuna, y `my/courses.php` menciona dos
  ids distintos.
  - `ResolveUserID` pide todos los candidatos en una llamada a
    `core_user_get_users_by_field` y se queda con el que tiene el mismo nombre
    que la sesión.
  - Si hay cualquier duda, queda en 0 y no se hacen las verificaciones de
    foro: un id equivocado ocultaría las respuestas del instructor.
- `Client.CallAJAX` envía una función por petición. Un error con código
  `servicerequireslogin` o `invalidsesskey` es `ErrSessionExpired`; cualquier
  otro fallo es `ErrAJAXUnavailable`, y quien llama vuelve al método anterior.
- `capture-checklist`, antes de abrir Chromium para un target, ejecuta una
  verificación de contenido (`workers/capture_checklist_ajax.go`,
  `checklist.ContentCheckForItem`).
  - Solo decide **ausente** cuando la API lo confirma. El slot queda
    «sin contenido en Zajuna», igual que con `capture.ErrContentAbsent`.
  - Si la API falla o no puede decidir, la captura sigue como antes.
  - Las respuestas se comparten entre los targets de una corrida, con clave
    por `sesskey`.

| Ítems | Verificación | Función |
|---|---|---|
| 7.2, 7.3.x, 7.4.x, 13.x (subsecciones con nombre) | Ninguna subsección con ese título tiene actividades, archivos ni resumen (`hassummary=false` explícito), incluidas sus subsecciones hijas (`children` unido con `parentid`) | `core_courseformat_get_state {courseid}` |
| 9.1.5, 9.1.6, 9.1.7 | El instructor no respondió a nadie en el foro. Si hay respuestas, el navegador sigue exigiendo que su mensaje sea el último | `mod_forum_get_discussion_posts_by_userid {userid, cmid}` |
| 14.1.1, 14.1.2 | Ninguna publicación del instructor (debate o respuesta) tiene «conclusión» en el asunto | ídem |
| 9.1.3, 9.1.4 | La tarjeta del foro no tiene la región `activity-dates` ni ninguna de las etiquetas que acepta el navegador (`checklist.ForumDateLabels`) | `core_course_get_module {id: cmid}` |

De los mensajes del foro la app solo guarda asuntos, fechas y si cada uno es
una respuesta. Nunca lee el texto de los mensajes ni los nombres de los
aprendices.

## Validación con la cuenta real (ficha 3135429, curso 41080)

La herramienta `go -C core run ./cmd/zajuna-probe` (de desarrollo, no se
empaqueta) inicia sesión con la cuenta guardada por la app y muestra solo la
forma de cada respuesta: claves, tipos y cantidades.

| Función | Resultado |
|---|---|
| `core_courseformat_get_state` | Disponible. 156 secciones jerárquicas (`parentid`, `children`, `cmlist`) y 436 actividades. «Documentos de retención de aprendices» y «Reuniones EEF - Actas / Marzo» tienen 0 contenidos. |
| `mod_forum_get_discussion_posts_by_userid` | Disponible. Una llamada por foro devuelve los mensajes del instructor y a qué mensaje responde cada uno. |
| `mod_forum_get_discussion_posts` | Disponible (autor, `parentid`, asunto y fecha por mensaje). |
| `core_course_get_module` | Disponible. Para «Foro temático. AA2-EV01» devuelve «Vencimiento: …»; para «Foro Temático», ninguna fecha. |
| `core_calendar_get_action_events_by_course` | Disponible: 2 actividades con 5 entregas por calificar. |
| `core_calendar_get_calendar_monthly_view` | Disponible. Sin usar. |
| `core_grades_get_grade_tree` | Disponible: 136 ítems de calificación. |
| `core_user_get_users_by_field` | Disponible. Confirma el id del instructor a partir de los candidatos de la página. |
| `core_webservice_get_site_info` | No disponible por AJAX (`servicenotavailable`), como se esperaba. |

Corrida real con «Ya lo hice, verificar» sobre 7.3.2, 13.1.1, 14.1.1 y 9.1.6.
Los cuatro ítems quedaron confirmados como ausentes por la API, sin abrir
Chromium:

- 7.3.2 tardó unos 5 s y 13.1.1 unos 8 s, login incluido.
- 9.1.7 y 14.1.2 heredaron la ausencia porque comparten el target.

Una revisión independiente pidió dos cosas, ya incorporadas:

- La verificación AJAX solo marca «ausente» lo que la captura con el
  navegador también rechazaría: se tienen en cuenta el resumen de la sección,
  las mismas etiquetas de fecha y el marcador `activity-dates`.
- El `sesskey` no aparece en ningún texto de error.

## Otras fuentes que solo necesitan la sesión

Ninguna depende del SENA, de tokens ni de plugins: son funciones de Moodle que
cualquier instructor tiene, más las hojas de Google que ya están publicadas.
Todas se validaron con la cuenta real (`zajuna-probe -feature …`):

| Fuente | Endpoint | Validación (curso 41080) | Uso en la app |
|---|---|---|---|
| Índice de foros | `GET /mod/forum/index.php?id=<curso>` | 14 foros, cada uno con su número de debates | Respaldo para encontrar el id de un foro por su nombre cuando la página del foro no lo expone. |
| Id de foro | `data-forumid` en `mod/forum/view.php?id=<cmid>` | Encontrado | Id que necesita la exportación. |
| Exportación de foros | `POST /mod/forum/export.php` (`format=json`) | 35 y 87 mensajes, con autor y respuesta a la que contesta | 9.1.5–9.1.7: «hay N aportes de aprendices sin respuesta tuya, el más antiguo del …». |
| Historial de calificaciones | `GET /grade/report/history/index.php?id=<curso>&showreport=1&download=csv` | 16 025 eventos; 3835 con calificación y 1521 con retroalimentación | 10.1.x: calificaciones y retroalimentaciones registradas para la actividad. |
| Entregas por calificar | `core_calendar_get_action_events_by_course` | 5 entregas en 2 actividades | 10.1.x: «Zajuna indica N entregas por calificar». |
| Árbol de calificaciones | `core_grades_get_grade_tree` | 136 ítems | 5.1: un calificador sin ítems es «ausente». |
| Cronogramas (Google Sheets publicados) | `…/d/e/<id>/pub?gid=…&single=true&output=csv` de la pestaña incrustada en la página de la actividad | Fase Planear y Fase Hacer tienen una celda `#REF!`. El Cronograma General responde 400 con su `gid` y no se revisa: leer otra pestaña daría errores ajenos | 1.x: aviso `sheet_errors` en Revisión, sin bloquear (la evidencia y el ítem siguen cumplidos), y una recomendación en Guías: una sola para todos los 1.2.x, que comparten la captura, con cada hoja con error y su enlace. |

Reglas comunes:

- Ninguna fuente puede hacer fallar una captura.
- La de verificación (5.1) solo decide «ausente» si la respuesta es
  inequívoca: un árbol sin lista de ítems es «no disponible», no vacío.
- 6.1 (fechas límite) no se decide por AJAX: la tarjeta del curso solo
  muestra fechas si el curso activa «Mostrar fechas de actividad», así que su
  ausencia no prueba nada.
- Las de contexto (foros, calificaciones) solo completan el motivo que ve el
  instructor; nunca cambian la decisión.
- De la exportación de foros y del historial solo quedan ids, fechas y
  conteos: nombres, correos y textos se descartan al leerlos.
- El CSV de las hojas se pide solo a `docs.google.com` por HTTPS, sin cookies.

## Fuera de alcance sin el SENA

- REST con token: requiere que el administrador active un servicio externo.
- plugNmeet (sesiones en línea), Microsoft Graph (Teams) y un plugin propio
  en Zajuna: requieren credenciales o aprobación institucional.
- RSS de foros: desactivado en Zajuna.
- iCal: el token de exportación es un secreto por usuario.
