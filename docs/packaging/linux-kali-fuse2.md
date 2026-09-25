# AppImage en Kali / Debian rolling (FUSE2)

## Problema

El runtime AppImage de electron-builder necesita `libfuse.so.2`. Kali rolling y
Debian testing/sid **ya no empaquetan** `libfuse2` (solo `fuse3` / `libfuse3`).

Al ejecutar el `.AppImage` aparece:

```
dlopen(): error loading libfuse.so.2
AppImages require FUSE to run...
```

## Workaround recomendado

No requiere instalar paquetes obsoletos:

```bash
chmod +x Zajuna.App-*.AppImage
./Zajuna.App-*.AppImage --appimage-extract-and-run
```

También puedes extraer una sola vez:

```bash
./Zajuna.App-*.AppImage --appimage-extract
./squashfs-root/AppRun
```

## Nombre del artefacto

El archivo publicado por CI se llama `Zajuna.App-<versión>.AppImage` (sin espacios; puntos en el nombre de producto, para que coincida con el manifiesto de electron-updater / GitHub Releases).

## Datos de usuario

Desde la 0.1.3, abrir un AppImage de otra versión vacía
`~/.local/share/zajuna-app` en el primer arranque. Ver
[`../evidence/update-policy.md`](../evidence/update-policy.md).
