# Auditoría de accesibilidad WCAG 2.1 AA

Fecha de la revisión: **2026-08-26**  
Alcance: frontend React embebido en el core Go. Las nueve rutas operativas
tienen pasada de teclado, zoom 200 % y reflow 320 CSS px registrada abajo.
NVDA/VoiceOver siguen sin ejecutar.

Esta revisión sigue la guía de accesibilidad del proyecto y separa las
comprobaciones automatizadas de las que necesitan una persona con teclado,
zoom y lector de pantalla. No se declara conformidad WCAG completa hasta
terminar la pasada manual.

## Comprobaciones automatizadas realizadas

`npm run test:visual:core` ejecuta el smoke con Chromium empaquetado en tres
viewport deterministas (1440×900, 1024×900 y 390×844). El test:

- compara cada captura con un SHA-256 de baseline para detectar regresiones de
  composición;
- verifica que no exista overflow horizontal y que el lateral se oculte en
  móvil;
- revisa que cada `button`, enlace, campo y selector visible tenga nombre
  accesible mediante texto, `aria-label`, `title` o una etiqueta asociada;
- espera las fuentes locales antes de capturar, desactiva motion/transitions
  durante el snapshot y deja el estado reproducible.

La hoja global contiene foco visible para controles (`:focus-visible`),
`sr-only` para texto auxiliar y una regla `prefers-reduced-motion` para
reducir animaciones. El menú lateral móvil tiene botón, overlay,
`aria-expanded`, cierre con Escape y restauración del foco. Las imágenes de
contenido tienen `alt`; las miniaturas decorativas se mantienen vacías cuando
la tarjeta ya ofrece el nombre textual.

## Contraste de tokens principales

El cálculo relativo sobre fondo blanco da los siguientes ratios (objetivo AA:
4.5:1 para texto normal y 3:1 para texto grande):

| Token | Ratio | Resultado |
|---|---:|---|
| `--ink` `#0b2432` | 16.00:1 | Pasa |
| `--muted` `#5f7683` | 4.77:1 | Pasa para texto normal |
| `--brand-action` `#2a7a00` | 5.41:1 | Pasa |
| `--no` `#c0392b` | 5.44:1 | Pasa |
| `--pending` `#9a5c00` | 5.38:1 | Pasa |

Los estados sobre fondos suaves, iconos y combinaciones de borde requieren
revisión contextual durante la prueba manual; el ratio de un token aislado no
garantiza que todas sus variantes cumplan.

## Pasada manual pendiente antes de declarar conformidad

1. ~~Navegar las nueve rutas solo con teclado~~ **Hecho 2026-08-26** (Chromium empaquetado; ver matriz).
2. ~~Probar NVDA en Windows~~ **Hecho 2026-08-26** (NVDA 2026.1.1 portable, Chromium headed).
   VoiceOver en macOS: **pendiente / otro día**.
3. ~~Revisar zoom al 200% y reflow en 320 CSS px~~ **Hecho 2026-08-26.**
4. ~~Confirmar objetivos táctiles de al menos 44×44 CSS px para acciones
   principales~~ **Hecho para controles primarios**; `.button.small` queda como residual.
5. Contraste contextual de badges/estados: los chips llevan texto además de color;
   tokens aislados siguen pasando AA. Revisar de nuevo si cambian fondos.

Estas verificaciones no se pueden afirmar desde un test DOM aislado. El
resultado de esta ronda debe registrarse aquí con fecha, navegador/lector y
hallazgos antes del release.

## Actualización después de la remediación paralela

- Las pestañas de Configuración exponen `tablist`, `tab`, `tabpanel` y
  navegación por teclado.
- Filtros, estados del checklist y procesos anuncian `aria-pressed`.
- Las previsualizaciones de evidencias atrapan el foco y el borrado solicita
  confirmación.
- El menú móvil deja accesible la navegación a 320 CSS px sin ocultar las
  rutas detrás de un sidebar imposible de abrir.
- El smoke visual ahora espera las fuentes locales, fuerza rasterización
  estable y mantiene una comprobación funcional del `<select>` de ficha; los
  hashes actuales son desktop `46f04e57…`, tablet `b2f5b778…` y mobile
  `33f27618…`.

