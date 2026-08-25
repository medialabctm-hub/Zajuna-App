# Acta de comité — 2026-08-25

Proyecto Linear: [Zajuna-App](https://linear.app/medialab-sena/project/zajuna-app-6d193f12dc3d).
Issue madre: [MDL-25](https://linear.app/medialab-sena/issue/MDL-25).

## Decisión

**Release comercial: bloqueado.** M0/M1 están en código y el PR #3 ya está en
`main`. M2 no está cerrado: no hay certificados de firma, no hay smoke nativo
de macOS/Linux y no hay E2E autenticado contra Zajuna real.

## Hechos de la estación (Windows)

- `main` sincronizado con `origin/main` (`f9602dd`, merge del PR #3).
- Los commits citados en Linear el 24 ago (`72f9fb7`, `302c078`) no existían
  en el repositorio; el trabajo de CI/firma y la evidencia de accesibilidad
  se versiona hoy en la rama `feat/m2-ci-native-gate`.
- No hay `CSC_LINK`, `APPLE_ID` ni `ZAJUNA_E2E` en el entorno.

## Issues

| Issue | Estado al cierre de esta acta | Nota |
|---|---|---|
| MDL-26 … MDL-31, MDL-28, MDL-27 | Done | M0/M1 de código en `main` |
| MDL-32 | Done en Linear; evidencia ahora en git | NVDA/VoiceOver no ejecutados |
| MDL-29 | In Progress | CI + `smoke:native`; firma bloqueada sin certificado |
| MDL-33 | In Progress | Fixtures sí; E2E vivo no |
| MDL-34 | In Progress | Matriz local + esta acta; gate no aprueba release |

## Próximos pasos

1. Cargar secretos de firma en GitHub y disparar el workflow `native`.
2. Cuenta de prueba para `ZAJUNA_E2E=1`.
3. Pasada NVDA/VoiceOver cuando haya lector en la estación.
