# Zajuna App 0.1.4

Issue: [MDL-224](https://linear.app/medialab-sena/issue/MDL-224) ·
PR [#33](https://github.com/medialabctm-hub/Zajuna-App/pull/33).
Base: 0.1.3 ([release-0.1.3.md](release-0.1.3.md)).

## Cómo se probó

Se instaló la 0.1.3 en un equipo real y se hizo el flujo completo en la ficha
3135429 (curso 41080):

1. sincronizar fichas;
2. buscar rutas;
3. seleccionar las 71 actividades técnicas desde la interfaz;
4. preparar evidencias;
5. revisar.

Tres agentes revisaron cada imagen contra lo que pide su ítem y un cuarto
recorrió la interfaz como lo haría un instructor. Los cambios de esta versión
salen de esas pruebas y se validaron otra vez en vivo contra Zajuna.

**Lo que ya funcionaba bien en la 0.1.3:**
- la instalación limpia (el instalador dejó la orden de borrado y el primer
  arranque abrió vacío);
- la guía de 5 pasos;
- la selección solo de actividades técnicas;
- los 62 ítems con evidencia y los anuncios filtrados por tema.

## Qué cambia

### Capturas
- **Cronograma (1.1.x y 1.2.x):**
  - Antes, la hoja de Google incrustada salía en blanco o con solo 2 filas, y
    la página «Fase – Hacer» mostraba la pestaña de la Fase 1.
  - Ahora se agranda el iframe, se espera a que la hoja cargue y se abre la
    pestaña correcta según la página (FASE 2 HACER, CRONOGRAMA GENERAL…). Se
    captura solo el área de contenido, sin menú ni pie.
- **5.1 (actividades en el libro de calificaciones):**
  - Antes se capturaba el calificador, de ~28.000 px de ancho y en blanco a
    partir de la columna 7.
  - Ahora se captura la **configuración del libro de calificaciones**:
    categorías y actividades en vertical, legible y en lotes de 2 filas.
- **Secciones del curso (7.x, 8.x, 12.1.x, 13.x):**
  - Se abre también el primer nivel de subsecciones, de modo que se ven las
    actas, las grabaciones, los meses y las bitácoras, no solo los títulos.
  - El nombre de la sección se compara desde el inicio: «Seguimiento y
    Evaluación» ya no coincide con «Planeación, Seguimiento y Evaluación».
    Para 7.1.x solo cuenta la sección principal.
- **Barra superior fija y widget flotante de Zajuna:** se neutralizan antes de
  cada captura. Antes la barra aparecía cosida a mitad de la imagen y el widget
  tapaba el borde derecho.
- **Fases de las actividades:** las actividades de subsecciones anidadas ya
  muestran su fase. Antes todas salían en «Sin fase identificada».

### Revisión automática
- **Regla nueva «sección sin contenido».** Una sección que no muestra
  actividades, archivos ni subsecciones queda pendiente con su motivo. Antes se
  aprobaban secciones colapsadas que solo mostraban el título.
- **Sin contenido en Zajuna.** Un foro sin publicaciones del instructor o una
  actividad sin tabla de calificación ya no hacen que toda la captura cuente
  como fallida: se informan como «sin contenido en Zajuna».

### Experiencia de uso
- **Revisión:** botón «Marcar N como cumplidos». Marca en el checklist los
  ítems que tienen toda su evidencia aprobada, así el % de cumplimiento refleja
  la revisión.
- **Resumen:**
  - distingue «ítems marcados como cumplidos» de «evidencia aprobada»;
  - el texto de la tarjeta dice el paso siguiente real, en lugar de «Todo
    listo»;
  - «Revisar evidencias» pasa a ser el botón principal cuando toca;
  - la barra ya no pinta segmentos con valor 0.
- **Guía:**
  - no parpadea mientras carga;
  - al terminar el paso 5 lleva a «generar el reporte PDF»;
  - «Buscar rutas» ya no se ve como acción principal cuando está hecho;
  - «Preparar evidencias» dice «Comprobando los pasos…» mientras carga, en
    lugar de verse activo sin responder.
- **Trabajos:** un trabajo que terminó con fallos muestra la barra en ámbar,
  no verde al 100 %.
- **Diagnóstico:** guía actualizada a los 5 pasos y mensajes en palabras claras
  en lugar de códigos (`capture_partial_failure`).

## Pendiente

- Foros 9.1.5–9.1.7 y 14.x: capturar la discusión con la pregunta del aprendiz
  y la respuesta o conclusión del instructor, no solo la lista.
- 11.1.2–11.1.4, 11.4 y 15.1: capturar el cuerpo del anuncio, no solo su
  título.
- 10.1.2: comparar la fecha de entrega con la de calificación (≤ 3 días
  hábiles).
- Instalador de Linux (MDL-222) y firma Authenticode (MDL-29).

## Estado al cerrar la sesión (2026-09-23)

**Hecho:**
- Código de la 0.1.4 terminado. Verificaciones en verde:
  - frontend: build, lint y vitest 38/38;
  - core: `go vet` y `go test ./...`.
- Instalador generado: `dist/Zajuna.App.Setup-0.1.4.exe` (sin firmar, SHA256
  `668ab8bc26430ba0f8772e12370adea8baa1ec40cf2306de399376137e66567f`).
- **Instalado en el equipo del usuario encima de la 0.1.3.** El borrado por
  versión funcionó: arrancó vacía y `/api/health` reporta `0.1.4`. Se
  restauró `config.json` con el usuario 3482483 / CC; la contraseña sigue en el
  Administrador de credenciales de Windows.
- Cada corrección se validó en vivo contra Zajuna con capturas dirigidas en una
  instancia aislada de desarrollo (cronograma, 5.1, secciones 7.x, 8.x,
  12.1.x y 13.x, `7.1.x`).
- Revisión independiente del diff; se corrigieron sus hallazgos (variante del
  título con enlace, barra fija en lotes, «Marcar como cumplidos» sin tocar
  «No»).

**Quedó pendiente:**
- **La corrida completa final en la 0.1.4 instalada no llegó a ejecutarse**:
  el script automático se interrumpió y la cola de trabajos está vacía.
  Siguiente paso: en la app 0.1.4 ya instalada, hacer los 5 pasos en la ficha
  3135429 y revisar el resultado en «Revisión».
  - Valores de referencia de la 0.1.3: 68 guardadas, 30 omitidas y
    3 ausencias reales; 52 de 62 ítems aprobados.
  - Esperado con la 0.1.4: cronogramas con la pestaña correcta, 5.1 legible,
    secciones con contenido y las 3 ausencias como «sin contenido en Zajuna»,
    no como fallo.
- Sin validar en la app instalada, aunque sí en desarrollo: toda la tanda
  0.1.4 salvo la instalación y el arranque.
- Mejoras identificadas y no implementadas: ver «Pendiente» arriba, junto
  con MDL-221, MDL-222 y MDL-29.
