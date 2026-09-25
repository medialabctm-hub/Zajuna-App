# MDL-161 — Permisos, XDG, FUSE y sandbox (AppImage)

Issue: [MDL-161](https://linear.app/medialab-sena/issue/MDL-161).
Evidencia previa: [MDL-120](../mdl-120-linux-appimage-2026-09-11.md),
[MDL-123](../mdl-123-linux-kali-2026-09-13.md),
[`linux-kali-fuse2.md`](linux-kali-fuse2.md).
Auditoría pendiente: [MDL-176](https://linear.app/medialab-sena/issue/MDL-176).

## Resumen honesto vs criterios de aceptación

| Criterio MDL-161 | Estado en v0.1.1 / código actual | Notas |
|---|---|---|
| Arranca como usuario estándar (sin `sudo`) | **Cumple** (Kali contenedor + WSL2; ver matriz) | No hace falta root para el AppImage. |
| No requiere desactivar el sandbox | **No cumple tal cual** | El wrapper Linux fuerza `--no-sandbox` en el argv real (`scripts/linux-no-sandbox-wrapper.cjs`). Ver §Sandbox. |
| Datos en rutas XDG documentadas | **Cumple** (core Go) | `~/.local/share/zajuna-app` / `$XDG_DATA_HOME/zajuna-app`. |
| Temporales / perfiles limpios tras fallo | **Parcial / no auditado de punta a punta** | Hay supervisor de proceso y stop del árbol; limpieza exhaustiva de perfiles Chromium de captura queda para MDL-176. |
| Mensajes sin recomendar `sudo` / `--no-sandbox` | **Parcial** | La guía documenta FUSE y extract-and-run; el flag `--no-sandbox` va embebido en el wrapper, no se pide al usuario. |

Este documento registra la **realidad operativa**. No convierte un gap en “aprobado”.

## Rutas XDG y Electron

### Core Go (persistencia de producto)

Definido en `core/cmd/zajuna-core/main.go` → `dataDirectory()`:

| Uso | Ruta |
|---|---|
| Datos / SQLite / evidencias / config de producto | `$XDG_DATA_HOME/zajuna-app` o, si no hay, `~/.local/share/zajuna-app` |
| Windows (referencia) | `%LOCALAPPDATA%\ZajunaApp` |

Archivos típicos: `zajuna.db`, `config.json`, `evidences/`, reportes y respaldos.
Desde la 0.1.3, abrir un AppImage de otra versión borra el contenido de esta
carpeta en el primer arranque (`backup.EnforceVersion`); ver
[`../evidence/update-policy.md`](../evidence/update-policy.md).

### Launcher Electron (logs del supervisor)

`desktop/main.cjs` escribe el log del core en `app.getPath('userData')/logs/…`.

En Linux, Electron resuelve `userData` bajo **configuración**
(`$XDG_CONFIG_HOME/<nombre-app>` o `~/.config/…`), no bajo `XDG_DATA_HOME`.
Datos de producto (Go) y logs del launcher **no comparten** la misma raíz XDG.

### Temporales

- Endpoint del core: JSON en `os.tmpdir()/zajuna-app-<pid>-<nonce>.json`, eliminado al parar.
- Runtime AppImage: montaje `/tmp/.mount_*` cuando hay FUSE.
- Sin FUSE: `--appimage-extract` / `--appimage-extract-and-run`.

## FUSE / AppImage

| Situación | Comportamiento esperado |
|---|---|
| Distro con `libfuse.so.2` | AppImage monta y ejecuta. |
| Kali / Debian rolling **sin** `libfuse2` | `dlopen(): error loading libfuse.so.2` → extract-and-run documentado. |
| Contenedor sin `/dev/fuse` | Mismo síntoma; extract-and-run como diagnóstico. |

```bash
chmod +x Zajuna.App-*.AppImage
./Zajuna.App-*.AppImage --appimage-extract-and-run
```

## Sandbox — realidad vs aceptación

### Qué pide MDL-161

Arrancar **sin** pedir al usuario que desactive el sandbox ni use `sudo`.

### Qué hace el producto hoy

1. `app.commandLine.appendSwitch('no-sandbox')` en JS **llega tarde** para el FATAL nativo de root (MDL-123).
2. `scripts/linux-no-sandbox-wrapper.cjs` (afterPack) deja un wrapper:

   ```sh
   exec "$DIR/<exec>.bin" --no-sandbox --ozone-platform=headless "$@"
   ```

3. AppImage no instala `chrome-sandbox` SUID. La app **no** crea `BrowserWindow`
   (React en el navegador del sistema), pero el flag sí debilita el aislamiento
   del proceso Electron frente a un modelo con sandbox SUID completo.
4. `--ozone-platform=headless` evita “Missing X server” sin sesión gráfica.

### Gap explícito

“No se desactiva el sandbox” **no es cierto** mientras el wrapper fuerce
`--no-sandbox`. Queda abierto para **MDL-176**. No se documenta al usuario
“ejecuta con `--no-sandbox`”; el flag ya va en el wrapper. No se recomienda `sudo`.

## Checklist (usuario estándar)

- [ ] Usuario no root (o root de laboratorio Kali aceptando el modelo del wrapper).
- [ ] AppImage `chmod +x`; nombre `Zajuna.App-<ver>.AppImage`.
- [ ] FUSE2 presente **o** `--appimage-extract-and-run`.
- [ ] Health en `http://127.0.0.1:<puerto>/api/health`.
- [ ] Datos bajo `~/.local/share/zajuna-app` (o `$XDG_DATA_HOME/zajuna-app`).
- [ ] Log del launcher bajo `userData` de Electron.
- [ ] Al cerrar: sin `zajuna-core` huérfanos.
- [ ] Mensajes de guía **sin** pedir `sudo` / `--no-sandbox` manual / desactivar SELinux de forma permanente.

## Casos negativos (vs v0.1.1)

| Caso | Resultado esperado | Evidencia / estado |
|---|---|---|
| Sin `libfuse.so.2` (Kali rolling) | Fallo de mount; extract-and-run | MDL-123 |
| Sin `$DISPLAY` | Con wrapper headless: arranca; sin wrapper: crash Ozone | MDL-123 |
| Root sin flag en argv real | FATAL; con wrapper: arranca | MDL-123 |
| `XDG_DATA_HOME` sin permiso de escritura | Core no crea datos; error en log | A validar; no inventar pass |
| Dos instancias | Single-instance lock; segunda reabre URL | Código actual |
| Kill -9 con captura en curso | Process group evita huérfanos del core; perfiles Playwright = MDL-176 | MDL-123 |
| Docs sugiriendo `sudo` / `--no-sandbox` | **No debe ocurrir** | Revisado en este PR |

## Definition of Done (esta nota)

Cumple el entregable documental de MDL-161. **No** cierra MDL-161 ni MDL-176:
el criterio “no desactivar sandbox” sigue abierto hasta aceptación formal del
riesgo o empaquetado con sandbox viable.
