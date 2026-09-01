# Prueba de instalación nativa — 2026-09-01 (MDL-29)

Protocolo y bitácora de la primera instalación de Zajuna App en máquinas
distintas a la estación de desarrollo. Cubre **Windows 10/11 x64** y **Linux
Mint 22.3 «Zena» x64**, que es la plataforma Linux objetivo del release.
macOS está fuera de alcance y Ubuntu no se usa como plataforma de prueba.

Este documento se llena **mientras** se ejecuta la prueba. Un casillero sin
resultado escrito significa "no ejecutado", nunca "pasó".

## Qué desbloqueó esta jornada

El workflow `Native installers` existía desde el 2026-08-26 pero **nunca
produjo un artefacto**: sus 7 ejecuciones registradas terminaban en 0 s.

| # | Causa | Corrección |
|---|---|---|
| 1 | El paso «Verify Windows Authenticode» leía el contexto `secrets` dentro de un `if` de paso, donde ese contexto no existe. GitHub invalidaba el workflow completo antes de arrancar. | Los secretos CSC pasan a `env` a nivel de job; el `if` lee `env.CSC_LINK`. |
| 2 | El runner Linux es headless y Electron exige un servidor X aunque la app no abra `BrowserWindow`. | El smoke empaquetado se separa por sistema; Linux corre bajo `xvfb-run`. |
| 3 | electron-builder detecta CI y activa publicación implícita: el AppImage se construía completo y luego abortaba con `GitHub Personal Access Token is not set`. | `scripts/package.cjs` pasa `--publish never` salvo que se pida lo contrario. |
| 4 | El smoke buscaba `dist/linux-unpacked/Zajuna App`. electron-builder nombra el binario Linux con `appInfo.sanitizedName` en minúscula, o sea el campo `name` → `zajuna-app`; `Zajuna App` solo aplica al `.exe`. | El smoke prueba ambos nombres y descubre el ejecutable en `linux-unpacked` si cambia. |
| 5 | El smoke lanzaba el paquete con `stdio: 'ignore'`: un arranque fallido solo se veía como «no expuso /api/health». | Se guarda una cola acotada de stdout/stderr y se adjunta al error. |
| 6 | Con la salida ya visible: Electron aborta con `FATAL: The SUID sandbox helper binary was found, but is not configured correctly` porque `chrome-sandbox` no es setuid root en `dist/linux-unpacked`. | Solo la invocación del smoke pasa `--no-sandbox`; la app entregada no cambia. **Queda abierto en Mint 22.3 real** (ver B1 y B5). |

## Artefactos bajo prueba

| Campo | Valor |
|---|---|
| Rama | `mdl-29-native-installers-fix` |
| Commits | `284e34d`, `d2eb8a1`, `76e4a87`, `0de73d8`, `25512ac` |
| Windows | `Zajuna App Setup 0.1.0.exe` (NSIS, x64) |
| Linux | `Zajuna App-0.1.0.AppImage` (x64), compilado en `ubuntu-24.04`, la base de Mint 22.3 |
| Firma Windows | **Ausente.** Sin certificado Authenticode (`CSC_LINK` no configurado). |
| Integridad | `release-manifest.json` con SHA256 por artefacto. |

Los SHA256 se copian aquí desde `release-manifest.json` antes de instalar, y se
vuelven a calcular en el PC de prueba. Si no coinciden, la prueba se detiene.

| Artefacto | SHA256 esperado | SHA256 en el PC de prueba |
|---|---|---|
| `Zajuna App Setup 0.1.0.exe` | `a375623903f7d49f6a7d036814cb20ee2742a35b1cabc60730d4b9589b906c45` | (pendiente) |
| `Zajuna App-0.1.0.AppImage` | `469dd81c62fe200093a997f77862c87cc10cf1cb424b77113ceeb867e6d0964b` | (pendiente) |

## Qué debe hacer la app si todo va bien

Zajuna App no es un sitio web. El acceso directo lanza un launcher Electron
**sin ventana propia**, que arranca el core Go escuchando solo en
`127.0.0.1:<puerto aleatorio>` y abre el navegador predeterminado contra esa
dirección. Cerrar la pestaña no apaga el core.

- Datos en Windows: `%LOCALAPPDATA%\ZajunaApp` (`config.json`, SQLite, evidencias).
- Datos en Mint: `~/.local/share/zajuna-app` (o `$XDG_DATA_HOME/zajuna-app`).
- Contraseña de Zajuna: almacén del sistema, servicio `zajuna-app`. Nunca en
  `config.json` ni en un `.env`.
