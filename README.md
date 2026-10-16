# Zajuna Sync

**Zajuna Sync** es una aplicación para instructores del SENA que trabajan en
Zajuna. Trae tus fichas desde Zajuna, te muestra el checklist de cada una,
toma por ti las capturas de pantalla que sirven como evidencia y genera el
reporte final en PDF.

Todo ocurre en tu propio computador: la aplicación se abre en tu navegador
(Chrome, Edge, Firefox…), tus evidencias y reportes se quedan en el equipo y
la contraseña se guarda en el almacén seguro del sistema operativo. Solo
necesitas internet para entrar a Zajuna.

Funciona en **Windows 10/11 (64 bits)** y en **Linux**. macOS no está
soportado.

## Instalar en 3 pasos

1. **Descarga el instalador.** Entra a la página de
   [versiones publicadas (Releases)](https://github.com/ElBrayayinn/Zajuna-App/releases/latest)
   y, en la sección *Assets*, descarga:
   - Windows: el archivo que termina en **`.exe`** (por ejemplo
     `Zajuna.Sync.Setup-<versión>.exe`).
   - Linux: el archivo que termina en **`.AppImage`**.
2. **Ejecuta el instalador.** Haz doble clic en el archivo descargado. Si
   Windows muestra el aviso azul *«Windows protegió su PC»*, pulsa
   **Más información** y luego **Ejecutar de todas formas**. Aparece porque
   el instalador todavía no tiene firma digital; no hace falta desactivar el
   antivirus. La instalación termina sola en uno o dos minutos.
3. **Abre la aplicación y conecta tu cuenta.** Abre **Zajuna Sync** desde el
   menú Inicio o el escritorio. Se abrirá una pestaña en tu navegador con la
   pantalla *Conecta tu cuenta de Zajuna*: elige tu tipo de documento,
   escribe tu número y tu contraseña de Zajuna y pulsa **Continuar**. La
   primera vez la aplicación busca tus fichas y prepara las rutas de tus
   cursos automáticamente.

Guía completa, con cada pantalla, el primer uso, solución de problemas y cómo
desinstalar: **[docs/guia-instalacion.md](docs/guia-instalacion.md)**.

## Qué puedes hacer con la aplicación

| Pantalla | Para qué sirve |
|---|---|
| **Resumen** | Ver el avance de la ficha activa y el siguiente paso recomendado. |
| **Fichas** | Traer tus fichas desde Zajuna y elegir con cuál trabajar. |
| **Checklist** | Revisar cada punto del checklist y su estado. |
| **Guías** | Lo que solo tú puedes hacer dentro de Zajuna, explicado paso a paso. |
| **Actividades** | Marcar las actividades técnicas que tú calificas. |
| **Evidencias** | Ver, previsualizar y agregar las capturas y archivos. |
| **Revisión** | Aprobar las evidencias correctas y volver a capturar las que fallaron. |
| **Trabajos** | Ver qué está haciendo la aplicación y reintentar lo que falló. |
| **Reportes** | Generar y descargar el reporte final en PDF. |
| **Configuración** / **Diagnóstico** | Cambiar la cuenta, probar la conexión, copias de seguridad y estado de la aplicación. |

## ¿Qué hay en cada carpeta?

| Carpeta | Qué contiene, en palabras sencillas |
|---|---|
| [`docs/`](docs/) | Documentación: la [guía de instalación](docs/guia-instalacion.md), notas de cada versión, decisiones técnicas y actas de pruebas. |
| [`frontend/`](frontend/) | Las pantallas que ves en el navegador (botones, menús, colores). |
| [`core/`](core/) | El "motor" de la aplicación: se conecta a Zajuna, toma las capturas, guarda los datos en el equipo y arma los reportes. |
| [`desktop/`](desktop/) | El programa que se instala en Windows/Linux: enciende el motor y abre la aplicación en tu navegador. |
| [`scripts/`](scripts/) | Herramientas automáticas para construir el instalador y probar que todo funcione. |
| [`build/`](build/) | Iconos de la aplicación y ajustes del instalador de Windows. |

Archivos sueltos en la raíz: `package.json` (lista de dependencias y comandos
del proyecto), `README.md` (este documento), `AGENTS.md` y `CLAUDE.md`
(reglas de trabajo para los asistentes de programación).

---

## Para desarrolladores

Todo lo que sigue es técnico: cómo está construida la aplicación, cómo
compilarla, probarla y empaquetarla.

### Documentación técnica

- Migración completa, decisiones técnicas, tareas abiertas y matriz de
  pruebas: [`docs/desktop-migration.md`](docs/desktop-migration.md).
- Cierre de Linear M0/M1 (2026-08-20):
  [`docs/hardening-2026-08-20.md`](docs/hardening-2026-08-20.md).
- Validación autenticada contra Zajuna real, con el contrato de login de dos
  pasos y el registro de selectores por curso:
  [`docs/mdl-33-2026-08-26.md`](docs/mdl-33-2026-08-26.md).
- Gate de integración más reciente (MDL-34), con el ciclo completo de
  instalar/usar/desinstalar Windows:
  [`docs/release-gate-2026-09-11.md`](docs/release-gate-2026-09-11.md).
- Notas de la versión actual: [`docs/release-0.1.6.md`](docs/release-0.1.6.md).
- Arquitectura: [`docs/architecture.md`](docs/architecture.md); API local y
  workers: [`docs/api-local.md`](docs/api-local.md); sistema visual:
  [`docs/design-system.md`](docs/design-system.md).
- Cómo arrancar en local: [`docs/run-local.md`](docs/run-local.md).

### Arquitectura actual

La aplicación instalada funciona como un launcher silencioso: inicia el core
Go y abre el localhost en el navegador del sistema. Electron no crea ninguna
ventana ni WebView; solo mantiene el proceso local y evita instancias
duplicadas.

```text
Electron
  └─ core Go en 127.0.0.1:<puerto dinámico>
       ├─ React 19 + Vite embebido con go:embed
       ├─ API local same-origin
       ├─ SQLite + migraciones
       ├─ workers, scheduler y eventos
       ├─ evidencias, reportes y backups
       └─ Chromium/Playwright empaquetado
```

No requiere n8n, Docker, MySQL, ngrok, JWT ni un servidor remoto. La única
dependencia externa es la conexión HTTPS de Zajuna. La contraseña se guarda en
el almacén seguro del sistema operativo.

Carpetas, en detalle:

- `frontend/`: interfaz React y sistema visual.
- `core/`: API, SQLite, workers, capturas y reportes Go.
- `desktop/`: ciclo de vida Electron y supervisor del core.
- `scripts/`: sincronización, build, staging, metadata y smoke.
- `build/`: iconos y `installer.nsh` (macros NSIS que borran los datos locales
  al instalar/desinstalar).
- `docs/`: arquitectura, API, auditorías, plan de migración y registro de hardening.
- `core/cmd/zajuna-core/web/`: salida generada de `npm run frontend:sync`; no
  se edita a mano.

### Desarrollo

```powershell
npm install
npm run frontend:install
npm run browser:install
npm run desktop:dev
```

Modo de navegador local:

```powershell
npm run desktop:start
```

El launcher espera `/api/health` antes de abrir el navegador. El core continúa
escuchando únicamente en loopback; la sesión local por proceso solo se obtiene
con el enlace de un solo uso que pide el launcher (ver `docs/api-local.md`).
Cerrar la pestaña no detiene el core; volver a ejecutar el acceso directo solo
abre de nuevo la URL de la instancia existente.

Para smoke tests sin abrir una pestaña real: `$env:ZAJUNA_SKIP_EXTERNAL_OPEN =
'1'`.

Para trabajar solo en React:

```powershell
cd frontend
npm run dev
```

### Pruebas

```powershell
npm run build --prefix frontend
npm run lint --prefix frontend
npm run test --prefix frontend
go -C core test ./...
go -C core vet ./...
npm run test:downloads
npm run test:smoke:native
npm run test:desktop
npm audit --omit=dev --audit-level=high
npm run test:browser:core
```

CI (`.github/workflows/ci.yml`) ejecuta lint, Vitest y build del frontend;
`go vet`, `go test` y `govulncheck` del core; los tests de `scripts/` y
`desktop/`; y un job `browser-smoke` en Linux que instala Chromium con
`scripts/install-playwright.cjs` y corre los smokes contra fixtures locales.
El smoke visual del dashboard queda fuera de CI porque compara hashes exactos
de capturas que dependen del renderizado de fuentes del sistema.

El smoke visual comprueba Resumen en desktop, tablet y móvil; el smoke
empaquetado inicia el ejecutable, verifica `/api/health` en loopback, el
frontend embebido, los deep links SPA, assets y respuestas 404 de API/static.

#### Pruebas contra Zajuna real

El E2E autenticado necesita una cuenta de prueba. Las credenciales van solo en
variables de entorno de la sesión: nunca en un archivo ni en un commit.

```powershell
$env:ZAJUNA_E2E = '1'
$env:ZAJUNA_TEST_DOCUMENT_TYPE = 'CC'
$env:ZAJUNA_TEST_USERNAME = '<documento>'
$env:ZAJUNA_TEST_PASSWORD = '<contraseña>'
npm run test:e2e:zajuna
```

Añadiendo `ZAJUNA_MAPS_E2E=1` se descubren las rutas del curso, y con
`ZAJUNA_CAPTURE_E2E=1` más `ZAJUNA_PLAYWRIGHT_DIR` se ejecutan las capturas
reales. Si además se define `ZAJUNA_SELECTOR_REPORT`, la corrida escribe en esa
ruta el registro de selectores por curso (`docs/evidence/mdl-33-selectors.json`),
sin credenciales, nombres, host ni rutas locales. El procedimiento completo está
en [`docs/mdl-33-2026-08-26.md`](docs/mdl-33-2026-08-26.md).

### Empaquetado

El empaquetado debe ejecutarse en el sistema objetivo para incluir el Chromium
correcto:

```powershell
npm run package:windows
npm run package:linux
```

macOS no es una plataforma soportada. `npm run build:platforms` genera los
cores Go para Windows/Linux x64 y ARM64. `scripts/package.cjs` exige que el
runner coincida con la plataforma, hace el staging del core + Playwright, y
genera `dist/release-manifest.json` y `dist/sbom.cyclonedx.json`. El
instalador Windows (`dist/Zajuna.Sync.Setup-<versión>.exe`, NSIS de un clic,
instalación por usuario) actualmente no tiene firma digital
([`docs/signing.md`](docs/signing.md)).

En GitHub, `.github/workflows/packaging.yml` empaqueta Windows (NSIS) y Linux
(AppImage) al subir un tag `v*` o a mano, y
`.github/workflows/native-installers.yml` solo a mano (`workflow_dispatch`).
Ambos dejan el instalador, el manifiesto y el SBOM como *artifacts* del run de
Actions; la publicación en GitHub Releases
(`medialabctm-hub/Zajuna-App`, de donde lee el auto-update) se hace a mano
adjuntando esos archivos.

El smoke específico del modo externo puede ejecutarse contra un paquete o,
durante desarrollo, contra Electron directamente:

```powershell
npm run test:smoke:external-browser
$env:ZAJUNA_EXTERNAL_DEV = '1'
npm run test:smoke:external-browser
```
