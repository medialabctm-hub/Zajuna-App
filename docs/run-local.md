# Cómo corre Zajuna App en local

Zajuna App **ya no es un sitio web remoto**. Es una aplicación de escritorio
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

Si **ya compilaste** antes:

```powershell
npm run desktop:start
```

Cerrar la pestaña del navegador **no** apaga el core. Para salir, cierra
Electron (icono de la bandeja / proceso) o termina el terminal.

Un segundo `desktop:start` no duplica el backend: reabre la URL existente.

## Cómo la usará un instructor (instalador)

1. Instala `Zajuna App Setup …exe` (Windows) o el AppImage (Linux).
2. El acceso directo inicia el mismo launcher: core local + navegador.
3. En el primer arranque aparece **Setup**: documento y contraseña de Zajuna.
4. A partir de ahí, Resumen, fichas, checklist, evidencias y reportes son
   locales. Los backups ZIP también son locales.

macOS no se entrega en este release. Ver [`macos-deferred.md`](macos-deferred.md).

## Pruebas rápidas

```powershell
go -C core test ./...
npm run lint --prefix frontend
npm run test:downloads
```

El E2E contra Zajuna real usa variables de entorno (`ZAJUNA_E2E=1`); nunca
se guardan credenciales en git.

## Si algo no abre

- Revisa que exista `core/bin/zajuna-core.exe` (salida de `npm run build`).
- El core solo escucha en loopback; no publiques el puerto.
- Logs del core (desarrollo empaquetado): carpeta de datos de Electron,
  `logs/zajuna-core.log`.
