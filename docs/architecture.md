# Arquitectura de Zajuna App Desktop

La arquitectura vigente está descrita en detalle en
[`desktop-migration.md`](desktop-migration.md). Este documento resume los
límites entre procesos, datos y seguridad.

## Componentes

### Electron

`desktop/main.cjs` es un launcher silencioso sin `BrowserWindow`. Solicita una
instancia única, inicia el core con `--port=0` y un archivo temporal de
endpoint y un secreto de lanzador en su entorno, espera `/api/health`, pide al
core un enlace de sesión de un solo uso, lo abre con el navegador del
sistema y termina el proceso durante `before-quit`. Un supervisor detecta la
muerte del core, guarda stderr redacted en logs rotativos de 1 MiB e intenta
recuperarlo hasta tres veces. Un segundo lanzamiento reutiliza el endpoint y
abre otra pestaña con un enlace nuevo, sin crear otro backend.

El launcher no confía ciegamente en el archivo temporal: cada endpoint usa un
nonce criptográfico por ejecución y se acepta únicamente `http://127.0.0.1` con
puerto válido, sin credenciales, query ni fragmento, antes de hacer health-check
o abrir el navegador. Esto evita que un archivo temporal manipulado redirija el
launcher a un origen remoto.

### Core Go

El binario en `core/cmd/zajuna-core` sirve los assets React embebidos mediante
`//go:embed` y la API `/api/*` desde el mismo origen. Escucha en loopback,
selecciona un puerto libre y conserva fallback SPA para rutas profundas de
React. No contiene login propio, usuarios locales ni JWT.

El middleware local aplica (contrato y detalle en
[`api-local.md`](api-local.md#protección-del-origen-local)):

1. `Host` loopback y coincidencia de `Origin`/`Sec-Fetch-Site`.
2. Sesión local por proceso en todo `/api/*` salvo `/api/health`: cookie
   `HttpOnly`/`SameSite=Strict` más la cabecera `X-Zajuna-Capability`, ambas
   emitidas solo al canjear un enlace de un solo uso que únicamente el
   lanzador puede pedir (ver `docs/api-local.md`).
3. `Content-Type` esperado y límite de cuerpo.
4. Timeouts de lectura/escritura/idle y `MaxHeaderBytes`.
5. Headers de seguridad y respuestas de error sin secretos.

### Frontend

`frontend/` es React 19 + TypeScript + Vite. `frontend/dist` se sincroniza en
`core/cmd/zajuna-core/web` antes del build Go. React Query gestiona polling,
invalidación y estados de carga/error; React Router maneja las rutas de las
vistas. Las fuentes y assets se empaquetan localmente para funcionamiento
offline de la interfaz.

### Persistencia

SQLite (`zajuna.db`, WAL, `foreign_keys = ON`) vive en la carpeta de datos del
usuario (`%LOCALAPPDATA%\ZajunaApp` en Windows,
`$XDG_DATA_HOME/zajuna-app` en Linux). El schema actual es **v14**
(`currentSchemaVersion` en `core/internal/storage/sqlite/store.go`). Las
migraciones se aplican en una sola transacción, solo hacia adelante, y cada
versión queda registrada en `schema_migrations`:

| Versión | Cambio |
|---|---|
| v1 | `app_settings`, `jobs`, `job_events`, `fichas`, `checklist_items`, `evidences`. |
| v2 | `schedules`. |
| v3 | `evidences` admite evidencias sin ficha (`ficha_id` opcional). |
| v4 | `reports`. |
| v5 | `course_capture_maps` (mapas de rutas por curso). |
| v6 | Catálogo del checklist (`checklist_catalog_categories`, `checklist_catalog_items`) y estado `PENDIENTE`. |
| v7 | Deduplica capturas del checklist: una fila por slot. |
| v8 | `evidence_groups` y `evidence_group_members`. |
| v9 | `ficha_activity_selections` (actividades elegidas por ficha). |
| v10 | `checklist_route_reviews` (revisión/corrección de rutas). |
| v11 | `checklist_item_events` (historial por ítem). |
| v12 | `notifications`. |
| v13 | Una evidencia vigente por `(ficha, ítem, slot, origen)`: deduplica y cambia la clave única. |
| v14 | `evidence_reviews` (revisión automática y manual de cada evidencia). |

Tras migrar, el core recoge los archivos de evidencia que ya no referencia
ninguna fila. Un core nunca abre ni restaura una base con schema mayor que el
suyo.

La contraseña se escribe exclusivamente en Credential Manager (Windows) o
Secret Service (Linux). Cookies y tokens permanecen en memoria; URLs y eventos
pasan por redacción antes de persistirse.

Los backups ZIP incluyen snapshot SQLite, hashes SHA256 y `schemaVersion`. El
restore valida `PRAGMA integrity_check` y el schema en staging; si `Open`
falla tras el swap, se restaura la base anterior.

Qué pasa con estos datos al instalar, actualizar o restablecer la app está en
[`evidence/update-policy.md`](evidence/update-policy.md).

### Workers y Chromium

Los workers registrados son `sync-fichas`, `test-zajuna-connection`,
`discover-course-maps`, `capture-evidence`, `capture-browser`,
`capture-checklist`, `capture-checklist-target` y `export-report`; la tabla con
qué ruta encola cada uno está en
[`api-local.md`](api-local.md#contratos-de-workers-disponibles). El runtime de
jobs persiste estado, eventos, reintentos y progreso. Las transiciones son
CAS en SQLite: un worker por job y, al arrancar, los `running` huérfanos
pasan a `retrying` (o `failed` si no quedan intentos). Playwright Go usa el
runtime instalado bajo `core/bin/playwright`; el empaquetado copia esa carpeta
junto al core en `resources/core/playwright`.

Las capturas de producción permiten únicamente el origen Zajuna configurado,
bloquean IPs privadas/loopback, comprueban cookies de sesión contra ese
origen, validan la URL final antes del screenshot y redaccionan metadata.
CAPTCHA/MFA abortan la captura. Los helpers de pruebas pueden usar servidores
locales controlados.

## Flujo de ejecución

```text
Electron
  → inicia core Go y espera endpoint
  → pide un enlace de sesión y abre el navegador predeterminado en loopback
  → React llama API same-origin con cookie + cabecera de sesión
  → API crea job persistente
  → worker usa SQLite, keyring y Chromium
  → eventos/polling actualizan React
  → resultado local: evidencia, reporte, backup o diagnóstico
```

## Build y distribución

```text
frontend build → sync web → Go targets → Chromium del runner nativo
→ staging target → electron-builder → smoke → manifest/SBOM
```

El staging se valida contra el host: no se permite generar un instalador Linux
desde Windows con Chromium incorrecto. Los cores cross-compiled se generan para
Windows/Linux x64 y ARM64, pero el instalador debe probarse nativamente.

## Riesgos aún abiertos

- Firma Authenticode de Windows (MDL-29).
- Smoke nativo de NSIS/AppImage y ciclo instalar/actualizar/desinstalar (MDL-29).
- Cobertura de lector de pantalla limitada a NVDA en Windows (MDL-32 cerró
  teclado, zoom, reflow y NVDA).
- Gate de release con matriz y acta (MDL-34).
