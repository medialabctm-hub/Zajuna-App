# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Las reglas de colaboración multiagente (Worktrees de Orca, cambio mínimo, flujo de verificación, informe final) están en [`AGENTS.md`](AGENTS.md) y aplican también aquí. Este archivo solo cubre comandos y arquitectura. Documentación en español; responder y comentar en español.

## Qué es

Aplicación de escritorio local para sincronizar Zajuna (Moodle), revisar el checklist, capturar evidencias y generar reportes. Electron es un **launcher silencioso** (sin `BrowserWindow`): arranca el core Go en `127.0.0.1:<puerto dinámico>` y abre la URL en el navegador del sistema. El core sirve la API `/api/*` y el frontend React embebido con `go:embed`, mismo origen. No hay n8n, Docker, MySQL ni servidor remoto.

## Comandos

Todo desde la raíz, salvo que se indique. Requiere Node 22 y Go (versión en `core/go.mod`).

```powershell
npm install; npm run frontend:install   # dependencias (raíz + frontend)
npm run browser:install                 # runtime de Chromium/Playwright en core/bin/playwright
npm run desktop:dev                     # build completo + Electron
npm run build                           # frontend build + sync + compila core/bin/zajuna-core(.exe)
.\core\bin\zajuna-core.exe              # correr solo el core (sin Electron)

# Iterar solo la UI: core en un terminal, Vite en otro (Vite hace proxy de /api a
# 127.0.0.1:62301, o al puerto de ZAJUNA_CORE_PORT)
go -C core run ./cmd/zajuna-core -port 62301 -no-browser
npm run dev --prefix frontend

# Lint / tests (lo que corre CI)
npm run lint --prefix frontend          # oxlint
npm run test --prefix frontend          # vitest
npm run build --prefix frontend         # tsc -b && vite build
go -C core vet ./...
go -C core test ./...
npm run test:downloads; npm run test:smoke:native; npm run test:desktop   # tests de scripts/ y desktop/

# Un solo test
go -C core test ./internal/workers -run TestNombre -v
npm run test --prefix frontend -- src/lib/format.test.ts
```

Smokes con navegador real (necesitan `npm run browser:install`; usan `powershell`, ver `package.json`): `npm run test:browser:core`, `test:e2e:core`, `test:visual:core`. Están detrás de `ZAJUNA_RUN_BROWSER_SMOKE=1` y `ZAJUNA_PLAYWRIGHT_DIR`. `TestDashboardBrowserSmoke` compara hashes exactos de capturas y por eso no corre en CI.

`npm run test:e2e:zajuna` pega contra Zajuna real y exige `ZAJUNA_E2E=1` + `ZAJUNA_TEST_DOCUMENT_TYPE/USERNAME/PASSWORD` **solo en variables de entorno de la sesión** (nunca en archivos ni commits). Variables útiles para pruebas manuales: `ZAJUNA_SKIP_EXTERNAL_OPEN=1` (no abre pestaña), `ZAJUNA_JOBS_CONCURRENCY`.

Empaquetado: `npm run package:windows` / `package:linux`. Debe ejecutarse en el SO de destino (incluye el Chromium del runner; `scripts/package.cjs` rechaza cruzar plataformas). macOS no se soporta.

## `core/cmd/zajuna-core/web/` está versionado pero es generado

Es la salida de `npm run frontend:sync` (copia `frontend/dist`). No se edita a mano. Al cambiar `frontend/`, regenerar con `npm run build` (o `frontend:build` + `frontend:sync`) y commitear `web/` en un commit aparte, como hace el historial (`build: web/ regenerado para X`). `go build`/`go test` del `cmd` fallan si `web/` no existe.

## Arquitectura

Vista general en [`docs/architecture.md`](docs/architecture.md); contrato HTTP y workers en [`docs/api-local.md`](docs/api-local.md); sistema visual en [`docs/design-system.md`](docs/design-system.md). Lo que exige leer varios archivos:

- **Seguridad del origen local** (`core/cmd/zajuna-core/api_security.go`, `core/internal/security`, `desktop/main.cjs`): todo `/api/*` salvo `/api/health` exige cookie `HttpOnly`/`SameSite=Strict` **más** cabecera `X-Zajuna-Capability`, emitidas solo al canjear un enlace de un solo uso que únicamente el lanzador puede pedir (con `ZAJUNA_LAUNCHER_SECRET`). El launcher valida el archivo de endpoint con nonce y acepta solo `http://127.0.0.1:<puerto>`. Un cambio de API o del launcher tiene que respetar este contrato.
- **Instancia única por carpeta de datos**: `core/internal/storage/datalock` impide dos cores sobre la misma carpeta; un segundo lanzamiento de Electron reutiliza el endpoint existente y abre otra pestaña con enlace nuevo.
- **Jobs y workers** (`core/internal/jobs`, `core/internal/workers`): toda operación larga (sync de fichas, capturas, exportar reporte, descubrir mapas de curso) es un job persistente en SQLite con estado, eventos, reintentos y transiciones CAS. La API `*_api.go` encola; un worker por tipo de job lo ejecuta (`sync-fichas`, `capture-checklist`, `capture-browser`, `export-report`, …). Al arrancar, los jobs `running` huérfanos pasan a `retrying`/`failed`. El frontend se entera por polling/eventos.
- **Capturas** (`core/internal/capture`, `workers/capture_*`): Playwright Go con el Chromium de `core/bin/playwright`. Solo se permite el origen Zajuna configurado, se bloquean IPs privadas/loopback, se valida la URL final antes del screenshot y se redaccionan URLs/metadata. CAPTCHA/MFA abortan la captura. El login de Zajuna es de dos pasos (ver `docs/mdl-33-2026-08-26.md`). `coursemaps` guarda las rutas por curso y `checklist` el catálogo/slots/targets de cada ítem.
- **Persistencia** (`core/internal/storage/sqlite`): SQLite en WAL, migraciones solo hacia adelante en una transacción; `currentSchemaVersion` en `store.go` (tabla de versiones en `docs/architecture.md`). Un core nunca abre ni restaura una base con schema mayor que el suyo. Al añadir una migración, subir esa constante y actualizar la tabla del doc. Backups ZIP (`storage/backup`) llevan snapshot SQLite + SHA256 + `schemaVersion`.
- **Secretos**: la contraseña de Zajuna vive solo en Credential Manager (Windows) / Secret Service (Linux) vía `core/internal/secrets`; cookies y tokens solo en memoria; URLs y eventos se redaccionan antes de persistir.
- **Frontend** (`frontend/src`): React Router con rutas en español (`/resumen`, `/fichas`, `/checklist`, `/evidencias`, `/trabajos`, `/reportes`, …) y fallback SPA en el core; React Query en `hooks/api.ts` sobre `api/client.ts`; tipos de la API en `types/index.ts`; estilos en `styles/global.css` (tokens del sistema visual). Fuentes y assets locales para funcionar offline.
- **Release**: `scripts/` genera cores Go para Windows/Linux x64 y ARM64 (`build-core-targets.cjs`), empaqueta con electron-builder y produce `dist/release-manifest.json` y SBOM. Los instaladores nativos en CI solo por `workflow_dispatch`.
