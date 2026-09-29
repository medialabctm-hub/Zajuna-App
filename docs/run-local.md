# Cómo corre Zajuna Sync en local

Zajuna Sync **ya no es un sitio web remoto**. Es una aplicación de escritorio
que enciende un servidor solo en tu PC (`127.0.0.1`) y abre el navegador
predeterminado contra esa dirección. No hay n8n, Docker, MySQL ni túnel.

## Piezas

```text
Electron (launcher, sin ventana propia)
  └─ core Go  →  http://127.0.0.1:<puerto aleatorio>
       ├─ interfaz React embebida (mismas URLs /resumen, /fichas, …)
       ├─ API /api/...
       ├─ SQLite + evidencias en la carpeta de datos del usuario
       └─ Chromium empaquetado (solo para capturas PNG/PDF)
```

La única red externa es HTTPS hacia Zajuna (login, fichas, capturas). La
contraseña vive en el almacén del sistema (Credential Manager en Windows),
no en un `.env`.

## Cómo iniciarla en desarrollo (esta estación)

En la raíz del repo, con Node, npm y Go instalados:

```powershell
npm install
npm run frontend:install
npm run desktop:dev
```

`desktop:dev` construye el frontend, compila `core/bin/zajuna-core.exe`,
lanza Electron y abre el navegador cuando `/api/health` responde.

Si Electron no descarga el binario (`path.txt` ausente), el core Go basta:

```powershell
npm run build
.\core\bin\zajuna-core.exe
```

El core imprime `Zajuna Sync local disponible en http://127.0.0.1:<puerto>` y
abre el navegador. Electron solo supervisa ese proceso; no es obligatorio
para desarrollar la interfaz.

Cerrar la pestaña del navegador **no** apaga el core. Para salir, cierra
Electron (icono de la bandeja / proceso) o termina el terminal.

Un segundo `desktop:start` no duplica el backend: reabre la URL existente.

## Cómo la usará un instructor (instalador)

La guía paso a paso (requisitos, SmartScreen, asistente, Setup y fallos) está
en [`guia-instalacion.md`](guia-instalacion.md). Resumen:

1. Instala `Zajuna.Sync.Setup-…exe` (Windows) o el AppImage (Linux).
2. El acceso directo inicia el mismo launcher: core local + navegador.
3. En el primer arranque aparece **Setup**: documento y contraseña de Zajuna.
4. A partir de ahí, Resumen, fichas, checklist, evidencias y reportes son
   locales. Los backups ZIP también son locales.

macOS no es una plataforma soportada.

## Pruebas rápidas

```powershell
go -C core test ./...
npm run lint --prefix frontend
npm run test --prefix frontend
npm run test:downloads
```

Smoke de interfaz con Chromium empaquetado (incluye `/evidencias` y la ruta
de «Borrar evidencias» en Configuración → Almacenamiento; el confirm se
cancela, no borra datos):

```powershell
$env:ZAJUNA_RUN_BROWSER_SMOKE='1'
$env:ZAJUNA_PLAYWRIGHT_DIR=(Join-Path (Get-Location) 'core/bin/playwright')
go -C core test ./cmd/zajuna-core -run 'TestDashboardBrowserSmoke|TestEvidenciasBrowserSmoke' -v
```

En Linux/macOS de desarrollo: `ZAJUNA_RUN_BROWSER_SMOKE=1` y la misma
invocación `go test`. También: `npm run test:browser:core`.

El E2E contra Zajuna real usa variables de entorno (`ZAJUNA_E2E=1`); nunca
se guardan credenciales en git.

## Si algo no abre

- Revisa que exista `core/bin/zajuna-core.exe` (salida de `npm run build`).
- El core solo escucha en loopback; no publiques el puerto.
- Logs del core (desarrollo empaquetado): carpeta de datos de Electron,
  `logs/zajuna-core.log`.
