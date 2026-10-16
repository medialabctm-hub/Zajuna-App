# Guía de instalación y primer uso de Zajuna Sync

Esta guía te acompaña paso a paso, desde la descarga hasta tener tu primer
reporte. Está pensada para instructores: no hace falta saber programar ni
abrir una terminal (salvo en Linux, o si quieres comprobar el archivo
descargado).

**Resumen del recorrido**

```text
1. Revisar requisitos → 2. Descargar → 3. Instalar (aviso de SmartScreen)
→ 4. Abrir y conectar tu cuenta de Zajuna → 5. Primer uso:
   fichas → actividades → evidencias → revisión → reporte PDF
```

- Windows: secciones 1 a 9.
- Linux: sección 10.
- macOS no está soportado.

---

## 1. Requisitos

| Requisito | Qué significa en la práctica |
|---|---|
| **Windows 10 u 11 de 64 bits**, o Linux de 64 bits | En Windows: Configuración → Sistema → Acerca de; el «Tipo de sistema» debe decir *64 bits*. |
| **2 GB libres** como mínimo | El instalador pesa unos 350 MB, pero instalado ocupa más de 1 GB (incluye el navegador interno que toma las capturas). Reserva 5–10 GB si vas a capturar muchas evidencias. |
| **4 GB de RAM** (8 GB recomendado) | Las capturas abren un navegador interno; con poca memoria, evita lanzar muchas a la vez. |
| **Internet** | Solo para entrar a Zajuna. La aplicación en sí corre en tu computador. |
| **Cuenta de Zajuna** | Tipo de documento, número y contraseña. Se piden al abrir la aplicación por primera vez, no durante la instalación. |
| **Un navegador** | Chrome, Edge o Firefox. La aplicación se muestra ahí. |

---

## 2. Descargar el instalador

