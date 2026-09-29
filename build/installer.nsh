; Zajuna Sync: datos locales (%LOCALAPPDATA%\ZajunaApp) en instalación y
; desinstalación. El core Go guarda ahí la base SQLite, evidencias, reportes,
; copias y config.json; electron-builder no toca esa carpeta, por eso el
; checklist, las evidencias y los trabajos sobrevivían a una reinstalación.
;
; Regla de producto: cada instalación de Zajuna Sync empieza desde cero. No se
; pregunta: el instalador siempre ordena borrar los datos anteriores y el
; desinstalador manual siempre los borra. El core aplica además la misma
; regla por versión (backup.EnforceVersion), lo que cubre las actualizaciones
; automáticas y Linux (AppImage no tiene instalador).
;
; Verificado contra las plantillas de app-builder-lib 26.15.3. makensis corre
; con -WX: no declarar Var/Function que no se usen.

; ---- Instalación: siempre empezar desde cero ------------------------------
; No borramos aquí (el core viejo podría estar terminando): escribimos
; .reset-pending con "full" y el core lo aplica antes de abrir SQLite, en su
; primer arranque tras la instalación. Corre después de desinstalar la
; versión anterior y copiar archivos, antes de lanzar la app.
!macro customInstall
  Push $R0
  ${if} $installMode == "all"
    SetShellVarContext current
  ${endif}
  ${if} ${FileExists} "$LOCALAPPDATA\ZajunaApp\*.*"
    ClearErrors
    FileOpen $R0 "$LOCALAPPDATA\ZajunaApp\.reset-pending" w
    ${ifNot} ${Errors}
      FileWrite $R0 "full"
      FileClose $R0
    ${endif}
  ${endif}
  ${if} $installMode == "all"
    SetShellVarContext all
  ${endif}
  Pop $R0
!macroend

; ---- Desinstalación manual: borrar siempre los datos ----------------------
!macro customUnInstall
  ; ${isUpdated}: el instalador nuevo ejecuta el desinstalador viejo con
  ; --updated. En ese caso no borramos aquí; lo hace customInstall + core.
  ${ifNot} ${isUpdated}
    Push $R0
    ; CHECK_APP_RUNNING sin PowerShell solo termina "Zajuna Sync.exe":
    ; aseguramos que el core (abre zajuna.db) y su Chromium hayan terminado.
    nsExec::Exec `"$SYSDIR\cmd.exe" /C taskkill /F /T /FI "USERNAME eq %USERNAME%" /IM zajuna-core.exe`
    Pop $R0
    Sleep 500

    ${if} $installMode == "all"
      SetShellVarContext current
    ${endif}

    RMDir /r "$LOCALAPPDATA\ZajunaApp"
    RMDir /r "$APPDATA\zajuna-app"
    RMDir /r "$APPDATA\Zajuna Sync"
    RMDir /r "$LOCALAPPDATA\zajuna-app-updater"

    ${if} $installMode == "all"
      SetShellVarContext all
    ${endif}
    Pop $R0
  ${endif}
!macroend