## Resultado del bloque

La base automatizada de nombres accesibles, foco, reduced motion y responsive
queda integrada en el smoke visual.

## Pasada 2026-08-26 (MDL-32)

Ejecutada sobre Chromium empaquetado (Windows). Fecha, runtime y acta:
[`committee-minutes-2026-08-26.md`](committee-minutes-2026-08-26.md).

### Matriz de nueve rutas

| Ruta | Teclado | Zoom 200 % | Reflow 320 CSS px | NVDA/VoiceOver |
|---|---|---|---|---|
| `/resumen` | Skip link, Tab, `main`, `h1` único, toasts `aria-live` | Contenido usable | Sin overflow de documento | NVDA: skip link, título, nav |
| `/fichas` | Igual; tabla con scroll interno | Igual | Sin overflow de documento | NVDA: shell + “Fichas” |
| `/checklist` | `aria-pressed` en filtros y estados | Igual | Rail y filtros reflow | NVDA: shell |
| `/actividades` | Igual | Igual | Toolbar a una columna | NVDA: “página actual · Actividades” |
| `/evidencias` | Preview con Escape y retorno de foco (código) | Igual | Galería y acciones relacionadas reflow | NVDA: shell |
| `/trabajos` | Filtros `aria-pressed` | Igual | Meta apilada | NVDA: shell |
| `/reportes` | Acciones primarias 44 px | Igual | Igual | NVDA: shell |
| `/configuracion` | `tablist` + flechas; tabs inactivas `tabindex=-1` | Igual | Grid a una columna | NVDA: shell |
| `/diagnostico` | Landmarks del shell | Igual | Igual | NVDA: shell |

### Criterios de Linear

- Teclado documentado en las nueve rutas: **sí**.
- NVDA (Windows): **sí** (2026.1.1 portable, `TestNVDAScreenReaderPass`). VoiceOver: **bloqueo explícito** (macOS otro día).
- Zoom 200 % y reflow 320 CSS px: **sí**, con remediación de overflow en Checklist y Evidencias.
- Información crítica no depende solo de color o motion: estados llevan texto (`SI`/`NO`/`PENDIENTE`, chips con etiqueta) y `prefers-reduced-motion` está activo.
- Hallazgos P0/P1: el nombre “Fichas0” se remedia en el mismo cambio (`aria-label="Fichas, N"`). No se abrieron issues hijas.
- No se declara WCAG 2.1 AA completa hasta VoiceOver.

### Remediación incluida en esta pasada

- Skip link “Saltar al contenido” hacia `#dashboard-main`.
- Toasts en región `aria-live="polite"`.
- Objetivos táctiles ≥ 44 CSS px en acciones primarias, skip link, icono de notificaciones y pestañas.
- Un solo `h1` por vista (título visible de página como `.page-head-title`).
- Reflow de checklist, galería y acciones relacionadas a 320 CSS px.
- Enlace Fichas: NVDA ya no concatena la cifra (`Fichas0`).

La evidencia de teclado vive en `TestWCAGKeyboardMatrix` (`ZAJUNA_RUN_BROWSER_SMOKE=1`). La de NVDA en `TestNVDAScreenReaderPass` (`ZAJUNA_RUN_NVDA=1`).

## Refuerzo multiplataforma 2026-09-01

- La matriz incluye ahora las diez rutas del shell, incluida
  `/notificaciones`, y usa un viewport de portátil de 1366×768.
- La automatización aproxima el espacio disponible con viewports efectivos
  equivalentes a escalas de 125 %, 150 % y 200 %, verificando el
  desbordamiento horizontal. La pasada manual de zoom continúa siendo la
  evidencia normativa.
- Los botones de acción, incluidas las variantes `.button.small`, y los campos
  de formulario mantienen un objetivo mínimo de 44×44 CSS px.
- La hoja global estabiliza el espacio de la barra de desplazamiento, el
  escalado de texto y los scrollbars entre Chromium/Firefox en Windows y
  Linux.
