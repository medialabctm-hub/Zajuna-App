# Auditoría de evidencias — ficha 3135429 (2026-09-24)

Revisión manual, una por una, de las capturas de la corrida completa de la
versión 0.1.5 sobre el curso ANIMACIÓN 3D (3135429). El objetivo era saber
cuántas capturas prueban de verdad el ítem al que están asignadas y corregir
los fallos del revisor automático.

Contexto: [API local](../api-local.md) (sección «Revisión de evidencias») ·
[release 0.1.5](../release-0.1.5.md) ·
[verificación MDL-124](../mdl-124-verify-2026-09-14.md).

## Punto de partida

La corrida `capture-checklist` terminó sin fallos: 101 unidades, 68 capturas,
30 omitidas (lotes de filas vacíos), 3 ausentes (9.1.6, 10.1.1, 14.1.1).

| Estado en la app | Evidencias | Archivos |
|---|---|---|
| Aprobadas | 108 | 61 |
| Pendientes | 8 | 7 |

Las 116 evidencias salen de 68 archivos y 63 contenidos distintos: una misma
captura cubre varios ítems (`coveredItemCodes`).

## Resultado de la revisión manual

60 de 68 capturas son efectivas: 59 seguras y 1 dudosa (9.1.5). Corresponden
a 58 contenidos distintos.

**Correctas.** Cronogramas (1.1.1, 1.2.x), perfil (2.1.x), material (3.1),
menú (4.1), calificaciones (5.1), fechas límite (6.1), Seguimiento y
Evaluación (7.x), Sesiones en línea (8.x), calificación (10.1.1), anuncios
(11.x), grabaciones (12.1.x), comités y reporte del curso (13.x).

**Bien detectadas como pendientes.** 7.3.2, 7.3.3, 13.1.1, 13.1.3 y 12.1.1
slot 4: son secciones vacías también en Zajuna.

**Aprobadas por error:**

- 9.1.6 y 14.1.1 (slot 3, la misma imagen): un único debate «Apertura del
  FORO», abierto por el instructor y con 0 réplicas. No prueba que el
  instructor responda (9.1.6) ni que haya conclusión (14.1.1). El trabajo
  marcaba esos ítems como ausentes, pero la evidencia de una corrida anterior
  seguía aprobada.
- 9.1.3 slot 2 («Foro Temático»): no muestra fechas, así que no prueba la
  configuración con fecha de inicio y fin.
- 9.1.5 (dudosa): en uno de los dos debates el último mensaje es de un
  aprendiz.

**Pendientes por error.** 11.4 slot 1 y 15.1 se marcaron como duplicadas
entre sí, aunque son dos ítems sobre el mismo foro de anuncios. Una URL
llevaba `forceview=1` y la otra no.

**Incompletas.** Los cronogramas se cortaban al alto fijo del iframe (2400
px): el Cronograma General solo mostraba Inducción y parte de Planear.

**Contenido de Zajuna, no de la app.**

- El Cronograma Fase Planear muestra `#¡REF!` en «Fecha fin fase».
- La columna «Comentarios de retroalimentación» de las 4 calificaciones está
  vacía.

## Correcciones

