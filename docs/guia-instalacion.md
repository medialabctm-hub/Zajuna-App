# Guía de instalación de Zajuna Sync

Esta guía acompaña **cada pantalla** desde la descarga hasta el primer uso.
Está pensada para un instructor o usuario final en **Windows 10/11 de 64 bits**.
No hace falta saber programar ni abrir una terminal, salvo para comprobar el
checksum si el equipo de entrega te lo pide.

Hoy el instalador **no tiene firma Authenticode**. Por eso, en algunos equipos
aparece **SmartScreen**. Eso es esperado y temporal: se cierra cuando exista
certificado ([MDL-29](https://linear.app/medialab-sena/issue/MDL-29)). Mientras
tanto, **no desactives** Windows Defender ni SmartScreen de forma permanente.

```text
Descargar → (opcional) SHA256 → Ejecutar .exe
    → ¿SmartScreen? → Más información → continuar
    → Asistente / barra de instalación
    → Abrir Zajuna Sync → Setup (documento + contraseña)
    → Resumen en el navegador
```

Linux (AppImage) va al final, como anexo. macOS no es una plataforma soportada.

---

## 1. Antes de instalar

Comprueba esto en el equipo:

| Requisito | Qué significa en la práctica |
|---|---|
| Windows 10 u 11, **64 bits** | En Configuración → Sistema → Acerca de, la edición debe ser x64. |
| **2 GiB libres** como mínimo | El instalador pesa unos 350 MB, pero al descomprimir (Electron + Chromium) ocupa más de 1 GiB. Reserva 2 GiB; 5–10 GiB si vas a capturar muchas evidencias. |
| Conexión a internet | Solo para entrar a Zajuna (HTTPS). La app en sí corre en tu PC (`127.0.0.1`). |
| Cuenta de Zajuna | Tipo de documento, número y contraseña. No se piden en el instalador; salen en el **primer arranque**. |

Descarga **solo** el archivo que te entregue el equipo de Medialab / el canal
oficial. El nombre típico es:

```text
Zajuna.App.Setup-0.1.0.exe
```

El número de versión puede cambiar. Si te pasan también un
`release-manifest.json`, úsalo en el paso 2. Si no hay manifiesto, salta a
«Ejecutar el instalador» y no inventes un checksum.

---

## 2. Comprobar el archivo (si hay manifiesto)

1. Deja el `.exe` y el `release-manifest.json` en la misma carpeta (por
   ejemplo Descargas).
2. Abre PowerShell en esa carpeta: clic derecho en un espacio vacío +
   **Abrir en Terminal** (o *Abrir la ventana de PowerShell aquí*).
3. Ejecuta, cambiando el nombre si tu archivo es otro:

```powershell
Get-FileHash -Algorithm SHA256 ".\Zajuna.App.Setup-0.1.0.exe"
```

4. Compara el valor `Hash` con el `sha256` del manifiesto. Deben coincidir
   **exactamente** (mayúsculas o minúsculas no importan).
5. Si no coinciden: **no instales**. Borra el archivo y pide de nuevo el
   instalador oficial.

---

## 3. Ejecutar el instalador

1. En el Explorador de archivos, ve a Descargas (o la carpeta donde lo
   guardaste).
2. Doble clic en `Zajuna.Sync.Setup-….exe`.
3. Windows puede pedir permiso de administrador (**Control de cuentas de
   usuario**). Si el diálogo muestra *Zajuna Sync*, pulsa **Sí**.

Si en vez del instalador aparece una pantalla azul/amarilla de Windows, ve al
paso 4. Si el asistente arranca de una vez, salta al paso 5.

---

## 4. SmartScreen: «Windows protegió su PC»

**Por qué sale.** El instalador actual no lleva firma Authenticode (a veces
llamada AuthCode). Windows no reconoce al editor y, en **algunos** equipos,
bloquea el primer clic. En otros no aparece nada: depende de la versión de
Windows, de SmartScreen y de la política del PC.

**Pantalla que ves.** Un recuadro de Windows, a menudo con un escudo o un
icono de advertencia. Título típico: *Windows protegió su PC*. Texto típico:
*Microsoft Defender SmartScreen impidió que se iniciara una aplicación no
reconocida*. El botón grande suele ser **No ejecutar**.

**Qué pulsas, en este orden:**

1. **Más información** (abajo a la izquierda; en inglés: *More info*).
   Hasta que no pulses eso, no aparece la opción de continuar.
2. Revisa que el nombre del archivo sea el instalador oficial
   (`Zajuna.Sync.Setup-….exe`).
3. **Ejecutar de todas formas** (en inglés: *Run anyway*).
4. Si pide administrador, **Sí**.

Eso **no** apaga SmartScreen para siempre. Solo autoriza este archivo en esta
ocasión.

Otras variantes que puedes ver:

- En **Edge**, al descargar: aviso de que el archivo no se suele descargar.
  Usa **…** / **Conservar** / **Mostrar más** y confirma conservar el archivo.
  Luego ábrelo desde Descargas; ahí puede salir el diálogo del paso 4.
- El botón puede decir *Run anyway* si Windows está en inglés.
- Si **no** ves «Más información» y solo hay *No ejecutar*: el equipo tiene
  una política que bloquea ejecutables sin firma. No intentes desactivar
  Defender. Pide al administrador de esa máquina una excepción, o espera el
  instalador firmado (MDL-29).

---

## 5. Asistente de instalación

El empaquetado es NSIS (electron-builder). En la mayoría de equipos verás un
instalador de **un clic** o una barra de progreso corta:

1. Acepta el Control de cuentas de usuario si aún no lo hiciste.
2. Espera a que copie los archivos (puede tardar uno o dos minutos: lleva
   Chromium para las capturas).
3. Al terminar, deja marcada la opción de **ejecutar Zajuna Sync** si aparece,
   o pulsa **Finalizar**.

Windows crea un acceso en el menú Inicio llamado **Zajuna Sync**. En muchos
equipos también hay acceso en el escritorio.

Si el asistente muestra *Siguiente* / *Elegir carpeta* / *Instalar*, deja la
ruta por defecto (`Archivos de programa\Zajuna Sync`) salvo que te hayan
indicado otra.

---

## 6. Primer arranque (Setup)

Zajuna Sync **no abre una ventana propia**. Electron arranca en segundo plano,
enciende el core en `127.0.0.1` y abre tu **navegador predeterminado**.

1. Si el instalador no la lanzó, abre **Zajuna Sync** desde el menú Inicio.
2. Espera a que el navegador muestre la app. La dirección será algo como
   `http://127.0.0.1:#####/` (el puerto cambia).
3. En el primer uso verás **Conecta tu cuenta de Zajuna** (título *Tu espacio
   de trabajo local* a la izquierda). Completa:
   - **Tipo de documento:** Cédula de ciudadanía (CC), Tarjeta de identidad
     (TI) o Cédula de extranjería (CE).
   - **Número de documento.**
   - **Contraseña de Zajuna.**
4. Pulsa **Continuar**. Debe aparecer el aviso *Cuenta conectada
   correctamente* y pasar a **Resumen**.

La app no abre un icono en la bandeja: si cierras el navegador, el proceso
sigue en segundo plano (paso 7).

La contraseña se guarda en el almacén del sistema (Credential Manager en
Windows), no en un archivo a la vista. Las fichas, evidencias y reportes
quedan en este equipo.

Si Zajuna pide CAPTCHA u otra verificación, la guía de la propia pantalla te
indica que la resuelvas en el navegador. No hace falta “saltar” ese paso.

---

## 7. Cómo saber que quedó bien

- El navegador muestra **Zajuna Sync · Operación local** y la vista **Resumen**.
- En el menú lateral puedes abrir Fichas, Checklist, Actividades, Evidencias,
  Reportes, Configuración y Diagnóstico.
- Si cierras **solo la pestaña**, la app **sigue corriendo**. Volver a pulsar
  el acceso directo reabre la misma URL; no instala una segunda copia.

**Cómo salir de verdad.** Cerrar Chrome/Edge no apaga el core. En Windows:

1. Abre el Administrador de tareas (`Ctrl` + `Mayús` + `Esc`).
2. Busca **Zajuna Sync**.
3. Finaliza esa tarea.

La próxima vez que uses el acceso directo, volverá a arrancar limpio.

---

## 8. Si algo falla

| Qué ves | Qué hacer |
|---|---|
| SmartScreen sin «Más información» | Política del equipo. No desactives Defender. Pide excepción o el instalador firmado. |
| «No hay espacio suficiente» | Libera al menos 2 GiB en `C:` y vuelve a ejecutar el Setup. |
| El antivirus pone el `.exe` en cuarentena | Restaura el archivo desde la cuarentena **solo** si viene del canal oficial. No añadas exclusiones globales. |
| El instalador se corta a medias | Desinstala (paso 9), borra la carpeta a medias en Archivos de programa si quedó, y reintenta. |
| El navegador no abre | Abre **Zajuna Sync** otra vez. Si sigue igual, revisa Diagnóstico cuando logres entrar, o el log en `%APPDATA%\zajuna-app\logs\zajuna-core.log`. |
| La página queda en blanco / no carga | Confirma que no cerraste el proceso de Zajuna Sync. Prueba de nuevo el acceso directo. |
| Error al guardar la cuenta | Revisa documento y contraseña de Zajuna, y que haya internet. El Setup no sustituye el login de la plataforma. |
| «Ya hay una instancia» / no pasa nada | La app ya está corriendo: mira las pestañas del navegador o el Administrador de tareas. |

---

## 9. Desinstalar y volver a intentar

1. Configuración de Windows → **Aplicaciones** → **Aplicaciones instaladas**.
2. Busca **Zajuna Sync** → **Desinstalar**.
3. Confirma el desinstalador.
4. Vuelve al paso 3 con el mismo `.exe` oficial (o uno nuevo que te pasen).

Desde la versión **0.1.3**, desinstalar borra también los datos locales de
Zajuna Sync: el avance del checklist, las evidencias, las actividades
seleccionadas, los reportes, los trabajos y las copias de seguridad, que viven
en `%LOCALAPPDATA%\ZajunaApp`. También se borran los registros de
`%APPDATA%\zajuna-app`. La contraseña de Zajuna guardada en el Administrador
de credenciales de Windows se reemplaza la próxima vez que configures la cuenta.

### Cada versión nueva empieza desde cero

Instalar una versión nueva de Zajuna Sync **siempre** empieza con los datos
vacíos, sin preguntar, y lo mismo pasa con las actualizaciones automáticas. Así
el checklist arranca en 0 % y no aparecen evidencias, actividades ni trabajos
de una versión anterior. Antes de instalar una versión nueva:

- genera y guarda el **reporte PDF** de las fichas que necesites conservar;
- si quieres una copia de tus datos, descárgala desde
  **Configuración › Copias de seguridad** (la instalación nueva también borra
  las copias guardadas dentro de la app).

Para empezar de cero sin reinstalar, usa
**Configuración › Almacenamiento › Restablecer la aplicación**. En
**Configuración › Acerca de** ves la versión instalada y la carpeta de datos.

Las versiones 0.1.2 y anteriores no borraban nada al desinstalarse. Si vienes de
una de ellas, la 0.1.3 detecta esos datos antiguos en su primer arranque y los
borra.

---

## 10. Anexo: Linux (AppImage)

En Linux el artefacto es un AppImage, no un Setup. Entorno validado: Kali
Linux (basado en Debian). Comprueba el SHA256 del manifiesto, marca el
archivo como ejecutable (`chmod +x`) y ábrelo. No hay SmartScreen. Si el
escritorio bloquea un binario no firmado, no eludas esa protección: usa el
canal oficial y el checksum.

### Requisitos del equipo

Las capturas usan Chromium/Puppeteer, así que el consumo depende también de
cuántas instancias corran en paralelo:

| Recurso | Mínimo | Recomendado |
|---|---|---|
| Procesador | 4 núcleos | 8 núcleos |
| Memoria RAM | 4 GB | 8 GB |
| Almacenamiento | 2 GB libres dedicados a la app | + espacio adicional para capturas (varía según resolución, formato y frecuencia) |

Con 8 núcleos y 8 GB hay margen para dos o tres capturas simultáneas; con el
mínimo, evita abrir muchas a la vez. No trates los 2 GB como el espacio total
de operación: reserva más si vas a capturar mucho y supervisa la carpeta de
evidencias.

### Primera ejecución

El artefacto de CI se llama `Zajuna.Sync-<versión>.AppImage` (sin espacios; puntos en el nombre de producto).

```bash
cd ~/Downloads   # o ~/Descargas
ls -lh
chmod +x Zajuna.App-0.1.1.AppImage
ls -l Zajuna.App-0.1.1.AppImage      # confirmar el permiso
./Zajuna.App-0.1.1.AppImage
```

Se recomienda iniciarla con el usuario habitual del sistema, no como root, por
buena práctica de mínimo privilegio — pero root también funciona (empaquetado
verificado en Kali Linux, MDL-123).

Comprobación rápida de recursos antes de instalar:

```bash
nproc      # CPU disponible
free -h    # memoria RAM
df -h .    # espacio libre
```

### Instalación opcional en una ubicación global

AppImage no necesita "instalarse" para funcionar, pero si quieres iniciarla
desde cualquier terminal:

```bash
sudo mkdir -p /opt/ZajunaApp
sudo mv ~/Downloads/Zajuna.App-0.1.1.AppImage /opt/ZajunaApp/Zajuna.App.AppImage
sudo chmod +x /opt/ZajunaApp/Zajuna.App.AppImage
sudo ln -sf /opt/ZajunaApp/Zajuna.App.AppImage /usr/local/bin/zajunaapp
```

Después se inicia con `zajunaapp` desde cualquier carpeta.

### Actualizar

Al abrir una versión nueva, el AppImage borra los datos de la versión anterior
(`$XDG_DATA_HOME/zajuna-app`, por defecto `~/.local/share/zajuna-app`). Genera
antes los reportes PDF que necesites. Reemplaza el archivo conservando el mismo
nombre y repite el permiso de ejecución:

```bash
sudo cp Zajuna.App-0.1.1.AppImage /opt/ZajunaApp/Zajuna.App.AppImage
sudo chmod +x /opt/ZajunaApp/Zajuna.App.AppImage
```

### Solución de problemas comunes

**"Permission denied" al ejecutar** — falta el permiso de ejecución:

```bash
chmod +x Zajuna.App-0.1.1.AppImage
./Zajuna.App-0.1.1.AppImage
```

**`dlopen(): error loading libfuse.so.2` al ejecutar** — el runtime del
AppImage necesita `libfuse2` (API FUSE 2.x). Ubuntu la trae como paquete de
transición (`libfuse2t64`), pero **Kali Linux/Debian rolling ya no la
empaquetan en absoluto** — solo ofrecen `fuse3`/`libfuse3-4` (verificado en
Kali 2026.3, MDL-123). No hay paquete que instalar para resolverlo en Kali:
usa la extracción temporal, que no depende de FUSE (detalle en [`packaging/linux-kali-fuse2.md`](packaging/linux-kali-fuse2.md)):

```bash
./Zajuna.App-0.1.1.AppImage --appimage-extract-and-run
```

En otras distribuciones que sí ofrezcan `libfuse2`/`libfuse2t64`, instalarla
resuelve el mismo error sin necesitar `--appimage-extract-and-run`:

```bash
sudo apt update
apt search fuse | grep -E "libfuse|fuse"
```

Para reportar un problema, incluye distribución y versión de Linux,
arquitectura, el mensaje exacto de la terminal y los pasos previos. Nunca
incluyas credenciales, tokens ni información sensible.

---


---

## Actualizaciones automáticas (escritorio)

En builds empaquetados, Electron comprueba GitHub Releases
(`medialabctm-hub/Zajuna-App`) después de arrancar el core. La descarga ocurre
en segundo plano y la instalación se aplica **al cerrar** la app (no se fuerza
un reinicio a mitad de una captura).

Una actualización instala una versión nueva, así que también empieza con los
datos vacíos (ver «Cada versión nueva empieza desde cero»). Genera tus reportes
PDF antes de cerrar la app cuando haya una actualización descargada.

**Windows sin firma Authenticode:** SmartScreen o la política del equipo pueden
bloquear la descarga o la aplicación del parche. CSC/Authenticode sigue siendo
**opcional** en el pipeline; sin certificado, el auto-update puede fallar y eso
queda en el log (`%APPDATA%\zajuna-app\logs\zajuna-core.log`). Mientras no haya
firma (MDL-29), la vía segura es descargar el instalador oficial y comprobar el
SHA256 del manifiesto.

## Relación con otras tareas

- [MDL-29](https://linear.app/medialab-sena/issue/MDL-29): firma Authenticode.
  Cuando exista, este aviso de SmartScreen debería desaparecer y esta guía se
  actualizará.
- [MDL-28](https://linear.app/medialab-sena/issue/MDL-28): la página pública
  `downloads.html` **no** enseña a continuar SmartScreen; trata el artefacto
  sin firma como bloqueo de release comercial. Esta guía es el camino
  **operativo** para candidatos internos / instructores mientras no haya firma.

Desarrollo en esta estación (npm, Go, Electron): [`run-local.md`](run-local.md).
