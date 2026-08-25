# Auditoría de accesibilidad WCAG 2.1 AA

Fecha de la revisión: **2026-08-09**  
Alcance: frontend React embebido en el core Go, con foco inicial en
`/resumen` y en los componentes compartidos por las nueve rutas.

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

1. Navegar las nueve rutas solo con teclado: orden de tabulación, foco visible,
   activación con Enter/Espacio, cierre con Escape y retorno del foco después de
   abrir/cerrar modales y previsualizaciones.
2. Probar NVDA en Windows (y VoiceOver en macOS cuando haya equipo
   disponible): landmarks, encabezados, nombre/estado de switches, tablas,
   toasts, errores y cambios de estado durante polling.
3. Revisar zoom al 200% y reflow en 320 CSS px, incluida Configuración,
   Detalle de tarea, timeline y la galería de evidencias.
4. Confirmar objetivos táctiles de al menos 44×44 CSS px para acciones
   principales y que ninguna información dependa solo de color o motion.
5. Repetir la revisión de contraste en badges, estados, placeholders,
   imágenes y botones secundarios sobre cada fondo real.

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
  hashes actuales son desktop `318c2cd2…`, tablet `5ba3eb8b…` y mobile
  `6d9552bf…`.

## Resultado del bloque

La base automatizada de nombres accesibles, foco, reduced motion y responsive
queda integrada en el smoke visual. El bloqueo restante es la validación
manual con lector de pantalla y teclado completo; no se oculta como un falso
"100% WCAG".

## Actualización 2026-08-25 (MDL-32)

Linear cerró MDL-32 el 24 ago con commits que **no llegaron a git**. Hoy se
versiona la evidencia pendiente en `main`/PR:

- Skip link “Saltar al contenido” hacia `#dashboard-main`.
- Toasts dentro de una región `aria-live="polite"` (`role="alert"` si es error).
- Acciones primarias `.button:not(.small)` y pestañas de Configuración con
  mínimo 44×44 CSS px.
- Pestañas de Configuración: flechas, Home y End (ya estaba en código).

### Matriz de rutas (teclado / reflow) — implementación en código

| Ruta | Teclado en código | Zoom/reflow 320px | NVDA/VoiceOver |
|---|---|---|---|
| `/resumen` | Skip link, foco visible, `aria-live` en trabajo activo | Layout responsive existente | No ejecutado |
| `/fichas` | Botones con nombre, acciones 44px | Columna única &lt;640px | No ejecutado |
| `/checklist` | `aria-pressed` en estados | Acciones apiladas | No ejecutado |
| `/actividades` | Igual | Toolbar a una columna | No ejecutado |
| `/evidencias` | Modal con focus trap previo | Galería apilada | No ejecutado |
| `/trabajos` | Timeline y filtros | Meta apilada | No ejecutado |
| `/reportes` | Acciones primarias 44px | Igual | No ejecutado |
| `/configuracion` | `tablist` + flechas | Grid a una columna | No ejecutado |
| `/diagnostico` | Landmarks del shell | Igual | No ejecutado |

No se declara conformidad WCAG 2.1 AA: falta pasada con NVDA (Windows) y
VoiceOver (macOS). El acta está en
[`committee-minutes-2026-08-25.md`](committee-minutes-2026-08-25.md).
