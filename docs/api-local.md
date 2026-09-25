# API local de Zajuna App

La API se sirve desde el core Go en el mismo origen que la interfaz y escucha
exclusivamente en `127.0.0.1`. No usa JWT, login propio ni CORS abierto para la
interfaz integrada.

## Protección del origen local

Invariantes que cualquier versión del core mantiene:

- El servidor escucha solo en loopback y toda petición con `Host` que no sea
  loopback (`127.0.0.1`, `::1`, `localhost`) se rechaza con `400`.
- Las mutaciones (`POST`, `PUT`, `PATCH`, `DELETE` bajo `/api/`) exigen una
  credencial local emitida por el propio proceso, que cambia en cada arranque
  y solo se obtiene a través del lanzador.
- Las mutaciones cross-site o con `Origin` distinto del core se rechazan.
- Los cuerpos con datos declaran `application/json` (o `multipart/form-data`
  en `/api/evidences/upload`) y tienen límite de tamaño; el servidor aplica
  timeouts y `MaxHeaderBytes`.

Implementación vigente:

Al iniciar, el core genera por proceso tres secretos: el de la cookie de
sesión, el de la cabecera de sesión y el del lanzador. Ninguna respuesta
normal emite cookies: la sesión solo se obtiene con un enlace de un solo uso.

1. El lanzador Electron crea `ZAJUNA_LAUNCHER_SECRET` y se lo pasa al core en
   su entorno (el core lo borra de su entorno al arrancar para que Chromium no
   lo herede).
2. En cada lanzamiento (primero, segunda instancia, recuperación del core) el
   lanzador llama `POST /api/session/bootstrap` con
   `Authorization: Bearer <secreto>` y recibe `{"path":"/api/session/start?token=..."}`.
   El token vence a los 2 minutos, se usa una sola vez y hay como máximo 32
   pendientes.
3. El navegador abre esa URL. `GET /api/session/start` canjea el token, emite
   la cookie `zajuna_capability_<puerto>` (`HttpOnly`, `SameSite=Strict`) y
   redirige con `303` a `/#zc=<secreto de cabecera>`. El fragmento nunca viaja
   a un servidor; React lo guarda en `localStorage` (aislado por origen, es
   decir, por puerto) y lo borra de la URL.
4. Cada llamada de React envía la cookie y la cabecera `X-Zajuna-Capability`.

Todo `/api/*` exige cookie y cabecera, lecturas incluidas, excepto:

- `GET /api/health`, sin sesión y con la respuesta mínima `{"status":"ok"}`.
- `GET /api/{evidences,reports,backups}/{id}/download` y
  `GET /api/evidences/{id}/thumbnail`, que solo exigen la cookie porque se
  cargan con `<img>`/`<a href>`.

Las cookies no se aíslan por puerto: otro proceso escuchando en
`127.0.0.1:<otro puerto>` podría recibirlas si el navegador lo visita. Por eso
la cookie sola no permite leer JSON ni mutar; como mucho permitiría descargas.
Sin sesión la API responde `401` con `"code":"local_session_required"`, y la
interfaz pide volver a abrir la app desde su acceso directo. Un core
standalone (sin supervisor) abre el navegador con su propio enlace; con
`--no-browser` lo escribe en el log.

`POST`, `PUT`, `PATCH` y `DELETE` requieren además:

- `Host` loopback y `Origin` coincidente cuando el navegador lo envía.
- `Sec-Fetch-Site` que no sea `cross-site` ni `same-site` (todos los puertos
  de `127.0.0.1` son el mismo sitio); esto se aplica a todo `/api/*`.
- `Content-Type` JSON para cuerpos JSON o `multipart/form-data` para uploads.
- Límites de tamaño, headers y timeouts del servidor.

Los helpers de tests que crean `newRouterWithServices` prueban handlers sin
middleware para aislar cada caso; el runtime real de `main` siempre registra
`protectLocalAPI`, cubierto por las pruebas de caja negra de
`api_security_test.go`.

Las rutas de captura solo aceptan el origen Zajuna configurado en producción,
rechazan loopback/IP privadas y validan redirects y URL final. Los errores,
eventos y metadata pasan por redacción de tokens, cookies y parámetros
sensibles antes de responder o persistir.

## Salud y configuración

### `GET /api/health`

Sonda de vida sin sesión: devuelve solo `{"status":"ok"}`. La versión y la
carpeta de datos están en `GET /api/app/info`, que exige sesión.

### `GET /api/setup/status`

Devuelve si la configuración inicial está completa y el usuario configurado.
Nunca devuelve la contraseña.

### `POST /api/setup`

```json
{
  "zajunaUsername": "123456789",
  "zajunaDocumentType": "CC",
  "zajunaPassword": "••••••••"
}
```

La contraseña se guarda en el almacén seguro del sistema operativo. La
configuración no sensible se guarda localmente.

### `GET /api/app/info`

