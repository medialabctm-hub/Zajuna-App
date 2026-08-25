# Gate de integración — 2026-08-25

Issue: [MDL-34](https://linear.app/medialab-sena/issue/MDL-34).
Estación: Windows 10, Node v24.17.0, npm 11.13.0, Go 1.26.3.
Commit de trabajo: rama `feat/m2-ci-native-gate`.

**Decisión: release bloqueado.** No se afirma matriz verde.

| Comando | Resultado | Notas |
|---|---|---|
| `go -C core test ./...` | exit 0 | Incluye fixtures de CAPTCHA/sesión |
| `go -C core vet ./...` | exit 0 | |
| `npm run lint --prefix frontend` | exit 0, 0 warnings | oxlint |
| `npm run build --prefix frontend` | exit 0 | Vite 8.2.1 |
| `node scripts/prepare-downloads.test.cjs` | 3 passed | Sin lenguaje de bypass |
| `node scripts/smoke-native.test.cjs` | 4 passed | |
| `npm run smoke:native` | exit 0, `releaseBlocked: true` | Sin `dist/` de instalador ni `CSC_LINK` |
| `TestAuthenticatedZajunaE2E` | no ejecutado | Falta `ZAJUNA_E2E=1` y cuenta de prueba |
| `npm run test:smoke:packaged` | no ejecutado | No hay `win-unpacked` en esta corrida |
| `npm run package:macos` / `package:linux` | no ejecutado | Host Windows; Chromium de otro OS prohibido |
| Authenticode / notarización | no ejecutado | Sin certificados |
| NVDA / VoiceOver | no ejecutado | Sin lector en la estación |

El workflow `.github/workflows/ci.yml` cubre frontend, Go y descargas en cada
PR. El job `native` (Windows/macOS/Linux + firma) solo corre con
`workflow_dispatch` y el input `native=true`.

Acta: [`committee-minutes-2026-08-25.md`](committee-minutes-2026-08-25.md).
Firma: [`signing.md`](signing.md).