| Fallo | Causa | Corrección |
|---|---|---|
| Foros «responde» aprobados sin respuestas | El filtro de filas solo miraba quién inició el debate. | `RowRequireReply`: solo debates con réplicas cuyo «Último mensaje» es del instructor. Las columnas se localizan por el texto del encabezado. |
| 14.1.x sin conclusión | No había filtro de tema. | `RowMatch` «conclusion». |
| 9.1.3/9.1.4 sin fechas | Se aceptaba toda la región del foro. | `ForumDatesSelector` exige fechas visibles y no tiene selectores de respaldo. |
| La evidencia vieja sobrevivía a una ausencia | La limpieza trataba «ausente» como «fallido» y conservaba el slot. | `captureChecklistPrunePlan` retira la evidencia de los slots ausentes. |
| 11.4 / 15.1 marcadas como duplicadas | Comparación literal de URL. | La URL se compara sin `forceview`, sin fragmento y sin importar el orden de los parámetros. |
| Evidencias antiguas seguían aprobadas | El revisor no sabía con qué regla se capturaron. | La metadata guarda `semanticCheck`; si falta o es otra, queda pendiente con `outdated_rule`. |
| Cronograma cortado | Alto fijo del iframe; además el widget de Google muestra cada pestaña en un iframe anidado (`#pageswitcher-content`), así que buscar `table.waffle` en el widget nunca encontraba la tabla (y la espera de 8 s fallaba siempre). | Se mide dentro del iframe anidado y el widget crece al alto real de la hoja (entre 2400 y 8400 px). |

Con el revisor nuevo, sobre una copia de la base real, quedan 101 aprobadas y
15 pendientes. 11.4 y 15.1 pasan a aprobadas. Las evidencias de foros
capturadas con la regla anterior (9.1.3–9.1.7 y 14.1.x) quedan pendientes con
`outdated_rule` hasta volver a capturar. Las 5 secciones vacías siguen
pendientes.

## Verificación

- `go -C core test ./...` y `go -C core vet ./...`.
- Smoke con Chromium real (`ZAJUNA_RUN_BROWSER_SMOKE=1`):
  `TestForumReplyFilterSmoke` y `TestForumDatesSelectorSmoke`. El primero
  encontró y corrigió un error en el script de filas antes de publicarlo.

## Segunda corrida (misma fecha, con las correcciones)

Corrida completa desde «buscar rutas». La captura terminó con 65 capturas, 6
ausencias, 0 fallos y 6 evidencias obsoletas retiradas (antes 0). El revisor
queda en 104 aprobadas, 6 pendientes y 4 ítems sin evidencia:

- 9.1.5: debates abiertos por aprendices («reporte») con 1 réplica y último
  mensaje del instructor. Es evidencia correcta.
- 9.1.6, 9.1.7: ausentes, porque no hay foros temáticos respondidos por el
  instructor. Se retiró la captura vieja.
- 14.1.1, 14.1.2: ausentes, porque no hay un debate de conclusión. Se retiró
  la captura vieja.
- 9.1.3/9.1.4: solo el foro con «Vencimiento» (AA2-EV01). «Foro Temático», que
  no tiene fechas, queda ausente.
- 11.4 y 15.1: aprobadas.
- 6 pendientes: 7.3.2, 7.3.3, 12.1.1/12.1.2 slot 4, 13.1.1 y 13.1.3, que son
  secciones vacías también en Zajuna.

La corrida mostró que los cronogramas seguían cortados (2642 px): la primera
versión del ajuste medía el documento del widget y no el iframe anidado. Con
la corrección, al recapturar solo 1.1.1 y 1.2.1 el Cronograma General sale de
5892 px, de Inducción a Actuar, y las fases de 2678 a 3601 px, completas.
`TestEmbeddedSheetGrowsToItsNestedContentSmoke` reproduce el widget anidado.

## Tercera corrida: capturas no reproducibles

La tercera corrida completa salió sin fallos, pero 3.1 y 4.1 cambiaban de
contenido y de tamaño en cada corrida. 3.1 midió 5650, 5213 y 3757 px en las
tres corridas; en la última no mostraba la subsección «Evidencias», que es
justo lo que pide el ítem. Hubo tres causas:

| Fallo | Causa | Corrección |
|---|---|---|
| Secciones abiertas distintas en cada corrida | `prepareCourseMenu` hacía clic en hasta 40 botones de plegar de Moodle. Moodle guarda cada clic como preferencia del usuario, así que se cambiaba la vista del curso del instructor en Zajuna y cada captura dependía de las anteriores (y del orden de los 2 workers). | Ningún clic: las secciones se abren o cierran solo en el DOM. `CourseLayout` fija el estado: 4.1 `menu` (todo cerrado) y 3.1 `first-section` (la primera sección después de la cabecera, con todo su contenido). |
| 3.1 abría la cabecera (sección 0) en lugar de Inducción | La sección 0 también tiene panel plegable. | Se salta `section-0`, que queda abierta. |
| Ancho distinto en cada corrida (2000 o 976 px) | La sesión reutiliza la página: los cronogramas ponían la ventana a 2560 px y el resto de capturas heredaba ese tamaño, o el de 1280 px por defecto. | Toda captura fija su ventana; la de por defecto es 2560×1200. |
| El login falló tras dos capturas seguidas | El aviso de conexión lenta de Zajuna (`#connection-guard-modal`, que el propio script declara que «nunca bloquea el login») tapaba el botón «Iniciar sesión» y el clic esperaba 30 s. | Una regla CSS oculta el aviso antes de enviar el formulario. |

Resultado: dos capturas seguidas de 3.1, 4.1, 7.2, 8.1 y 12.1.1 dan
exactamente los mismos bytes. La ficha completa queda en 65 capturas, 0
fallos, 104 aprobadas, 6 pendientes (secciones vacías) y 4 ítems sin
evidencia (9.1.6, 9.1.7, 14.1.1 y 14.1.2), que no tienen contenido en Zajuna.
Los nuevos duplicados exactos (8.1/8.2/8.3, 7.2/13.2.x, 7.3.1/7.4.1/13.1.2)
son ítems que apuntan a propósito a la misma sección.

Smoke tests nuevos: `TestCourseLayoutNeverClicksMoodleTogglesSmoke`, que
falla si se guarda alguna preferencia, y `TestLoginNoticeDoesNotBlockSubmitSmoke`.

## Revisión independiente (sesión de integración)

| Hallazgo | Corrección |
|---|---|
| Media: la ausencia se deducía del texto del error, así que una página de error o de permisos se tomaba como ausencia y podaba evidencia válida. | Error tipado `capture.ErrContentAbsent`, emitido solo cuando la página correcta cargó: la lista existe pero no hay filas del instructor o respuestas, o el foro renderizado no muestra fechas (`AbsenceSelector`). Solo esa señal retira evidencia; 10.1.x solo se informa. |
| Media: `VerifyFicha` podía pisar una decisión manual guardada mientras verificaba. | El upsert no reemplaza una revisión `manual` con una `auto` del mismo `sha256`; «pending» borra la manual de forma explícita. |
| Baja: con una selección solo transversal, todos los foros ligados a actividades pasaban el filtro. | `eligibleRouteForGroup` recibe `selectionGiven`: sin actividad técnica elegida, solo pasan los foros genéricos. |
| Baja: 6.1 sin actividades técnicas no se contaba en el resumen del plan. | Cuenta como no resuelto. |

## Riesgo detectado

El core no bloquea su carpeta de datos. Electron impide dos instancias de la
app, pero un core de desarrollo y la app instalada pueden arrancar a la vez
sobre `AppData\Local\ZajunaApp`. En ese caso el segundo, al iniciar
(`ReconcileInterrupted`), marca como interrumpidos y reintenta los trabajos
del primero. Ocurrió en la segunda corrida con «buscar rutas», que terminó
bien en el reintento.

## Pendiente

- Recapturar la ficha 3135429 para confirmar en Zajuna real:
  - que 9.1.5–9.1.7 muestran debates respondidos, o quedan ausentes;
  - que 14.1.x quedan ausentes si no hay un debate titulado «Conclusión»;
  - que los cronogramas salen completos.
- 14.1.2 pide la conclusión «publicada como respuesta nueva». Una respuesta
  dentro de un debate no aparece en la lista de debates, así que la regla
  actual solo encuentra debates titulados como conclusión.