Devuelve `version`, `dataDir` (carpeta de datos local), `supervised` (si el
core corre bajo el launcher Electron) y `resetPending` (si hay un
restablecimiento preparado para el próximo arranque).

### `POST /api/app/reset`

```json
{ "backupFirst": true, "forgetCredentials": false }
```

Prepara el restablecimiento completo de los datos locales. No borra nada en
caliente: escribe el marcador `.reset-pending` y apaga el core; el borrado se
aplica en el siguiente arranque, antes de abrir SQLite. Con `backupFirst`
crea antes una copia ZIP (si falla, responde `500` y no prepara nada) y
devuelve su nombre en `backupName`; esa copia se conserva tras el reset. Con
`forgetCredentials` borra la contraseña guardada en el almacén del sistema.
Responde `{ "staged": true, "restarting": <supervised> }`. Ver
[`evidence/update-policy.md`](evidence/update-policy.md).

### `POST /api/zajuna/test-connection`

Encola una prueba real de autenticación contra Zajuna y validación de `Mis
cursos`. No devuelve ni persiste cookies o contraseñas; el resultado se
consulta como cualquier otro job. Con cuerpo vacío (`{}`) usa el usuario y el
tipo de documento guardados en la configuración local.

Configuración › Cuenta Zajuna ofrece la acción **Probar conexión**. Guardar
credenciales (`setupComplete`) solo las marca como *Configurada*; el estado
*Verificada* aparece únicamente cuando el job `test-zajuna-connection` termina
en `completed` (con el número de fichas de `result.fichas`), y un job `failed`
muestra el motivo y el siguiente paso. La interfaz recuerda el id del último
job en este equipo y lo olvida al guardar credenciales nuevas.

La sesión HTTP envía `documentType` (por defecto `CC`) y tolera las dos
variantes actuales del formulario de Zajuna: `logintoken` solo, o
`logintoken` acompañado de `josso`. Ningún token o cookie se devuelve por la
API ni se persiste localmente.

```json
{
  "username": "123456789",
  "documentType": "CC"
}
```

### `POST /api/course-maps/discover`

Encola el descubrimiento local de rutas para los cursos sincronizados. Si no
se envía `courseIds`, el core usa los cursos guardados en SQLite. El worker
reutiliza una sesión HTTP efímera, recorre enlaces internos con límites,
lee también opciones del selector “Ir a…” y conserva títulos de actividad.
Después construye pools ordenados para fases, cronogramas, foros, anuncios,
asignaciones, sesiones y perfil; esos pools se proyectan a `itemCode` y slot.

```json
{
  "username": "123456789",
  "documentType": "CC",
  "courseIds": ["41080"],
  "maxDepth": 2,
  "maxPages": 80
}
```

Los límites son opcionales. La respuesta es `202 Accepted`; el resultado y
los eventos se consultan mediante `/api/jobs/{id}` y
`/api/jobs/{id}/events`.

### `POST /api/course-maps/import-activities`

Importa el resultado seguro del script de DevTools cuando se necesita validar
manualmente un curso. Solo acepta `courseId`, `profileUrl` opcional y arreglos
de enlaces con `path`/`label`; rechaza campos desconocidos y nunca recibe
cookies, contraseñas ni tokens.

```json
{
  "courseId": "41080",
  "profileUrl": "https://zajuna.sena.edu.co/zajuna/user/profile.php",
  "pageLinks": [{ "path": "/zajuna/mod/page/view.php?id=10", "label": "Cronograma General" }],
  "jump": [{ "path": "/zajuna/mod/forum/view.php?id=20", "label": "Foro de Dudas e Inquietudes" }]
}
```

La respuesta es `201 Created` y devuelve el mapa persistido. El importador
conserva el orden de los enlaces para que la asignación de slots coincida con
el orden mostrado por Zajuna.

Para diagnosticar manualmente un curso autenticado existe
`scripts/dev/export-zajuna-course-links.js`. Se puede pegar en la consola de
desarrollador de Chrome y copia únicamente URLs y títulos de enlaces; no lee
cookies, contraseñas ni tokens. Es una herramienta de diagnóstico, no una
dependencia del runtime: la aplicación repite esta extracción desde su worker
local.

### `GET /api/course-maps?limit=50`

Lista los mapas persistidos localmente con sus rutas normalizadas, estadísticas
y advertencias de límites.

### `GET /api/course-maps/{courseId}`

Devuelve el mapa completo de un curso o `404` si todavía no fue descubierto.

## Fichas y checklist

### `GET /api/fichas?limit=100`

Lista las fichas sincronizadas localmente. Cada ficha incluye su identificador
local, código externo, curso y última sincronización.

### `GET /api/checklist/dashboard?fichaId=<id>`

Devuelve la ficha activa, el resumen de progreso, las 15 categorías y sus 62
ítems. Si no se envía `fichaId`, usa la ficha activa persistida en SQLite.

