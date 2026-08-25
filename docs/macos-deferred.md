# Por qué no se trabaja el instalador de macOS

**Decisión (2026-08-25):** el release actual solo produce instaladores
**Windows (NSIS)** y **Linux (AppImage)**. El DMG de macOS y su notarización
quedan **fuera de alcance** hasta que Medialab tenga certificado Apple
Developer ID.

## Motivo

1. Un instalador macOS usable en equipos ajenos exige firma **Developer ID** y
   notarización de Apple. Sin eso, Gatekeeper bloquea la app y no hay canal
   oficial que podamos documentar.
2. El empaquetado debe correr **en un Mac** (Chromium/Playwright es del host).
   Compilar el core Go para `darwin` desde Windows no sustituye ese paso.
3. Hoy no hay fecha para esos certificados. Seguir el DMG en CI haría fallar
   o fingir un artefacto que no se puede distribuir.

## Qué queda en el repo

- `npm run package:macos` existe por si más adelante hay Mac + certificado.
- `build:platforms` sigue generando binaries Go `macos-x64` y `macos-arm64`.
- El job `native` de `.github/workflows/ci.yml` **no** incluye `macos-latest`.

## Cuándo se retoma

Cuando existan `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD` y `APPLE_TEAM_ID` en
GitHub Secrets (nunca en git ni en Linear) y un runner macOS. Entonces se
vuelve a añadir macOS al job nativo y a [MDL-29](https://linear.app/medialab-sena/issue/MDL-29).

Detalle de firma Windows/Linux: [`signing.md`](signing.md).
Cómo arrancar la app: [`run-local.md`](run-local.md).