- Archivo de endpoint: `zajuna-app-<pid>-<nonce>.json` en el directorio temporal.

## Bloque A — Windows 10/11 x64

### A0. Preparación

1. PC de prueba **sin** Node, Go ni el repo. Se prueba el instalador, no el build.
2. Anotar edición y build de Windows: `winver`.
3. Confirmar que no hay una instalación previa: Configuración → Aplicaciones →
   buscar «Zajuna». Si aparece, esta ya no es una instalación limpia.

### A1. Descargar y verificar integridad

```powershell
Get-FileHash '.\Zajuna App Setup 0.1.0.exe' -Algorithm SHA256
```

Comparar con `release-manifest.json`. **Si no coincide, detener la prueba.**

### A2. Advertencia de SmartScreen (hallazgo esperado)

El instalador **no está firmado**, así que SmartScreen mostrará «Windows
protegió su PC». Eso es el resultado correcto de esta versión, no un fallo de
la prueba: es la evidencia de que el release comercial sigue bloqueado hasta
que exista el certificado Authenticode.

- Capturar la advertencia. Es el artefacto que cierra ese criterio de MDL-29.
- La instalación continúa porque es un PC de la oficina que tú administras y
  el binario lo construyó nuestro propio CI. Es una decisión de QA registrada.
- **Esta instrucción no debe aparecer en la página pública de descargas** —
  MDL-28 eliminó justamente esa guía para usuarios finales.

### A3. Instalación limpia

| Paso | Qué observar | Resultado |
|---|---|---|
| Ejecutar el instalador | Termina sin error | |
| Carpeta de instalación | Anotar la ruta real | |
| Menú Inicio | Existe la entrada «Zajuna App» | |

### A4. Primer arranque

| Paso | Qué observar | Resultado |
|---|---|---|
| Abrir «Zajuna App» | Abre el navegador predeterminado en `127.0.0.1:<puerto>` | |
| Anotar el puerto | | |
| Pantalla inicial | Aparece **Setup** (tipo de documento y contraseña) | |

Salud del backend, sustituyendo el puerto observado:

```powershell
Invoke-RestMethod http://127.0.0.1:PUERTO/api/health | ConvertTo-Json
```

Se espera `status: ok`, `app: zajuna-app`, `runtime: windows`. Guardar la salida.

### A5. Credenciales en el almacén del sistema

Completar Setup con la cuenta de prueba de Zajuna. Después:

| Comprobación | Cómo | Resultado |
|---|---|---|
| La contraseña **no** está en `config.json` | `Get-Content "$env:LOCALAPPDATA\ZajunaApp\config.json"` | |
| Existe la credencial | Administrador de credenciales → Credenciales de Windows → `zajuna-app` | |

La contraseña no se escribe en este documento ni en Linear.

### A6. Instancia única y cierre limpio

| Paso | Qué observar | Resultado |
|---|---|---|
| Abrir el acceso directo por segunda vez | Reabre la misma URL; **no** aparece un segundo puerto | |
| `Get-Process zajuna-core` | Un solo proceso | |
| Cerrar la app (bandeja / cerrar el proceso del launcher Electron) | | |
| `Get-Process zajuna-core -ErrorAction SilentlyContinue` | **Vacío** — sin procesos huérfanos | |
| `Get-ChildItem $env:TEMP -Filter 'zajuna-app-*.json'` | **Vacío** — el archivo de endpoint se borró | |

### A7. Actualización sobre la instalación existente

Reinstalar el mismo `.exe` encima (simula la actualización).

| Comprobación | Resultado |
|---|---|
| El instalador no exige desinstalar primero | |
| La app arranca después | |
| `config.json` y la base SQLite se conservan | |

### A8. Desinstalación

Configuración → Aplicaciones → Zajuna App → Desinstalar.

| Comprobación | Cómo | Resultado |
|---|---|---|
| Binarios eliminados | La carpeta de A3 ya no existe | |
| Acceso directo eliminado | Menú Inicio | |
| Sin procesos vivos | `Get-Process zajuna-core -ErrorAction SilentlyContinue` | |
| Carpeta de datos | `Test-Path "$env:LOCALAPPDATA\ZajunaApp"` — anotar si queda | |
| Credencial | Administrador de credenciales — anotar si queda | |