La interfaz presenta cada categoría con su título y cantidad de ítems. También
tolera respuestas con nombres de campos JSON en mayúsculas o minúsculas, para
que el menú de secciones siga siendo legible aunque cambie el serializador.

```json
{
  "activeFichaId": "ficha-…",
  "summary": { "total": 62, "yes": 0, "no": 0, "pending": 62, "percentage": 0 },
  "items": [
    {
      "itemCode": "1.1.1",
      "status": "PENDIENTE",
      "maxEvidences": 1,
      "evidenceCount": 0
    }
  ]
}
```

### `POST /api/fichas/active`

Selecciona la ficha de trabajo y devuelve su dashboard.

```json
{ "fichaId": "ficha-…" }
```

### `PATCH /api/checklist/items/{itemCode}/status`

Actualiza de forma persistente el estado de un ítem. Los valores aceptados
son `SI`, `NO` y `PENDIENTE`.

```json
{ "fichaId": "ficha-…", "status": "SI" }
```

La respuesta devuelve el dashboard recalculado para que la interfaz actualice
el progreso general y el de la categoría sin mantener un estado paralelo.

### `GET /api/checklist/items/{itemCode}?fichaId=<id>`

Devuelve el ítem solicitado con sus slots/evidencias y hasta 50 eventos de
historial ordenados del más reciente al más antiguo. Cada cambio manual guarda
estado anterior, estado nuevo, origen y fecha en SQLite; no incluye cookies ni
credenciales.

### `GET /api/checklist/activities?fichaId=<id>`

Devuelve las actividades `assign` normalizadas desde el mapa local del curso.
Cada actividad incluye título, URL, fase, sección, indicador técnico y si fue
seleccionada por el instructor. La selección es local por ficha y excluye las
vistas de calificación o navegación.

Si la ficha existe pero todavía no tiene un mapa de rutas, la respuesta sigue
siendo `200` y devuelve `mapReady:false`, una lista vacía y `discovery` con la
acción `discover-course-maps`. Esto representa un estado normal de primer uso,
no un error de consulta; la interfaz debe ofrecer el CTA **Buscar rutas**.
Una ficha inexistente conserva la respuesta `404`.

`GET /api/checklist/targets?fichaId=<id>` usa el mismo protocolo de primer uso:
cuando la ficha no tiene mapa responde `200` con `mapReady:false`, objetivos
vacíos y `discovery.action:discover-course-maps`; cuando el mapa está listo
incluye `mapReady:true`.

### `PUT /api/checklist/activities`

Reemplaza de forma atómica las actividades que el instructor declara como
propias. El servidor valida que cada ID pertenezca al mapa actual del curso.
Si aún no existe el mapa, responde `409` con `code:course_map_required` y la
acción `discover-course-maps`, sin crear un trabajo fallido.

```json
{
  "fichaId": "ficha-…",
  "selectedActivityIds": ["3010294", "3010361"]
}
```

Debe ejecutarse antes de capturar las evidencias ligadas a fechas y entregas.

### `GET /api/checklist/targets?fichaId=<id>`

Resuelve las rutas del mapa local de la ficha en objetivos dirigidos por
`itemCode`. La respuesta incluye el resumen de ítems/slots resueltos, la
selección configurada y los selectores o etiquetas que aplicará Chromium. Para
`6.1`, una selección activa hace que el objetivo use el menú principal del
curso, abra la sección correspondiente y capture el bloque
`#module-<activityId>` con sus fechas. `10.1.1` y `10.1.2` usan en cambio la
tabla de calificación de cada actividad seleccionada (`action=grading`: nota,
retroalimentación y fecha de modificación) como una única captura compartida
por ambos ítems; si una actividad no tiene esa ruta, el ítem queda sin resolver
en lugar de repetir la tarjeta de fechas de `6.1`.

Las evidencias con forma de lista o tabla se capturan por lotes de filas
(`rowSelector`, `rowsPerShot` = 2, `rowBatch`): el slot 1 muestra las filas
1–2, el slot 2 las filas 3–4 y así hasta el límite de evidencias del ítem, con
el encabezado de la tabla visible. Aplica a las discusiones y anuncios
publicados por el instructor (`9.1.5`–`9.1.7`, `11.x`, `14.x`, `15.1`), donde
además solo se muestran filas del instructor. Cuando un ítem tiene varias
listas, sus slots se reparten entre ellas y el resto de la división va a las
primeras (8 slots en 3 listas: 3 + 3 + 2), así que ningún slot queda sin
planificar. Un lote que empieza después de la última fila no genera evidencia
(queda "omitido", no como fallo) y la captura retira las evidencias de slots
que ya no corresponden.