1. Abre la página de
   [versiones publicadas (Releases)](https://github.com/ElBrayayinn/Zajuna-App/releases/latest).
2. Baja hasta la sección **Assets** y descarga:
   - **Windows:** el archivo `.exe`, por ejemplo `Zajuna.Sync.Setup-0.1.6.exe`.
     (Las versiones antiguas se llamaban `Zajuna.App.Setup-<versión>.exe`.)
   - **Linux:** el archivo `.AppImage` (ver sección 10).
3. El número de versión puede cambiar; descarga siempre la más reciente.

Descarga el instalador **solo** desde esa página o desde el canal que te
indique el equipo de Medialab.

### Opcional: comprobar que el archivo no se dañó

En la misma página hay un archivo `release-manifest-windows.json` (o
`release-manifest.json`) con la «huella» (SHA256) de cada instalador. Si
quieres comprobarla:

1. Deja el `.exe` y el manifiesto en la misma carpeta (por ejemplo Descargas).
2. Clic derecho en un espacio vacío de la carpeta → **Abrir en Terminal**.
3. Escribe (cambiando el nombre si tu archivo es otro):

   ```powershell
   Get-FileHash -Algorithm SHA256 ".\Zajuna.Sync.Setup-0.1.6.exe"
   ```

4. Compara el valor `Hash` con el `sha256` del manifiesto. Deben ser iguales
   (mayúsculas o minúsculas no importan). Si no coinciden, **no instales**:
   borra el archivo y descárgalo de nuevo.

---

## 3. Ejecutar el instalador (Windows)

1. En el Explorador de archivos, ve a **Descargas**.
2. Haz doble clic en `Zajuna.Sync.Setup-….exe`.
3. Si aparece una pantalla azul de Windows, sigue el paso 4. Si no, salta al
   paso 5.

---

## 4. Aviso «Windows protegió su PC» (SmartScreen)

**Por qué sale.** El instalador todavía no tiene firma digital, así que
Windows no reconoce al editor y, en algunos equipos, frena el primer intento.
En otros equipos no aparece nada. Es esperado y no significa que el archivo
tenga un virus.

**Lo que ves.** Un recuadro azul con el título *Windows protegió su PC* y el
texto *Microsoft Defender SmartScreen impidió que se iniciara una aplicación
no reconocida*. El único botón visible es **No ejecutar**.

**Qué pulsar, en este orden:**

1. **Más información** (enlace debajo del texto; en inglés, *More info*).
2. Comprueba que el nombre del archivo sea `Zajuna.Sync.Setup-….exe`.
3. **Ejecutar de todas formas** (en inglés, *Run anyway*).

Esto no apaga SmartScreen: solo autoriza este archivo esta vez. **No
desactives** Windows Defender ni SmartScreen.

Otras situaciones:

- **Edge avisa al descargar** que el archivo no se descarga con frecuencia:
  pulsa **…** → **Conservar** (o **Mostrar más** → **Conservar de todas
  formas**). Luego ábrelo desde Descargas.
- **No aparece «Más información»**, solo *No ejecutar*: el equipo tiene una
  política que bloquea programas sin firma. Pide al administrador de ese
  equipo que autorice el instalador.

---

## 5. Instalación

El instalador es de **un clic**: no hace preguntas.

1. Verás una barra de progreso. Espera uno o dos minutos (copia también el
   navegador interno que toma las capturas).
2. Al terminar, la aplicación se abre sola.

La instalación es solo para tu usuario de Windows (queda en
`%LOCALAPPDATA%\Programs\zajuna-app`), así que normalmente **no pide permiso
de administrador**. Se crean accesos directos llamados **Zajuna Sync** en el
menú Inicio y en el escritorio.

---

## 6. Abrir la aplicación y conectar tu cuenta

Zajuna Sync **no abre una ventana propia**: se muestra en una pestaña de tu
navegador predeterminado.

1. Si no se abrió sola, abre **Zajuna Sync** desde el menú Inicio o el
   escritorio.
2. Espera unos segundos. El navegador mostrará una dirección parecida a
   `http://127.0.0.1:52741/` (el número cambia cada vez). Esa dirección es tu
   propio computador; nadie más puede abrirla.
3. Verás la pantalla **Conecta tu cuenta de Zajuna**. Completa:
   - **Tipo de documento:** Cédula de ciudadanía (CC), Tarjeta de identidad
     (TI) o Cédula de extranjería (CE).
   - **Número de documento.**
   - **Contraseña de Zajuna.**
4. Pulsa **Continuar**. Aparece el aviso *Cuenta conectada correctamente*.

**Sobre el inicio de sesión.** Zajuna pide el ingreso en dos pasos (primero
el portal del SENA y después el aula virtual). No tienes que hacer nada: la
aplicación hace los dos pasos por ti con los datos de este formulario, cada
vez que necesita entrar.

Tu contraseña se guarda en el almacén seguro del sistema (el Administrador de
credenciales de Windows), no en un archivo. Si Zajuna pide una verificación
adicional (CAPTCHA o código), la aplicación se detiene y te avisa; resuélvela
entrando a Zajuna en tu navegador y vuelve a intentarlo.

Para confirmar que Zajuna acepta tus datos, puedes ir a **Configuración** y
pulsar **Probar conexión**.

---

## 7. Primer uso: de la ficha al reporte

### 7.1 La primera carga (automática)

Justo después de conectar la cuenta aparece la pantalla **Primer arranque**:

1. *Buscando tus fichas*: la aplicación trae tus cursos desde Zajuna.
2. *Preparando las rutas de tus cursos*: localiza las secciones de cada ficha
   donde están las evidencias.

Solo pasa la primera vez. Puedes pulsar **Continuar sin esperar** y seguir
trabajando; la búsqueda continúa por detrás. Si falla, verás **Buscar mis
fichas** o **Reintentar rutas** para intentarlo de nuevo.

Al terminar llegas a **Resumen**. En la parte de arriba de las pantallas de
trabajo hay una barra de pasos que siempre te dice cuál es el **siguiente
paso**.

### 7.2 Menú lateral

| Pantalla | Para qué sirve |
|---|---|
| **Resumen** | Avance de la ficha activa, siguiente paso y botón **Preparar evidencias**. |
| **Fichas** | Lista de tus fichas. **Sincronizar fichas** las actualiza desde Zajuna; **Seleccionar** elige la ficha activa; **Buscar rutas** lee el contenido de su curso. |
| **Checklist** | Cada punto del checklist con su estado (sin evidencia, por revisar, para corregir, confirmado) y dónde se encontró la evidencia. |
| **Guías** | Lo que solo tú puedes hacer dentro de Zajuna, paso a paso (por ejemplo, publicar algo que falta). |
| **Actividades** | Marca las actividades técnicas que tú calificas. Las transversales aparecen bloqueadas. |
| **Evidencias** | Galería de capturas y archivos. **Agregar una evidencia** sube un archivo tuyo. |
| **Revisión** | Revisión automática de cada evidencia (aprobada, pendiente, rechazada) y tu decisión final. |
| **Trabajos** | Lo que la aplicación está haciendo o hizo (sincronizar, capturar, generar reportes) y si algo necesita atención. |
| **Reportes** | Genera el reporte final y guarda una copia. |
| **Configuración** | Credenciales de Zajuna, **Probar conexión**, preferencias de captura, almacenamiento, copias de seguridad y versión instalada. |
| **Diagnóstico** | Estado de la aplicación, incidencias recientes y una guía rápida. |
| **Notificaciones** | Avisos de los trabajos terminados o con problemas. |

### 7.3 El recorrido recomendado

1. **Elige tu ficha.** En **Fichas**, pulsa **Seleccionar** en la ficha con
   la que vas a trabajar. Si no aparece, pulsa **Sincronizar fichas**.
2. **Busca las rutas del curso** (si la aplicación no lo hizo ya): botón
   **Buscar rutas** en Fichas o en Resumen.
3. **Selecciona tus actividades.** En **Actividades**, marca las actividades
   técnicas que tú calificas.
4. **Prepara las evidencias.** En **Resumen**, pulsa **Preparar evidencias**.
   La aplicación entra a Zajuna y toma las capturas. Puedes ver el avance en
   **Trabajos**; puede tardar varios minutos.
5. **Revisa.** En **Revisión**, aprueba las evidencias correctas. Si alguna
   salió mal, usa **Volver a capturar**. En **Checklist** ves el estado de
   cada punto y en **Guías** lo que te toca hacer a ti en Zajuna.
6. **Genera el reporte.** En **Reportes**, deja el formato **PDF** y pulsa
   **Generar PDF**. Cuando termine, usa **Abrir PDF** para verlo o guardarlo.

Un punto del checklist solo cuenta como cumplido cuando todas sus evidencias
están bien: si falta algo o hay errores, no se puede marcar como cumplido, ni
siquiera a mano.

### 7.4 Cerrar la aplicación

- Si cierras **solo la pestaña**, la aplicación sigue funcionando por detrás
  (por ejemplo, terminando capturas). Volver a abrir el acceso directo abre
  otra pestaña de la misma aplicación; no se instala ni se abre otra copia.
- Para cerrarla del todo en Windows: abre el **Administrador de tareas**
  (`Ctrl` + `Mayús` + `Esc`), busca **Zajuna Sync** y pulsa **Finalizar
  tarea**.

---

## 8. Solución de problemas frecuentes

| Qué ves | Qué hacer |
|---|---|
| SmartScreen sin «Más información» | El equipo bloquea programas sin firma. No desactives Defender; pide al administrador que autorice el instalador. |
| «No hay espacio suficiente» | Libera al menos 2 GB en el disco `C:` y vuelve a ejecutar el instalador. |
| El antivirus pone el `.exe` en cuarentena | Restáuralo **solo** si lo descargaste de la página oficial. No añadas exclusiones para todo el equipo. |
| La instalación se corta a medias | Desinstala (sección 9) y vuelve a ejecutar el instalador. |
| No se abre el navegador | Abre **Zajuna Sync** otra vez desde el menú Inicio. Si sigue sin abrir, revisa el registro en `%APPDATA%\Zajuna Sync\logs\zajuna-core.log` (en versiones antiguas, `%APPDATA%\zajuna-app\logs\`). |
| *Esta pestaña no tiene una sesión local válida* | Abriste una dirección guardada o una pestaña vieja. Cierra esa pestaña y abre **Zajuna Sync** desde su acceso directo. |
| *No pudimos contactar a la aplicación local* | La aplicación se cerró. Ábrela otra vez desde el acceso directo y pulsa **Reintentar**. |
| Error al conectar la cuenta | Revisa tipo y número de documento, contraseña e internet. Comprueba que puedes entrar a Zajuna en el navegador. Luego usa **Configuración › Probar conexión**. |
| *Todavía no tienes fichas* / *No pudimos traer tus fichas* | Revisa la conexión y las credenciales y pulsa **Buscar mis fichas**, o sigue y sincroniza después desde **Fichas**. |
| Zajuna pide CAPTCHA o un código | La aplicación no puede resolverlo por ti. Entra a Zajuna en tu navegador, completa la verificación y vuelve a intentar. |
| Una captura falló o salió mal | Mira el detalle en **Trabajos**; reintenta desde ahí o usa **Volver a capturar** en **Revisión**. |
| Abro el acceso directo y no pasa nada | La aplicación ya está abierta: busca su pestaña en el navegador o revisa el Administrador de tareas. |

Si necesitas ayuda, cuenta qué pantalla ves, el mensaje exacto y los pasos
que hiciste. **Nunca** compartas tu contraseña.

---

## 9. Actualizar y desinstalar

### Cada versión nueva empieza desde cero

Instalar una versión nueva de Zajuna Sync **siempre** empieza con los datos
vacíos, sin preguntar. Lo mismo pasa con las actualizaciones automáticas. El
checklist vuelve a 0 % y no se conservan evidencias, actividades ni trabajos
de la versión anterior. Antes de actualizar:

- genera y guarda el **reporte PDF** de las fichas que necesites conservar;
- si quieres una copia de tus datos, descárgala desde
  **Configuración › Copias de seguridad** (la instalación nueva también borra
  las copias guardadas dentro de la aplicación).

**Actualizaciones automáticas.** La aplicación revisa si hay una versión
nueva al abrirse, la descarga en segundo plano y la instala **cuando cierras**
la aplicación (nunca a mitad de una captura). Como el instalador no tiene
firma digital, Windows puede bloquear esa actualización; en ese caso descarga
el instalador nuevo desde la página de versiones (sección 2).

Para empezar de cero sin reinstalar: **Configuración › Almacenamiento ›
Restablecer la aplicación** (botón **Restablecer datos**). En
**Configuración › Acerca de** ves la versión instalada y la carpeta de datos.

### Desinstalar en Windows

1. Configuración de Windows → **Aplicaciones** → **Aplicaciones instaladas**.
2. Busca **Zajuna Sync** → **⋯** → **Desinstalar**, y confirma.

Desinstalar borra también los datos de la aplicación: el avance del
checklist, las evidencias, las actividades seleccionadas, los reportes, los
trabajos y las copias de seguridad (carpeta `%LOCALAPPDATA%\ZajunaApp`), y
los registros (`%APPDATA%\Zajuna Sync` y `%APPDATA%\zajuna-app`). La
contraseña guardada en el Administrador de credenciales de Windows se
reemplaza la próxima vez que conectes la cuenta.

Las versiones 0.1.2 y anteriores no borraban nada al desinstalarse. Si vienes
de una de ellas, la versión nueva detecta esos datos antiguos en su primer
arranque y los borra.

Para reinstalar, vuelve a la sección 3 con el instalador oficial.

---

## 10. Linux (AppImage)

En Linux la aplicación es un único archivo `.AppImage`: no se instala, se
ejecuta. Entorno validado: Kali Linux (basado en Debian). No hay
aviso de SmartScreen.

### Requisitos

| Recurso | Mínimo | Recomendado |
|---|---|---|
| Procesador | 4 núcleos | 8 núcleos |
| Memoria RAM | 4 GB | 8 GB |
| Almacenamiento | 2 GB libres para la aplicación | + espacio para capturas (depende de cuántas tomes) |

Con 8 núcleos y 8 GB hay margen para dos o tres capturas simultáneas; con el
mínimo, evita lanzar muchas a la vez.

Para comprobar tu equipo:

```bash
nproc      # núcleos de CPU
free -h    # memoria RAM
df -h .    # espacio libre
```

La contraseña se guarda en el llavero del escritorio (GNOME Keyring, KWallet
u otro compatible con *Secret Service*); debe estar disponible en tu sesión.

### Primera ejecución

1. Descarga el `.AppImage` desde la
   [página de versiones](https://github.com/ElBrayayinn/Zajuna-App/releases/latest).
   Se llama `Zajuna.Sync-<versión>.AppImage` (las versiones antiguas se
   llamaban `Zajuna.App-<versión>.AppImage`). Si quieres, comprueba su SHA256
   con `sha256sum` y el `release-manifest-linux.json`.
2. Abre una terminal en la carpeta de descargas, dale permiso de ejecución y
   ábrelo (cambia el nombre por el de tu archivo):

   ```bash
   cd ~/Descargas   # o ~/Downloads
   chmod +x Zajuna.Sync-0.1.6.AppImage
   ./Zajuna.Sync-0.1.6.AppImage
   ```

3. Se abre tu navegador con la pantalla **Conecta tu cuenta de Zajuna**.
   Desde aquí todo es igual que en Windows (secciones 6 y 7).

Se recomienda abrirla con tu usuario habitual, no como root (aunque root
también funciona).

### Instalación opcional en una ubicación global

Si quieres iniciarla escribiendo `zajunaapp` desde cualquier terminal:

```bash
sudo mkdir -p /opt/ZajunaApp
sudo mv ~/Descargas/Zajuna.Sync-0.1.6.AppImage /opt/ZajunaApp/Zajuna.Sync.AppImage
sudo chmod +x /opt/ZajunaApp/Zajuna.Sync.AppImage
sudo ln -sf /opt/ZajunaApp/Zajuna.Sync.AppImage /usr/local/bin/zajunaapp
```

### Actualizar

Al abrir una versión nueva se borran los datos de la anterior
(`~/.local/share/zajuna-app`, o `$XDG_DATA_HOME/zajuna-app`). Genera antes los
reportes PDF que necesites. Si usaste la instalación global, reemplaza el
archivo conservando el mismo nombre:

```bash
sudo cp Zajuna.Sync-0.1.7.AppImage /opt/ZajunaApp/Zajuna.Sync.AppImage
sudo chmod +x /opt/ZajunaApp/Zajuna.Sync.AppImage
```

### Desinstalar

Borra el archivo `.AppImage` (y, si hiciste la instalación global,
`/opt/ZajunaApp` y `/usr/local/bin/zajunaapp`). Para borrar también tus datos,
elimina la carpeta `~/.local/share/zajuna-app`.

### Problemas frecuentes en Linux

**«Permission denied» al ejecutar**: falta el permiso de ejecución.

```bash
chmod +x Zajuna.Sync-0.1.6.AppImage
```

**`dlopen(): error loading libfuse.so.2`**: el formato AppImage necesita la
biblioteca `libfuse2`. Ubuntu y sus derivadas la ofrecen (`libfuse2t64` o `libfuse2`);
instalarla resuelve el error:

```bash
sudo apt update
apt search libfuse2
```

Kali Linux y Debian rolling ya no la empaquetan. Ahí, ábrela así, sin
necesidad de FUSE (detalle en
[`packaging/linux-kali-fuse2.md`](packaging/linux-kali-fuse2.md)):

```bash
./Zajuna.Sync-0.1.6.AppImage --appimage-extract-and-run
```

Para reportar un problema, incluye la distribución y versión de Linux, la
arquitectura, el mensaje exacto de la terminal y los pasos previos. Nunca
incluyas contraseñas ni datos personales.

---

## Notas para el equipo técnico

- La firma digital del instalador (Authenticode) está pendiente
  ([MDL-29](https://linear.app/medialab-sena/issue/MDL-29),
  [`signing.md`](signing.md)). Cuando exista, el aviso de SmartScreen debería
  desaparecer y esta guía se actualizará. Sin firma, la actualización
  automática puede fallar en Windows y el fallo queda en el registro
  `zajuna-core.log`.
- [MDL-28](https://linear.app/medialab-sena/issue/MDL-28): la página pública
  `downloads.html` **no** enseña a continuar SmartScreen y trata el
  instalador sin firma como bloqueo de release comercial. Esta guía es el
  camino operativo para instructores mientras no haya firma.
- La actualización automática consulta GitHub Releases de
  `medialabctm-hub/Zajuna-App`.
- Desarrollo en esta estación (npm, Go, Electron): [`run-local.md`](run-local.md).
