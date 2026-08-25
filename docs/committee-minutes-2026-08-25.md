# Acta de comité — 2026-08-25

Proyecto Linear: [Zajuna-App](https://linear.app/medialab-sena/project/zajuna-app-6d193f12dc3d).
Issue madre: [MDL-25](https://linear.app/medialab-sena/issue/MDL-25).

## Decisión

**Release comercial: bloqueado.** M0/M1 están en código. M2: instaladores
Windows/Linux sin firma; macOS aplazado. El E2E autenticado de login+sync
pasó en esta estación (credenciales solo en entorno).

## Hechos de la estación (Windows)

- `main` sincronizado con `origin/main` (`f9602dd`, merge del PR #3).
- Los commits citados en Linear el 24 ago (`72f9fb7`, `302c078`) no existían
  en el repositorio; el trabajo de CI/firma y la evidencia de accesibilidad
  se versiona hoy en la rama `feat/m2-ci-native-gate`.
- `ZAJUNA_E2E=1` se ejecutó en esta estación; credenciales no se versionan.
- No hay `CSC_LINK`. macOS no entra al release.

## Issues

| Issue | Estado al cierre de esta acta | Nota |
|---|---|---|
| MDL-26 … MDL-31, MDL-28, MDL-27 | Done | M0/M1 de código en `main` |
| MDL-32 | In Review | Evidencia en PR #4; NVDA no ejecutado |
| MDL-29 | In Progress | CI Win/Linux; firma Windows bloqueada; macOS aplazado |
| MDL-33 | In Review | E2E vivo: login + sync de fichas OK |
| MDL-34 | In Progress | Gate no aprueba release (sin firma) |

## Próximos pasos

1. Certificado Authenticode para Windows cuando exista; Linux sigue por SHA256.
2. Mergear PR #4 y disparar job `native` (Win/Linux).
3. Pasada NVDA cuando haya lector en la estación.