El calificador (`5.1`) tiene una columna por ítem de calificación y en un curso
real mide ~28.000 px de ancho. Cada slot muestra las filas 1–2 y una ventana
de columnas completas de hasta 2560 px (`maxCaptureWidth`, `columnBatch`): el
slot 1 las primeras columnas, el slot 2 las siguientes, etc. Una ventana
posterior a la última columna queda "omitida". La metadata de la evidencia
guarda `columnWindows`, el número de ventanas que necesitaba la tabla.

Los cronogramas (`1.x`) con un iframe de Google Sheets (`docs.google.com/
spreadsheets`) agrandan el iframe al tamaño de la hoja antes de capturar
(máximo 2560 × 16.000 px). No se hace clic ni se navega, y si no se puede medir
la hoja se captura igual que antes.

Un foro al que la cuenta no tiene acceso (Moodle redirige con «No dispone de
permiso para ver los debates de este foro») queda marcado `restricted` en el
mapa de rutas y nunca se elige como evidencia. Si un mapa antiguo aún lo
incluye, la captura falla con un mensaje que pide volver a buscar las rutas.
El perfil (`2.x`) usa `profile.php?id=<id>` con el id del usuario autenticado,
leído de `M.cfg.userId` en la página del curso.

### `GET /api/checklist/reviews?fichaId=<id>`

Lista las decisiones locales para las rutas agrupadas. Los estados son
`review`, `confirmed` y `correction`; una revisión puede incluir un enlace o
selector manual sin exponer credenciales ni cookies.

### `PUT /api/checklist/reviews`

Guarda o reemplaza una revisión. La ruta debe pertenecer al mapa actual de la
ficha y un enlace manual debe mantener el mismo origen de Zajuna.

```json
{
  "fichaId": "ficha-…",
  "routeKey": "cronograma_general|https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10|page",
  "status": "confirmed",
  "manualUrl": "",
  "manualSelector": "",
  "note": ""
}
```

Las revisiones se guardan en SQLite y `capture-checklist` las aplica en su
próxima ejecución.

### `POST /api/checklist/capture`

Encola una captura autenticada de los objetivos disponibles para la ficha. Si
no se envían `username` y `documentType`, se toman de la configuración local.
`itemCodes` permite limitar la ejecución a tareas concretas y `maxTargets`
protege ejecuciones de prueba.

```json
{
  "fichaId": "ficha-…",
  "itemCodes": ["1.1.1", "1.2.1"],
  "maxTargets": 20
}
```

Cada PNG queda en la carpeta local de evidencias, se registra con `source`
`capture-checklist`, slot, hash, URL final, actividad, fase, selector y si el
selector coincidió. Los selectores semánticos son estrictos: si no existe el
bloque requerido, el objetivo falla y no se guarda una página genérica como si
fuera la evidencia correcta. Las capturas repetidas actualizan el mismo slot
de checklist en lugar de crear duplicados.

Si falla al menos un objetivo, el job termina `failed` con
`capture_partial_failure`; las evidencias guardadas se conservan. El mensaje
resume los conteos y los ítems afectados, y cada objetivo fallido deja un
evento `evidence_failed` en `/api/jobs/{id}/events` con `itemCode`,
`coveredItemCodes`, `slotNumber` y el motivo. El detalle del trabajo agrupa
esos eventos por ítem y espacio.

Los objetivos de perfil usan la página completa. En foros y anuncios, la
captura de configuración usa el contenedor completo sin la tabla de respuestas;
los objetivos de contenido exigen un post o fila asociado al instructor
autenticado. La captura no continúa con un fallback genérico si esa identidad
no está disponible. Para cronogramas se prioriza el contenido de Google Sheets
embebido y, si el curso usa HTML, se captura el bloque HTML con viewport ancho.
La estructura esperada es la del cronograma de referencia: hoja general y
secciones por fase con nombre de fase, actividades, resultados, evidencias,
área y fechas de inicio/finalización. La vista general no se etiqueta como
exclusiva del área técnica; las relaciones técnicas se determinan por las
actividades seleccionadas por el instructor y por el área/competencia
detectada.

### Captura PNG autenticada

La captura se encola mediante `POST /api/jobs` con `type` igual a
`capture-browser`. Para una ruta de Zajuna puede solicitarse una sesión técnica
efímera:

```json
{
  "type": "capture-browser",
  "input": {
    "url": "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080",
    "authenticated": true,
    "username": "123456789",
    "documentType": "CC"
  }
}
```

El worker valida el mismo origen, carga las cookies solo en el contexto
Chromium de ese job y persiste la evidencia PNG con hash. No guarda cookies ni
contraseñas. Si Zajuna devuelve la pantalla de login, el job termina con
`zajuna_session_expired` para que la UI indique una acción de recuperación.

## Jobs

### `GET /api/jobs?limit=20`

Lista los últimos jobs persistidos, ordenados por actividad. `limit` acepta un
valor entre 1 y 100. Cada job incluye `dismissed` cuando el usuario lo quitó
de «Requiere tu atención», y `fichaId`/`itemCodes` (solo el alcance no
sensible del input) para reintentar el mismo alcance.

