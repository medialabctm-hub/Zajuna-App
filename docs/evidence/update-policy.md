# Datos locales al instalar, actualizar o restablecer

## Carpeta de datos

| Plataforma | Carpeta de datos |
|---|---|
| Linux | `$XDG_DATA_HOME/zajuna-app` (por defecto `~/.local/share/zajuna-app`) |
| Windows | `%LOCALAPPDATA%\ZajunaApp` |

Ahí viven `zajuna.db`, `config.json`, `evidences/`, `reports/`, `exports/` y
`backups/`. La contraseña de Zajuna no está en esta carpeta: vive en el
almacén de credenciales del sistema. Los logs del launcher están aparte, en la
carpeta de Electron (`%APPDATA%\zajuna-app\logs` en Windows).

## Invariantes

Estas reglas se cumplen con cualquier política de actualización:

1. **El schema solo avanza.** Al abrir la base, el core aplica las migraciones
   pendientes en una transacción (v1…v14 hoy) y nunca abre una base de una
   versión más nueva. Una migración que falla no deja la base a medias.
2. **Nada se borra en caliente.** Los borrados masivos (restablecer, cambio de
   versión, restauración) se aplican al arrancar, antes de abrir SQLite, y no
   mientras workers o Chromium usan los archivos.
3. **La base es obligatoria para un borrado.** Si `zajuna.db` no se puede
   borrar (por ejemplo, otro proceso la tiene abierta en Windows), el core
   conserva los datos y reintenta en el siguiente arranque. Archivos sueltos
   que queden se recogen después como huérfanos.
4. **Las copias de seguridad se validan antes de usarse.** Un restore comprueba
   SHA-256, `PRAGMA integrity_check`, las tablas mínimas y que el schema esté
   en `1…CurrentSchemaVersion`; si la base restaurada no abre, se vuelve a la
   anterior.
5. **Borrar evidencias es siempre explícito** con `POST /api/evidences/clear`
   o Configuración; ningún flujo de actualización lo invoca por su cuenta.
6. **Los builds de desarrollo** (`appVersion` = `dev`, el que genera
   `npm run core:build`) nunca borran datos por versión.

## Comportamiento vigente (0.1.3 en adelante)

Por decisión de producto, **cada versión instalada empieza con los datos
vacíos**, copias de seguridad incluidas. Lo implementan tres piezas:

| Pieza | Cuándo actúa | Qué borra |
|---|---|---|
| `backup.EnforceVersion` (core) | Arranque de un core empaquetado cuyo `.app-version` no coincide con su versión, o que encuentra datos sin marcador (versiones ≤ 0.1.2). Cubre actualizaciones automáticas y AppImage. | Base, config, evidencias, reportes, exportaciones y `backups/`. |
| `customInstall` en `build/installer.nsh` | Cada instalación en Windows con datos previos: escribe `.reset-pending` con `full` y el core lo aplica en su primer arranque. | Igual que el anterior. |
| `customUnInstall` en `build/installer.nsh` | Desinstalación manual (no la que ejecuta el instalador al actualizar). | Toda la carpeta de datos y las de Electron/updater. |

Antes de instalar o aceptar una actualización, el usuario debe generar los
reportes PDF que necesite y descargar la copia de seguridad que quiera
conservar fuera de la app (guía en
[`../guia-instalacion.md`](../guia-instalacion.md)).

## Restablecer sin reinstalar

`POST /api/app/reset` (Configuración › Almacenamiento › Restablecer la
aplicación) escribe `.reset-pending` y apaga el core; bajo el launcher, el
supervisor lo vuelve a iniciar. En el siguiente
arranque borra base, config, evidencias, reportes y exportaciones, pero
**conserva `backups/`**, incluida la copia previa que puede crear la misma
petición (`backupFirst`).

## Una evidencia vigente por ranura

Desde el schema v13, la base guarda **una evidencia actual** por
`(ficha, ítem, ranura, origen)`. Las capturas nuevas reemplazan la anterior del
mismo slot; el checklist muestra solo las vigentes y respeta `max_evidences`
del catálogo.

Una misma captura puede respaldar varios ítems (varias filas con el mismo
archivo). Al reemplazar, recortar por `max_evidences`, podar el checklist o
eliminar con `DELETE /api/evidences/{id}`, el archivo solo se borra cuando ya
ninguna fila lo referencia.

## Cómo reiniciar evidencias

Si un docente necesita partir de cero (sin desinstalar):

1. **API:** `POST /api/evidences/clear`  
   Cuerpo opcional: `{ "fichaId": "<id>" }` para limitar a una ficha.  
   Sin `fichaId` elimina todas las evidencias locales y archivos huérfanos.
2. **Interfaz:** Ajustes → Datos → «Borrar evidencias locales».

La actualización de la app **nunca** ejecuta este reinicio sola.

## Restauración de respaldos

Tras restaurar un backup, el core reconcilia `evidences/` con las filas de la
base y elimina archivos que ya no estén referenciados.

## Si se cambia a una política que conserve datos

Si producto decide que actualizar conserve los datos, las invariantes de
arriba ya bastan para migrar de forma segura. Los puntos a cambiar son solo
`backup.EnforceVersion` (dejar de borrar al cambiar de versión y solo
actualizar `.app-version`), `customInstall` en `build/installer.nsh` (no
escribir `.reset-pending`) y la sección «Cada versión nueva empieza desde
cero» de la guía de instalación. `POST /api/app/reset` y
`POST /api/evidences/clear` seguirían siendo los borrados explícitos.
