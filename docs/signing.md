# Firma y smoke nativo

Alcance actual de instaladores: **Windows y Linux**. macOS queda fuera hasta
que exista certificado Developer ID; no se construye ni se firma DMG en CI.

Los secretos de firma **nunca** se versionan ni se pegan en Linear, logs o
artefactos. El pipeline lee variables de entorno; si faltan, el instalador
queda como **bloqueo de release**.

## Secretos de CI (GitHub Actions)

Configúralos en el repositorio, no en el código:

| Variable | Uso | Obligatorio ahora |
|---|---|---|
| `CSC_LINK` | PKCS#12 / certificado Authenticode (Windows) | Cuando se firme Windows |
| `CSC_KEY_PASSWORD` | Contraseña del certificado | Con `CSC_LINK` |

Linux publica AppImage + SHA256; no usa Authenticode ni Apple ID.

El workflow `.github/workflows/ci.yml` deja `CSC_IDENTITY_AUTO_DISCOVERY=false`
salvo que exista `CSC_LINK`. El job `native` solo corre Windows y Ubuntu.

## Comandos locales

```powershell
npm run smoke:native
npm run test:smoke:native
npm run package:windows
```

`package:linux` debe ejecutarse en un runner Linux (Chromium del host).
`package:macos` permanece en el repo pero **no forma parte del release** hasta
nuevo aviso.

`smoke:native` inspecciona `dist/`, calcula SHA256 y, en Windows, consulta
Authenticode. Sin firma válida el reporte marca `releaseBlocked: true`.

## Qué no hace este documento

No hay certificado del cliente en el repo. No se afirma Authenticode válido
ni instalador macOS.
