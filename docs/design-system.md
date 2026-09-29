# Sistema visual de Zajuna Sync

La interfaz usable vive en el frontend React y se sirve desde el mismo origen
local que la API del core Go. El build de Vite se sincroniza en
`core/cmd/zajuna-core/web/` antes de compilar el binario. Este documento fija
los tokens y estados que deben conservarse en cada vista y en el instalador.

## Dirección visual

La maqueta Mockups Zajuna SENA (offline) es la referencia de composición:
lateral azul institucional, cabecera con ruta y búsqueda, fondo gris claro,
tarjetas blancas, métricas compactas, verde SENA y jerarquía con Plus Jakarta
Sans e Inter. La aplicación usa lenguaje humano; los nombres internos de
workers y rutas solo aparecen en ayuda y diagnóstico.

## Tokens

| Categoría | Regla |
|---|---|
| Fondo | #F3F6F7, gris azulado claro para separar el contenido. |
| Superficie | #FFFFFF, borde #E1E8EC y radios de 12–14 px. |
| Azul institucional | #00324D, lateral, navegación y acciones secundarias. |
| Verde SENA | #39A900, acento, progreso y estados positivos. |
| Verde de acción | #2A7A00, botones primarios y texto de acción. |
| Información | #E7EFF4, referencias y procesos en curso. |
| Advertencia | #9A5C00 sobre #FFF3E0 para revisión pendiente. |
| Error | #C0392B sobre #FBE9E7 para fallos y recuperación. |
| Tipografía | Plus Jakarta Sans para títulos; Inter para interfaz; IBM Plex Mono para detalles técnicos. |

Las tres familias se empaquetan desde `@fontsource/*` en el build de React
(subconjunto latin-ext), por lo que el binario embebido conserva la jerarquía
tipográfica del mockup sin depender de Google Fonts ni de conexión externa.
| Espaciado | Escala base de 8 px; controles de 40–44 px. |
| Movimiento | Transiciones de 150–250 ms; reduced motion elimina lo no esencial. |

## Sistema de movimiento

Portado desde la maqueta y verificado en `core/cmd/zajuna-core/web/index.html`. Ninguna de estas animaciones gira (sin ruedas de carga); todas respetan `prefers-reduced-motion`.

| Animación | Uso | Comportamiento |
|---|---|---|
| `zjSweep` | Shimmer de skeleton mientras carga el checklist. | Loop mientras el estado de carga persista. |
| `zjStripes` | Rayas verdes en barras de progreso (`.progress.running`). | Loop mientras el trabajo esté `running`; nunca en un valor fijo. |
| `zjGrowIn` | Barra que crece de 0 a su valor. | Disparo único al primer render del Resumen (`state.firstPaint`), nunca en cada sondeo de 5 s. |
| `zjLive` / `zjLiveAlert` | Punto verde "en vivo" (`.live-dot`, `.live-pulse`) y punto rojo de alerta (`#notif-alert-dot`). | Halo pulsante mientras exista un trabajo `running` (verde) o `waiting_user` (rojo); nunca por un contador de no-leídos genérico. |
| `zjRiseIn` | Filas que entran (`.rise-in`) en "Últimas acciones". | Disparo único al primer render, con 55 ms de retardo por fila. |
| `zjToast` | Aviso emergente (`.toast`). | Entra/sale una sola vez, sincronizado con los 4.2 s reales que el aviso permanece en pantalla. |
| `zjSoft` | Respiración de opacidad en estados de espera (`.status-chip.waiting-pulse`). | Loop mientras el trabajo esté `waiting_user` o `retrying`. |

Regla de diseño: en la maqueta estática varias de estas animaciones usan `infinite`/`infinite alternate` solo para lucir en una captura fija. En la app real, como el estado se vuelve a dibujar por completo cada 5 s (sondeo), cualquier animación de entrada debe dispararse una sola vez — nunca en bucle — o se vería "resetear" cada 5 segundos.

## Estructura compartida

- Lateral: Operación contiene Resumen, Fichas, Checklist, Evidencias,
  Trabajos y Reportes; Sistema contiene Configuración y Diagnóstico.
- Cabecera: ruta actual, búsqueda, avisos y avatar.
- Contenido: título, descripción, acciones contextuales y tarjetas con radios
  de 12 px.
- Estados: texto y color; nunca comunicar estado solo por color.

## Regla de composición del Resumen

Resumen es una vista de orientación con acciones rápidas y contexto suficiente
para decidir el siguiente paso. Presenta ficha activa, métricas compactas,
checklist segmentado, actividad reciente y accesos directos a sincronizar,
abrir checklist, preparar evidencias y generar el reporte.

La selección de actividades, revisión de rutas, edición de evidencias,
respaldos y configuración detallada permanecen en sus secciones propias. Así
se conserva la maqueta sin convertir el Resumen en un panel saturado.

## Componentes y estados

- button: primario, secundario, fantasma y peligro.
- badge: estado breve con texto.
- confidence: confirmada, por revisar, agregada por ti y sin evidencia.
- progress: porcentaje textual y riel visual.
- card: métricas, ficha, trabajos, evidencias y reportes.
- skeleton: conserva alturas y columnas finales durante la carga.
- toast: confirmación breve; errores usan role alert.

## Lenguaje de interfaz

La interfaz principal usa Sincronizar fichas, Abrir checklist, Preparar
evidencias, Generar PDF, Guardar copia y Revisar rutas. Los identificadores
sync-fichas, discover-course-maps y capture-checklist solo aparecen en
diagnóstico.

## Accesibilidad aplicada

- Labels explícitos y foco visible con focus-visible.
- Landmarks main, nav y headings jerárquicos.
- Regiones aria-live para actualizaciones.
- Valores de API escapados antes de renderizar.
- Layout responsive, zoom y prefers-reduced-motion.
- Contraste semántico en texto, botones y estados.

## Integración React y empaquetado

El frontend final se construye en React 19 + TypeScript + Vite. Los tokens y
componentes de este documento viven en `frontend/src/styles/global.css` y en
los componentes compartidos de `frontend/src/components/`; no se mantiene una
segunda implementación visual en Angular o en un HTML alternativo.

El pipeline de entrega ejecuta `frontend:build`, sincroniza el resultado con
`core/cmd/zajuna-core/web/` y compila el core Go con `//go:embed`. El servidor
aplica fallback de SPA para las rutas de React, pero nunca para `/api/*` ni
para recursos con extensión que no existan. Las animaciones deben seguir
siendo clases/hooks condicionados por estado y respetar `prefers-reduced-motion`.