### `POST /api/jobs/dismiss`

```json
{ "ids": ["job-…"] }
```

Marca entre 1 y 100 jobs como descartados de «Requiere tu atención». El job y
su historial siguen en Trabajos; se recuerdan como máximo los últimos 500 IDs
descartados.

### `POST /api/jobs`

Crea un job y devuelve `202 Accepted`.

```json
{
  "type": "sync-fichas",
  "input": {
    "username": "123456789",
    "documentType": "CC"
  }
}
```

Respuesta resumida:

```json
{
  "id": "job-…",
  "type": "sync-fichas",
  "status": "queued",
  "progress": 0,
  "stage": "",
  "attempt": 0,
  "maxAttempts": 3
}
```

### `GET /api/jobs/{id}`

Consulta el estado de un job. Los estados posibles son:

`queued`, `running`, `waiting_user`, `retrying`, `completed`, `failed`,
`cancelled`.

### `GET /api/jobs/{id}/events`

Devuelve la secuencia de eventos de progreso y transición del job. Los eventos
no contienen contraseñas, cookies ni tokens.

### `POST /api/jobs/{id}/cancel`

Solicita la cancelación del job. El worker recibe la señal mediante
`context.Context` y debe cerrar sus recursos antes de terminar.

## Automatizaciones locales

### `GET /api/schedules`

Lista las tareas programadas persistidas en SQLite. Un schedule contiene el
tipo de worker, su input JSON, intervalo en segundos, estado habilitado y la
próxima ejecución.

### `POST /api/schedules`

```json
{
  "workerType": "sync-fichas",
  "input": { "username": "123456789", "documentType": "CC" },
  "intervalSeconds": 86400,
  "enabled": true
}
```

El scheduler local entrega el input al runtime de workers y conserva la
cadencia aunque la aplicación haya estado cerrada. No depende de n8n ni de un
servicio remoto.

### `POST /api/schedules/{id}/enabled`

```json
{ "enabled": false }
```

Habilita o pausa un schedule sin eliminar su configuración.

## Preferencias y diagnóstico

### `GET /api/settings` / `PUT /api/settings`

Lee o reemplaza preferencias no sensibles de sesión, captura y avisos. El
servidor valida la forma del documento y lo guarda en `app_settings`; las
contraseñas siguen exclusivamente en el almacén seguro del sistema.

```json
{
  "session": { "autoRenew": true },
  "capture": { "fullPage": true, "reuseSession": true, "motion": true },
  "notifications": { "jobCompleted": true, "needsReview": true },
  "storage": { "retentionKeep": 5, "retentionDays": 30 }
}
```

`capture-checklist` (y `capture-checklist-target`) leen las preferencias al
empezar cada ejecución, así que un cambio aplica al siguiente trabajo sin
reiniciar el core. El job emite el evento `capture_preferences` con los
valores aplicados y los incluye en el `output` del job (`preferences`), también
en fallos parciales y cancelaciones.

| Campo | Efecto |
|---|---|
| `capture.fullPage` | `true` conserva la regla de página completa del perfil del instructor y de los cronogramas. `false` captura solo el bloque detectado. |
| `capture.reuseSession` | `true` comparte sesiones Chromium autenticadas entre los objetivos de una ejecución. `false` abre y cierra una sesión (un login) por objetivo. |
| `session.autoRenew` | `true` reintenta una vez el objetivo con un login nuevo cuando la captura cae en la página de login de Zajuna. `false` deja ese objetivo como fallido. |
| `capture.motion` | Solo afecta a las animaciones de la interfaz. |
| `notifications.*` | Controla qué avisos locales generan los jobs. |
| `storage.retentionKeep` / `retentionDays` | 1–1000 copias y 1–3650 días. Solo los usa la acción **Limpiar antiguas** de Copias de seguridad (`POST /api/backups/cleanup`); no hay limpieza automática. |

`POST /api/checklist/capture` acepta `fullPage`, `reuseSession` y `autoRenew`
opcionales: si la petición los envía, sustituyen a la preferencia guardada
solo en esa ejecución.

Dos capturas de la misma ficha nunca corren a la vez: la segunda espera, y
cancelarla mientras espera la termina de inmediato.

### `GET /api/diagnostics`

Ejecuta comprobaciones locales de core, SQLite, presencia de credencial,
Chromium, carpeta de datos y trabajos fallidos. Devuelve estados `ok`, `warn`
o `error` e incidencias identificadas por job/error code. No realiza la prueba
remota de Zajuna durante el polling y no devuelve mensajes sensibles.

### `GET /api/notifications`

Lista hasta 50 avisos locales ordenados del más reciente al más antiguo. Los
avisos de jobs solo incluyen el tipo de resultado, código seguro y referencia
al job; no se copia el mensaje de error que pudiera contener contenido externo.

