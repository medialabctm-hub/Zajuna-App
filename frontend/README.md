# Zajuna Sync — Frontend

Interfaz de Zajuna Sync en React 19 + TypeScript + Vite. El build se sincroniza
con `core/cmd/zajuna-core/web/` y queda embebido en el binario Go para
producción.

## Stack

- **Vite** — build y dev server.
- **React 19 + TypeScript**.
- **React Router** — rutas reales por pantalla (`/resumen`, `/fichas`, `/checklist`, `/actividades`, `/evidencias`, `/trabajos`, `/reportes`, `/configuracion`, `/diagnostico`).
- **@tanstack/react-query** — fetching y polling (cada 5 s) contra la API del core Go, reemplaza el `refresh()` manual del vanilla.
- CSS global en `src/styles/global.css` — es el sistema de diseño exacto portado del vanilla (tokens, componentes, animaciones, responsive). No se reinventó nada visualmente en la migración.

## Desarrollo

Necesitas el core Go corriendo por separado (sirve la API en `/api/*`):

```bash
cd ../core
go run ./cmd/zajuna-core -port 62301 -no-browser
```

Luego, en esta carpeta:

```bash
npm install
npm run dev
```

Para una instalación limpia reproducible desde la raíz usa
`npm run frontend:install`; el empaquetado ejecuta este paso automáticamente.

Vite corre en un puerto propio (5173 o el siguiente libre) y hace proxy de
`/api/*` hacia `http://127.0.0.1:62301` (configurable con la variable de
entorno `ZAJUNA_CORE_PORT` si el core usa otro puerto). Así el mismo código
funciona igual en desarrollo y en producción, donde el build se sirve desde
el mismo origen que la API (sin proxy).

## Estructura

```text
src/
  api/client.ts       Cliente tipado de la API (incluye normalizeKeys: el
                       core a veces responde con claves PascalCase)
  types/index.ts       Tipos de los datos que expone la API
  hooks/api.ts          Hooks de React Query (queries con polling + mutaciones)
  hooks/useToast.tsx    Notificaciones (equivalente al toast() del vanilla)
  lib/format.ts         Helpers de formato/copy humano (fechas, estados, confianza...)
  lib/nav.ts             Configuración de navegación (sidebar + breadcrumb)
  components/            Icon, Sidebar, Topbar, AppShell
  pages/                  Una página por pantalla
```

## Alcance actual y siguiente deuda

El frontend consume detalle de tarea e historial (`/checklist/:itemCode`),
timeline de jobs (`/trabajos/:id`), programación local, preferencias de
captura/avisos, diagnóstico real, centro histórico de notificaciones, gestión
de backups (incluida retención configurable) y galería plana de miniaturas.
## Pruebas unitarias (Vitest)

En `frontend/`:

```bash
npm ci
npm run test
```

Cubre helpers puros (`format`, `friendlyError`, `nav`, URLs de descarga de
evidencias/reportes). El entorno es `node` (CI-friendly); no hace falta
navegador ni `@testing-library` para esta capa.

## Smoke de navegador (core)

`npm run test:visual:core` verifica la composición de Resumen en desktop,
tablet y móvil con baselines SHA-256. La deuda restante está documentada en
`../docs/ui-qa-audit.md` y `../docs/accessibility-audit.md`.

## Empaquetado con el core

El pipeline de raíz ejecuta:

1. `npm run frontend:build`;
2. `npm run frontend:sync`, que copia `frontend/dist` a
   `core/cmd/zajuna-core/web`;
3. `go build`, que embebe esa carpeta mediante `//go:embed`.

`npm run desktop:dev` y `npm run package` pasan por el mismo pipeline, por
lo que Electron y el instalador muestran el build React same-origin con la
API. El servidor Go devuelve `index.html` en las rutas sin extensión para que
React Router soporte navegación directa y refresco.
