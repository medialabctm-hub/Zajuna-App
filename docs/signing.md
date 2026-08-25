# Firma y smoke nativo

Los secretos de firma **nunca** se versionan ni se pegan en Linear, logs o
artefactos. El pipeline lee variables de entorno; si faltan, el instalador
queda como **bloqueo de release**.

## Secretos de CI (GitHub Actions)

Configúralos en el repositorio, no en el código:

| Variable | Uso |
|---|---|
| `CSC_LINK` | PKCS#12 / certificado Authenticode (Windows) o identidad de electron-builder |
| `CSC_KEY_PASSWORD` | Contraseña del certificado |
| `APPLE_ID` | Cuenta Apple para notarización |
| `APPLE_APP_SPECIFIC_PASSWORD` | Contraseña de aplicación |
| `APPLE_TEAM_ID` | Team ID de Developer ID |

El workflow `.github/workflows/ci.yml` deja `CSC_IDENTITY_AUTO_DISCOVERY=false`
salvo que exista un secreto. Así un runner con certificado de desarrollador
ajeno no firma por accidente.

## Comandos locales

```powershell
npm run smoke:native
npm run test:smoke:native
```

`smoke:native` inspecciona `dist/`, calcula SHA256 y, en Windows, consulta
Authenticode. Escribe `dist/native-smoke-report.json`. Sin artefactos o sin
firma válida el reporte marca `releaseBlocked: true` y **no falla** (es un
inventario). Para exigir firma:

```powershell
$env:ZAJUNA_REQUIRE_SIGNED = '1'
npm run smoke:native
```

El job `native` de Actions solo corre con *workflow_dispatch* y el input
`native=true`. Necesita runners Windows, macOS y Linux; esta estación no
sustituye esos logs.

## Qué no hace este documento

No hay un certificado del cliente en el repo. No se afirma notarización,
Authenticode válido ni smoke de DMG/AppImage hasta que existan esos artefactos.