### `POST /api/notifications/{id}/read` y `POST /api/notifications/read-all`

Marca un aviso o todos los avisos como leídos. La preferencia de avisos en
`/api/settings` controla si se generan avisos de trabajos completados o de
trabajos que requieren atención.

## Backups

### `POST /api/backups`

Crea una copia ZIP en la carpeta local de backups. Incluye un snapshot
consistente de SQLite, configuración no sensible y artefactos locales de
evidencia/reportes si existen. Las contraseñas y secretos del almacén del
sistema nunca se incluyen.

### `GET /api/backups` y `GET /api/backups/{name}/download`

Lista copias publicadas y permite descargar una copia validada por nombre. La
API nunca expone la ruta absoluta del equipo.

### `DELETE /api/backups/{name}`

Elimina una copia local después de la confirmación explícita en la interfaz.

### `POST /api/backups/cleanup`

Aplica la política conservadora de retención local. Por defecto conserva las
cinco copias más recientes y solo elimina archivos con más de 30 días. Puede
recibir `{ "keep": 7, "olderThanDays": 45 }`; la UI toma ambos valores del
bloque `storage` de `/api/settings` y solicita confirmación antes de ejecutar.

### `POST /api/backups/{name}/restore`

Valida el ZIP (archivos regulares, SHA256 del snapshot y versión de schema) y
lo deja preparado en una carpeta privada para el siguiente arranque. Antes
crea automáticamente una copia de seguridad de protección y responde `202`
con `restartRequired: true`; la base activa nunca se reemplaza mientras el
core está atendiendo peticiones.

En el arranque, `StageRestore` ejecuta `PRAGMA integrity_check` y comprueba
las tablas mínimas (`schema_migrations`, `jobs`, `fichas`, `evidences`)
antes de marcar `.restore-pending`. El swap es atómico. Si `sqlite.Open`
falla después del swap, el core restaura `*.restore-old`, registra
`.restore-applied.json` y reintenta abrir la base anterior. Un ZIP corrupto,
con hash incorrecto o con schema fuera de `1…CurrentSchemaVersion` (hoy
1…14) se rechaza y no toca la DB activa. Un backup con schema anterior se
acepta y se migra hacia adelante al abrirse.

## Evidencias y reportes

### `GET /api/evidences?limit=50&fichaId=<id>`

Lista evidencias locales con formato, origen, fecha, SHA-256 y metadatos de
ficha/ítem. `fichaId` limita la consulta a una ficha y alimenta la galería de
miniaturas seleccionables; la agrupación para reportes se conserva aparte.

Sin `fichaId`, `limit` va de 1 a 100 (50 por defecto). Con `fichaId` la
respuesta incluye todas las evidencias de la ficha (`limit` acepta hasta
10000, que es también el valor por defecto), así la galería no se trunca.

Las vistas de evidencias no exponen la ruta absoluta del archivo. En su lugar
devuelven `fileKey`, un identificador opaco: dos filas con el mismo `fileKey`
comparten archivo. Para leer el contenido se usa `/download`.

### `GET /api/evidences/{id}/download`

Sirve el archivo de evidencia únicamente si permanece dentro de la carpeta
local de evidencias.

El dashboard incluye los enlaces de descarga de las evidencias asociadas a
cada ítem del checklist. La galería visual también permite abrir una vista
previa de cada grupo sin salir de la aplicación.

### `GET /api/evidences/{id}/thumbnail`

Devuelve una miniatura JPEG de 480 px de ancho para las evidencias PNG/JPG,
con la misma validación de ruta que la descarga. Se genera una sola vez y se
guarda en `thumbnails/` dentro de la carpeta de datos (fuera de los respaldos;
el restablecimiento la borra). La clave incluye tamaño y fecha del archivo
original, así que un reemplazo genera una miniatura nueva. Los formatos sin
decodificador estándar (WebP) y cualquier fallo al generarla responden con el
archivo original. La galería de miniaturas usa este endpoint en lugar de la
descarga completa (~2000×2600 px por captura).

### `POST /api/evidences/upload`

Recibe un formulario `multipart/form-data` con `file`, `fichaId` y, de forma
opcional, `itemCode`. Acepta PNG, JPG, PDF y HTML hasta 25 MB. El archivo se
guarda dentro del almacenamiento local, se calcula su SHA-256 y se registra
con origen `manual`. El identificador depende de ficha, ítem, ranura y
contenido: el mismo archivo subido para dos ítems crea dos filas, y volver a
subir el mismo contenido en la misma ranura reutiliza la fila (`200`) sin
dejar archivos huérfanos. Tras cada subida se reconstruyen los grupos de la
ficha, de modo que la galería y el reporte la ven sin pasos manuales.

### `POST /api/evidences/clear`

Reinicia evidencias locales. Cuerpo opcional:

```json
{ "fichaId": "<id>" }
```