Que los datos del usuario sobrevivan a la desinstalación es normal en un NSIS
por usuario. Lo que hay que **decidir y registrar** es si eso es lo que
queremos: son evidencias y una base SQLite con datos de fichas reales.

## Bloque B — Linux Mint 22.3 «Zena» x64

Plataforma Linux objetivo del release. Mint 22.3 se publicó el 2026-01-13, está
construida sobre **Ubuntu 24.04.3 LTS**, trae kernel 6.14 y tiene soporte hasta
abril de 2029. Esa base es la razón por la que el AppImage se compila en un
runner `ubuntu-24.04`: misma línea base de glibc y librerías del sistema.

No se prueba sobre Ubuntu. Si el PC disponible no es Mint 22.3, se anota la
versión real y el resultado no cuenta como evidencia de esta plataforma.

### B0. Identificar la máquina

```bash
cat /etc/os-release
uname -r
echo "$XDG_CURRENT_DESKTOP / $XDG_SESSION_TYPE"
```

| Dato | Esperado | Observado |
|---|---|---|
| `NAME` / `VERSION` | Linux Mint 22.3 (Zena) | |
| `UBUNTU_CODENAME` | `noble` | |
| Kernel | 6.14.x | |
| Escritorio / sesión | `X-Cinnamon` / `x11` o `wayland` | |

### B1. La comprobación que decide todo: sandbox de Chromium

**Hacer esto antes de instalar.** Un solo valor predice si la app arranca:

```bash
sysctl -n kernel.apparmor_restrict_unprivileged_userns
```

| Salida | Qué significa |
|---|---|
| `0`, o error «unknown key» | Los user namespaces sin privilegios están permitidos. El sandbox de Chromium debería funcionar y la app debería arrancar normal. |
| `1` | AppArmor bloquea el sandbox por namespaces. Electron abortará con `FATAL: The SUID sandbox helper binary was found, but is not configured correctly`, igual que en el runner de CI. |

Ubuntu 24.04 introdujo esa restricción y rompió los AppImage de Electron: un
AppImage no tiene paso de instalación, así que nadie crea un perfil AppArmor
para él, a diferencia de un `.deb` que lo instala en `/etc/apparmor.d/`. **Está
sin confirmar si Mint 22.3 hereda la restricción o la revierte**, y por eso se
mide en la máquina real en vez de asumirlo.

Anotar el valor observado: `______`

### B2. Dependencia de FUSE

Un AppImage necesita FUSE 2, que la base 24.04 ya no instala por omisión. En la
base `noble` el paquete se llama `libfuse2t64`:

```bash
sudo apt install libfuse2t64
```

Si esa versión del paquete no existe en los repos configurados, probar el nombre
antiguo `sudo apt install libfuse2`. Anotar cuál funcionó: `______`

Alternativa sin instalar nada, útil para no tocar el PC:

```bash
./"Zajuna App-0.1.0.AppImage" --appimage-extract-and-run
```

### B3. Herramientas para verificar el llavero

La contraseña de Zajuna va al Secret Service de D-Bus. Cinnamon usa
`gnome-keyring`, que Mint ya trae. Para poder inspeccionarlo:

```bash
sudo apt install libsecret-tools
```

### B4. Descargar y verificar integridad

```bash
sha256sum "Zajuna App-0.1.0.AppImage"
chmod +x "Zajuna App-0.1.0.AppImage"
```

Comparar con `release-manifest.json`. **Si no coincide, detener la prueba.**

### B5. Primer arranque

```bash
./"Zajuna App-0.1.0.AppImage"
```

| Paso | Qué observar | Resultado |
|---|---|---|
| Arranca sin abortar | Sin mensaje `FATAL` de sandbox | |
| Abre el navegador predeterminado en `127.0.0.1:<puerto>` | Mint trae Firefox | |
| Anotar el puerto | | |
| Aparece **Setup** | | |

```bash
curl -s http://127.0.0.1:PUERTO/api/health
```

Se espera `status: ok`, `app: zajuna-app`, `runtime: linux`. Guardar la salida.

**Si aborta con el error de sandbox SUID**, ese es el hallazgo principal de la
jornada. Anotar el mensaje textual y seguir el resto del protocolo con:

```bash
./"Zajuna App-0.1.0.AppImage" --no-sandbox
```

