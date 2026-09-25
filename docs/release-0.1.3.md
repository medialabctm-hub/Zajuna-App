# Zajuna App 0.1.3

Issues: [MDL-215](https://linear.app/medialab-sena/issue/MDL-215),
[MDL-216](https://linear.app/medialab-sena/issue/MDL-216),
[MDL-217](https://linear.app/medialab-sena/issue/MDL-217) ·
release: [MDL-222](https://linear.app/medialab-sena/issue/MDL-222) ·
PR [#33](https://github.com/medialabctm-hub/Zajuna-App/pull/33).

## Importante antes de instalar

**Cada versión nueva empieza desde cero.** Al instalar la 0.1.3 (o al
actualizarse), la app arranca vacía: checklist en 0 %, sin evidencias,
actividades, rutas, trabajos, avisos ni copias de seguridad de la versión
anterior. Genera antes el **reporte PDF** de las fichas que necesites y, si
quieres conservar tus datos, descarga una copia desde
**Configuración › Copias de seguridad**.

## Qué cambia

### Datos que sobrevivían a la reinstalación (MDL-215)

- **Causa:** el core guarda todo en `%LOCALAPPDATA%\ZajunaApp`
  (`$XDG_DATA_HOME/zajuna-app` en Linux), y el desinstalador no tocaba esa
  carpeta. La guía indicaba una ruta equivocada.
- **Instalador de Windows** (`build/installer.nsh`): cada instalación deja
  ordenada la limpieza, y la desinstalación manual borra los datos.
- **Core** (`backup.EnforceVersion`): si la versión instalada no coincide con
  la que creó los datos, los borra antes de abrir la base. Cubre las
  actualizaciones automáticas y el AppImage.
- **En la app:**
  - **Configuración › Almacenamiento › Restablecer la aplicación**, con copia
    previa opcional.
  - **Configuración › Acerca de** muestra la versión y la carpeta de datos.
- La versión que reporta el core ya coincide con la del paquete (antes decía
  0.1.1).

### Guía en Actividades y errores claros (MDL-216)

- **Actividades** (también dentro del Checklist):
  - pasos explicados;
  - «Seleccionar todas», «Solo técnicas», «Marcar fase», «Quitar todas» y
    «Deshacer cambios»;
  - la etiqueta «Transversal» se explica en pantalla;
  - aviso de cambios sin guardar.
- **Cada error explica qué pasó, por qué y qué hacer**, con el botón para
  resolverlo: reintentar, buscar rutas, revisar tu cuenta, elegir actividades,
  abrir el checklist o el diagnóstico.
- **«Requiere tu atención»** ya no acumula procesos viejos:
  - los avisos se pueden descartar;
  - un fallo se considera resuelto cuando el mismo proceso vuelve a terminar
    bien.

### Evidencias sin duplicados y capturas correctas (MDL-217)

- **Lotes de 2 filas** (slot 1 = filas 1–2, slot 2 = 3–4…), con el encabezado
  visible:
  - calificador (`5.1`);
  - foros y anuncios del instructor (`9.1.5`–`9.1.7`, `11.x`, `14.x`, `15.1`),
    solo con sus publicaciones;
  - tablas de calificación de `10.1.x`.
- **`10.1.1` / `10.1.2`** capturan la tabla de calificación y retroalimentación
  de cada actividad, ya no la tarjeta de fechas de `6.1`.
- **Ítems 7, 8 y 13** capturan su propia subsección («Reporte del Curso»,
  «Seguimiento a la Formación», «Comités evaluativos», «Documentos de
  retención», «Sesiones en línea»), expandida, y no el banner ANUNCIOS.
- **`12.1.x`** usa las secciones «Grabaciones sesiones en línea» de cada fase.
- **Imágenes sin repetir:** una imagen se guarda y se muestra una sola vez
  aunque respalde varios ítems, también en el PDF.
- **Sin inicios de sesión de más:** la captura reutiliza las sesiones de
  Chromium, antes abría una por cada evidencia.
- **Sin avisos de Moodle en las capturas**, como «No dispone de permiso…».

### Guía de 5 pasos, selección de actividades y revisión automática

- **Guía paso a paso siempre visible** en las páginas de operación:
  1. Sincronizar fichas;
  2. Buscar rutas;
  3. Seleccionar actividades;
  4. Preparar evidencias;
  5. Revisar evidencias.

  Cada paso muestra si está hecho, en curso o pendiente. El siguiente se
  resalta, y sus botones llevan la insignia «Paso N». «Preparar evidencias» se
  bloquea mientras falte un paso anterior y enlaza al que falta. Ya no se
  puede sincronizar ni capturar dos veces seguidas.
- **Solo se pueden seleccionar actividades técnicas.**
  - Las transversales se muestran aparte y bloqueadas: las orienta otro
    instructor y solo generaban evidencia errónea.
  - El core rechaza las transversales e ignora las que hubiera guardado una
    versión anterior.
  - Cada actividad marcada indica si «se usa en la evidencia» (las 5 primeras
    en orden de fase).
- **Página Revisión (paso 5).** Al terminar cada captura, la app revisa
  automáticamente cada evidencia:
  - imagen inexistente o página de inicio de sesión;
  - demasiado ancha o larga;
  - sección vacía;
  - casi en blanco;
  - área genérica en lugar de la sección esperada;
  - copia de otro ítem.

  Las correctas quedan **aprobadas** y en pendientes quedan **solo las que
  tienen problemas**, cada una con el motivo en palabras claras y los botones
  Aprobar, Rechazar, Ver en grande y Volver a capturar este ítem. La decisión
  manual manda, y se aplica a todos los ítems que comparten la misma imagen,
  hasta que la evidencia se vuelva a capturar.
- **Anuncios `11.x` filtrados por tema:** inicio de fase, inicio o cierre de
  actividad, invitación a sesión en línea o aprendices aprobados. `11.2.3` ya
  tiene evidencia.
- **Subsecciones propias para 7.3.x y 7.4.x:** «Comités evaluativos»,
  «Reuniones EEF», «Planes de Mejoramiento», «Registro de Novedades» y
  «Llamados de atención».

## Prueba real contra Zajuna (curso 41080, 71 actividades técnicas)

| Corrida | Guardadas | Omitidas (lote vacío) | Errores | Nota |
|---|---|---|---|---|
| 1 | 42 | 4 | 22 | Casi todos los errores: tiempo de espera al iniciar sesión (una sesión por captura). |
| 2 | 57 | 6 | 11 | Con el pool de sesiones. Subsecciones invisibles dentro de secciones colapsadas. |
| 3 | 61 | 6 | 7 | Con expansión de secciones contenedoras. Los 7 errores son ausencias reales (foro sin publicaciones del instructor en `9.1.6`/`14.1.1`, actividad sin tabla de calificación en `10.1.1`) o MDL-219 (`11.4`). |
| 4 (dirigida) | 5 de 5 ítems | 0 | 0 | `7.1.1`, `7.3.1`, `8.1`, `12.1.1` y `13.1.2` tras ajustar la expansión: cada ítem muestra su sección (p. ej. `7.1.1` con la insignia «Ocultado a los aprendices») con tamaños normales. |
| 5 (final) | 68 | 30 | 3 | Con la guía, la revisión, el filtro de anuncios por tema, las subsecciones propias, la preparación sin navegación accidental y sin el foro de noticias del sitio. Los 62 ítems tienen evidencia. Los 3 errores son ausencias reales: 2 foros sin publicaciones del instructor y 1 actividad sin tabla de calificación. La revisión automática aprobó 52 ítems y dejó 10 pendientes, cada uno con su motivo. |

Antes de estos cambios: 105 evidencias con solo 35 imágenes distintas y 2 ítems
sin resolver. Después: los 62 ítems resueltos, 133 evidencias con 56 imágenes
distintas. Las repeticiones que quedan son intencionales: una misma captura
respalda varios ítems (p. ej. la tabla de calificación para `10.1.1` y
`10.1.2`) y se muestra una sola vez en la app y en el PDF.

## Pendiente

- [MDL-218](https://linear.app/medialab-sena/issue/MDL-218): el calificador
  `5.1` mide ~28.000 px de ancho.
- [MDL-220](https://linear.app/medialab-sena/issue/MDL-220): el iframe de
  Google Sheets de `1.2.x` solo muestra el área visible.
- [MDL-221](https://linear.app/medialab-sena/issue/MDL-221): mejorar el reparto
  de slots entre listas.
- [MDL-222](https://linear.app/medialab-sena/issue/MDL-222): AppImage de Linux
  (requiere runner Linux) y prueba instalar/actualizar/desinstalar del NSIS.
- [MDL-29](https://linear.app/medialab-sena/issue/MDL-29): firma Authenticode.