Sin `fichaId` elimina todas. Es el borrado explícito de evidencias desde la
API o Configuración. Qué ocurre con los datos al instalar o actualizar la app
está en [`evidence/update-policy.md`](evidence/update-policy.md).

### `DELETE /api/evidences/{id}`

Elimina el registro local de la evidencia. El archivo solo se borra cuando
ninguna otra fila lo referencia: una misma captura puede respaldar varios
ítems y seguir siendo evidencia de los demás. La API rechaza (`403`) archivos
existentes fuera de la carpeta local de evidencias; si el archivo ya no
existe, la fila se retira igualmente. Después se reconstruyen los grupos de la
ficha.

### `GET /api/evidences/groups?fichaId=<id>`

Lista las representaciones agrupadas por contenido: evidencias con el mismo
`sha256` forman un solo grupo aunque pertenezcan a ítems, páginas o grupos
funcionales distintos (sin hash se agrupan por URL, selector y grupo). Cada
grupo conserva los `itemCode` y slots que cubre, y el reporte PDF inserta cada
imagen una sola vez con la lista de ítems que respalda.

La galería local permite buscar por título, código de actividad o descripción,
y filtrar por nivel de confianza (`sugerida`, `confirmada` o `manual`) antes de
abrir la vista previa de un grupo. Cuando hay más de seis grupos, la interfaz
ofrece mostrar la lista completa sin alterar la agrupación utilizada por el
reporte.

La revisión guiada de rutas también admite búsqueda por sección/actividad y
filtros de estado (`por revisar`, `confirmadas` y `para corregir`).

### `POST /api/evidences/groups/rebuild`

Reconstruye los grupos de una ficha. El proceso mantiene los archivos y filas
originales, pero no publica en grupos ni reportes una captura automática que
termine en la página pública/login de Zajuna, que use un selector legado de
perfil/foro o que sea un fixture fuera del catálogo del checklist.

### Revisión de evidencias

Cada evidencia queda `approved`, `pending` o `rejected` (tabla
`evidence_reviews`, schema v14). La verificación automática detecta problemas
técnicos: `file_missing` y `login_page` (rechazada); `too_wide` (> 4000 px),
`too_tall` (> 9000 px), `too_small` (< 200×120 px, solo en secciones del
curso), `mostly_blank` (≥ 99,5 % casi blanco), `empty_section` (sección sin
actividades ni archivos), `generic_selector`, `duplicate_content` y
`outdated_rule` (pendiente). Una decisión manual se respeta mientras el
`sha256` no cambie; al recapturar se vuelve a verificar. `capture-checklist`
ejecuta la verificación al terminar (sin hacer fallar la captura).

- `duplicate_content` compara el `sha256` con evidencias de otros ítems. No
  cuenta como duplicado cuando las dos apuntan a la misma página y selector:
  la URL se compara sin `forceview`, sin fragmento y sin importar el orden de
  los parámetros (11.4 y 15.1 usan el mismo foro de anuncios).
- `outdated_rule`: los ítems cuya prueba depende del contenido (ver
  `checklist.SemanticCheckForItem`) guardan en la metadata la regla con la que
  se capturaron (`semanticCheck`). Una captura hecha con una regla anterior
  queda pendiente hasta volver a capturar el ítem. No aplica a subidas
  manuales.

| Ítems | Regla (`semanticCheck`) | Qué exige la captura |
|---|---|---|
| 9.1.5, 9.1.6, 9.1.7 | `forum-replies` | Debates con réplicas cuyo último mensaje es del instructor. |
| 14.1.1, 14.1.2 | `forum-conclusion` | Debate del instructor cuyo título contiene «conclusión». |
| 9.1.3, 9.1.4 | `forum-dates` | Página del foro que muestra sus fechas (apertura, cierre, vencimiento). |

Si la página correcta cargó (la lista de debates, un foro con su formulario
de búsqueda) y no tiene contenido que cumpla la regla, el slot queda
**ausente** («sin contenido en Zajuna», `capture.ErrContentAbsent`), no
fallido, y se retira la evidencia que dejó una corrida anterior en ese slot.
Una página de error o de permisos, una navegación fallida o una actividad
renombrada siguen siendo fallos y conservan la evidencia anterior. La
ausencia de tabla de calificación en 10.1.x se informa como ausente, pero
tampoco retira evidencia, porque se reconoce solo por el mensaje.

Una verificación automática nunca reemplaza una decisión manual sobre el
mismo archivo (`sha256`), aunque se haya guardado mientras la verificación
corría. «pending» retira la decisión manual de forma explícita.

- `GET /api/evidences/review?fichaId=<id>`: estado guardado (solo verifica las
  evidencias sin revisión). Responde `{fichaId, verifiedAt, summary,
  evidences[], missingItems[]}`; cada evidencia trae `status`, `source`
  (`auto`/`manual`), `note`, `reasons[{code,message}]`, `width`, `height`,
  `sha256` y `sharedWith`.