Ese `--no-sandbox` es solo para poder continuar la prueba: **no** es la
corrección. La corrección es una decisión de producto con tres caminos, y hay un
detalle técnico que la condiciona: `linux.executableArgs` de electron-builder
solo escribe argumentos en el `.desktop`, **no** en el AppRun del AppImage
(`LinuxTargetHelper.js`), así que no cubre a alguien que ejecute el AppImage
directamente desde la terminal.

| Camino | Costo | Consecuencia |
|---|---|---|
| Añadir `--no-sandbox` en `main.cjs` para Linux | Cambio de una línea, cubre todos los modos de lanzamiento | Desactiva el sandbox del launcher Electron, que no renderiza contenido: no abre `BrowserWindow` y la interfaz corre en el navegador del usuario. No toca el Chromium de capturas. |
| Pedir al instructor el sysctl `kernel.apparmor_restrict_unprivileged_userns=0` | Requiere `sudo` en cada PC | Debilita esa protección para todo el sistema, no solo para nuestra app. |
| Distribuir `.deb` en vez de AppImage | Trabajo de empaquetado nuevo | Un `.deb` puede instalar su perfil AppArmor y `chrome-sandbox` setuid, que es la vía soportada. |

### B6. Captura real: la prueba que sí tiene peso de seguridad

Esto es distinto del arranque. El core Go lanza un **segundo** Chromium, el de
Playwright, con `Headless: true` y **sandbox activo** (`browser.go`,
`pw.Chromium.Launch`). Ese sí carga contenido remoto de Zajuna, así que su
sandbox importa de verdad y no se debe desactivar a la ligera.

Si el sysctl de B1 dio `1`, es probable que las capturas también fallen. Hay que
medirlo:

| Paso | Qué observar | Resultado |
|---|---|---|
| Configurar la cuenta de prueba en Setup | | |
| Ejecutar una captura de evidencia desde la interfaz | Genera el PNG | |
| Si falla, anotar el error textual del core | | |
| Revisar el log del core | `~/.local/share/zajuna-app/logs/` | |

Si el arranque necesitó `--no-sandbox` **y** las capturas fallan, son dos
problemas con distinto peso: el del launcher es cosmético, el del Chromium de
capturas es de seguridad. Registrarlos por separado.

### B7. Credenciales

| Comprobación | Cómo | Resultado |
|---|---|---|
| La contraseña **no** está en `config.json` | `cat ~/.local/share/zajuna-app/config.json` | |
| Existe la credencial | `secret-tool search service zajuna-app` | |
| Si el llavero falla | Anotar el error textual de la app | |

La contraseña no se escribe en este documento ni en Linear.

### B8. Instancia única y cierre limpio

| Paso | Qué observar | Resultado |
|---|---|---|
| Ejecutar el AppImage otra vez | Reabre la misma URL; sin segundo puerto | |
| `pgrep -a zajuna-core` | Un solo proceso | |
| Cerrar la app | | |
| `pgrep -a zajuna-core` | **Vacío** — sin huérfanos | |
| `ls /tmp/zajuna-app-*.json` | **Vacío** | |

### B9. Actualización

Reemplazar el AppImage por el siguiente build y ejecutarlo.

| Comprobación | Resultado |
|---|---|
| Arranca con el binario nuevo | |
| `~/.local/share/zajuna-app` conserva `config.json` y la base | |

### B10. Desinstalación

Un AppImage no tiene desinstalador: se borra el archivo.

| Comprobación | Cómo | Resultado |
|---|---|---|
| Sin procesos vivos tras borrarlo | `pgrep -a zajuna-core` | |
| Datos que quedan | `du -sh ~/.local/share/zajuna-app` | |
| Credencial que queda | `secret-tool search service zajuna-app` | |
| Lanzador en el menú | Mint pregunta si integrar el AppImage al menú; anotar si queda un `.desktop` huérfano en `~/.local/share/applications` | |

## Resultado consolidado

| Criterio de MDL-29 | Windows 10/11 | Mint 22.3 «Zena» |
|---|---|---|
| Instalación limpia | | |
| Arranque y `/api/health` | | |
| Frontend React embebido servido | | |
| Contraseña fuera de `config.json` | | |
| Instancia única | | |
| Cierre sin procesos huérfanos | | |
| Actualización conservando datos | | |
| Desinstalación / borrado | | |
| Arranque sin desactivar el sandbox | No aplica | |
| Captura real con Chromium sandboxed | No aplica | |
| Firma verificada | **No.** Sin certificado Authenticode. | No aplica: integridad por SHA-256. |

## Bloqueos y decisiones

(pendiente de llenar al cerrar la jornada)
