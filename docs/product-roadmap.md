# Roadmap vigente de Zajuna App

El documento completo de migración está en [`desktop-migration.md`](desktop-migration.md).
Cómo arrancar: [`run-local.md`](run-local.md).
macOS aplazado: [`macos-deferred.md`](macos-deferred.md).

## Estado actual

| Área | Estado | Evidencia |
|---|---|---|
| Launcher Electron + core Go | Implementado | Launcher silencioso, endpoint dinámico, instancia única, recuperación y navegador predeterminado. |
| React embebido | Implementado | Vite → `go:embed`, PostCSS local, fallback SPA y API same-origin. |
| SQLite y secretos | Implementado | Schema v12, keyring, backups con hash/`integrity_check` y rollback si el restore no abre. |
| Jobs y scheduler | Implementado | CAS, un worker por job, recuperación de huérfanos al arrancar, eventos y schedules. |
| Checklist/evidencias/reportes | Implementado | 62 ítems, detalle, galería, capturas y PDF/HTML. |
| Configuración/diagnóstico/notificaciones | Implementado | APIs locales y vistas funcionales. |
| Fidelidad visual y accesibilidad automatizada | Implementado | Sistema de diseño, motion, responsive y smoke de tres viewports. |
| Seguridad OWASP | Hardening principal implementado | Capability, Host/Origin, anti-SSRF, cookies de captura acotadas, redacción y symlink guard. |
| Instalador Windows | Construido y probado | NSIS x64 con core + Chromium; **sin firma**. CI nativo opcional en Actions. |
| macOS | Aplazado | No hay certificado Developer ID; no entra al release actual. |
| Linux | Cross-build + AppImage en runner nativo | Checksum SHA256; sin firma de paquete. |

## Fases cerradas

### Fase 1 — Alcance e inventario

Se definió `Zajuna.App` como único destino y `zajuna-sync` como referencia no
modificable. Cada workflow se convirtió en un caso de uso local con trigger,
input, steps, progreso, resultado y error.

### Fase 2 — Runtime local

Se reemplazaron n8n, webhooks, MySQL, Docker y túneles por Go, SQLite,
workers, scheduler y Chromium empaquetado. La contraseña se trasladó al
almacén seguro del sistema operativo.

### Fase 3 — Dominio y persistencia

Se implementaron fichas, cursos, mapas de captura, checklist, slots,
evidencias, reportes, backups, settings, diagnóstico y notificaciones. El
schema actual es v12.

### Fase 4 — React y maqueta

La interfaz vanilla se migró a React 19 + TypeScript + Vite. Se portaron
tipografías, tokens, estados, animaciones, galerías, timeline, filtros y rutas
reales. Se añadieron menú móvil, foco, ARIA, confirmaciones y estados de error.

### Fase 5 — Entrega

El build sincroniza React dentro del core, genera seis targets Go, instala
Chromium en el runner nativo, hace staging por plataforma y produce metadata
SHA256/SBOM. El smoke empaquetado verifica `/api/health`.

### Fase 6 — Hardening M0/M1 (2026-08-20)

Se cerraron en código los bloqueadores de build/pruebas, las transiciones de
jobs, el restore seguro de SQLite y el aborto de CAPTCHA/MFA. Queda M2:
firma nativa, WCAG manual y gate de release. Ver Linear MDL-25.

## Plan de trabajo vigente (2026-08-25)

| Prioridad | Trabajo | Estado Linear |
|---|---|---|
| Hecho | M0/M1 en `main` (captura, build, descargas, jobs, backups) | MDL-26…31 Done |
| Hecho (código) | Login+sync E2E vivo (8 fichas); credenciales solo en entorno | MDL-33 In Review |
| En curso | CI e instaladores **Windows y Linux**; Windows sin Authenticode | MDL-29 In Progress |
| En curso | Accesibilidad: skip link/44px en PR #4; falta NVDA | MDL-32 In Review |
| En curso | Gate de release: acta roja hasta firmar Windows | MDL-34 In Progress |
| Aplazado | DMG macOS / notarización | Ver [`macos-deferred.md`](macos-deferred.md) |

Siguiente: mergear [PR #4](https://github.com/medialabctm-hub/Zajuna-App/pull/4),
smoke nativo Win/Linux, pasada NVDA, firma Windows cuando haya certificado.

### P0 — Antes de entregar una versión comercial

1. Firmar el instalador Windows cuando exista certificado Authenticode (MDL-29).
2. Construir y probar AppImage Linux en runner nativo (MDL-29).
3. Probar instalación limpia, actualización y desinstalación en Windows y Linux.
4. E2E autenticado: login+sync hecho; mapa/captura y sesión vencida en vivo pendientes (MDL-33).
5. macOS (DMG/notarización) **aplazado**. Motivo: [`macos-deferred.md`](macos-deferred.md).

### P1 — Antes de beta amplia

1. Pasada con NVDA (Windows) y VoiceOver (macOS); teclado/zoom ya documentados
   en [`accessibility-audit.md`](accessibility-audit.md) (MDL-32).
2. Registrar selectores y reglas por curso real en el E2E autenticado (MDL-33).
3. Cerrar el gate de integración cuando MDL-29 y MDL-33 tengan evidencia (MDL-34).

### P2 — Evolución posterior

1. Workflows administrativos restantes y adaptadores de correo/Slack/IMAP.
2. Logo, iconos, firma/notarización automatizada y actualización automática.
3. Optimización del tamaño de Chromium y pruebas de carga de capturas paralelas.

## Criterio de finalización

La versión comercial Windows/Linux estará lista cuando se instale en esos
sistemas, configure credenciales sin exponerlas, sincronice una ficha, capture
evidencia, genere PDF, recupere jobs, restaure un backup y pase las pruebas
acordadas. macOS no forma parte de este criterio hasta retomar
[`macos-deferred.md`](macos-deferred.md).