- `POST /api/evidences/verify` con `{ "fichaId": "…" }`: re-verifica toda la
  ficha y devuelve el mismo payload.
- `PUT /api/evidences/{id}/review` con `{ "status": "approved"|"rejected"|"pending", "note": "" }`:
  guarda la decisión manual (`pending` la borra y vuelve a la verificación
  automática). Devuelve la evidencia actualizada.

`POST /api/evidences/verify`, `PUT /api/evidences/{id}/review` y la revisión
automática al terminar `capture-checklist` sincronizan el checklist: un ítem
«PENDIENTE» cuyas evidencias están todas aprobadas pasa a «SI», y un «SI»
puesto por esta sincronización vuelve a «PENDIENTE» si una evidencia deja de
estar aprobada. Un «SI» o «NO» manual nunca se cambia. Cada cambio queda en el
historial del ítem con `source: "revision-automatica"`. Los ítems sin
evidencia de `missingItems` explican la ausencia de la última captura
(«Sin contenido en Zajuna: …») cuando la hubo.

### `POST /api/reports`

Encola la generación de un reporte mediante `export-report`.

```json
{
  "title": "Reporte mensual",
  "format": "pdf",
  "evidenceLimit": 100
}
```

`format` puede ser `pdf` o `html`. El PDF se renderiza con el Chromium
empaquetado; el HTML se genera directamente en el core.

`evidenceLimit` (100 por defecto) también se aplica al reporte agrupado por
`fichaId`: cuenta entradas del reporte (una por imagen única, aunque cubra
varios ítems). Si se omiten entradas, el reporte lo indica en el resumen y el
resultado del job incluye `omittedGroups`.

### `GET /api/reports?limit=20`

Lista reportes locales terminados. La vista no incluye la ruta del archivo;
se descarga con `/api/reports/{id}/download`.

### `GET /api/reports/{id}/download`

Descarga o abre el archivo local de un reporte validado.

La interfaz muestra los últimos reportes en el panel “Reportes disponibles”,
con su estado y un botón “Abrir PDF” cuando la generación terminó. El usuario
puede generar otro PDF desde ese mismo panel; el proceso se encola y el estado
se actualiza automáticamente.

## Contratos de workers disponibles

Son los workers que `core/cmd/zajuna-core/main.go` registra en el runtime. El
`type` de `POST /api/jobs` debe ser uno de estos IDs; cualquier otro se
rechaza con `400`. `POST /api/schedules` no valida el `workerType` al crear el
schedule: un tipo desconocido falla cuando el scheduler intenta encolarlo.

| Tipo | Lo encola | Qué hace |
|---|---|---|
| `sync-fichas` | `POST /api/setup`, `POST /api/fichas/sync`, schedules | Sesión HTTP de Zajuna y persistencia de fichas/cursos en SQLite. |
| `test-zajuna-connection` | `POST /api/zajuna/test-connection` | Login real y validación de `Mis cursos` sin escribir fichas. |
| `discover-course-maps` | `POST /api/course-maps/discover` | Crawl HTTP autenticado: fases, actividades y rutas clasificadas en SQLite. |
| `capture-evidence` | `POST /api/jobs` | Descarga HTML del origen Zajuna permitido y lo guarda con hash. |
| `capture-browser` | `POST /api/jobs` | Captura PNG con el Chromium empaquetado y sesión Zajuna efímera opcional. |
| `capture-checklist` | `POST /api/checklist/capture` | Resuelve `itemCode`/slots desde el mapa local y captura en paralelo (pool de sesiones Chromium) registrando evidencias por tarea. |
| `capture-checklist-target` | `POST /api/jobs` | Captura un único objetivo del checklist como job propio; `capture-checklist` hace el mismo trabajo en proceso para cada objetivo. |
| `export-report` | `POST /api/reports` | Genera el reporte HTML/PDF local. |

Todos los jobs se crean con `maxAttempts: 3`. Solo se reintentan los fallos
que el worker marca como reintentables (por ejemplo, errores transitorios de
red de Zajuna); errores de entrada, credencial ausente o cancelación terminan
en `failed`/`cancelled`. La concurrencia del runtime es 2 por defecto y se
ajusta entre 1 y 4 con `--jobs-concurrency` o `ZAJUNA_JOBS_CONCURRENCY`; un
valor fuera de ese rango vuelve a 2.

El cliente debe mostrar el progreso que devuelve la API y no ejecutar trabajos
largos dentro de la petición HTTP.

## Fichas

### `GET /api/fichas?limit=50`

Lista las fichas sincronizadas localmente. La respuesta incluye identificador
externo, nombre, curso, estado y fecha de sincronización.

### `POST /api/fichas/sync`

Encola `sync-fichas` para la interfaz y el scheduler. Si no se envía usuario,
usa el usuario guardado en la configuración local.

```json
{
  "username": "123456789",
  "documentType": "CC"
}
```
