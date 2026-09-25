# Zajuna App 0.1.6

Base: 0.1.5 ([release-0.1.5.md](release-0.1.5.md)), que ya incluía SolucionCache.
Integra en una sola versión diez líneas de trabajo: SolucionCache, SolucionCapt,
ReOrganizacionFront, AnalisisProyect, CalidadDocsCI, AnalisisEvidenceFixes,
ConexionPreferencias, Pendientes013, CapturaHardening y HardeningCore.

## Datos locales

Regla de producto sin cambios: **cada versión nueva empieza desde cero**. El
instalador ordena borrar los datos anteriores y el core aplica la misma regla
por versión (`backup.EnforceVersion`), lo que cubre actualizaciones
automáticas y Linux. No hay opción de conservar datos entre versiones: genera
el reporte PDF antes de actualizar si lo necesitas. Las miniaturas nuevas
(`thumbnails/`) también se borran.

## Resumen y flujo guiado (SolucionCache, ya en 0.1.5)

- **El Resumen sin ficha activa se quedaba cargando.** Recién instalada, la app
  no tiene ficha activa y `/api/checklist/dashboard` responde 404; la consulta
  volvía a «cargando» al montar cada botón y entraba en bucle (más de 900
  peticiones en 5 s). Ahora no se repite al montar (`retryOnMount: false`).
- Guía de 5 pasos (sincronizar, buscar rutas, seleccionar actividades,
  preparar evidencias, revisar) en el Resumen y en las acciones.
- Solo las actividades técnicas se pueden seleccionar; las transversales
  aparecen bloqueadas.
- Página nueva **Revisión** (`/revision`): revisión automática de cada
  evidencia (aprobada, pendiente, rechazada) y decisión manual.

## Capturas y evidencias (SolucionCapt)

- Foros: 9.1.5–9.1.7 exigen réplicas del instructor; 14.1.x exigen un debate
  de «conclusión»; 9.1.3/9.1.4 exigen fechas visibles.
- Una ausencia real de contenido (la página correcta cargó y no hay nada que
  pruebe el ítem) es un error tipado (`capture.ErrContentAbsent`) y retira la
  evidencia vieja. Una página de error o de permisos de Moodle sigue siendo un
  fallo y conserva la evidencia anterior.
- La revisión automática nunca pisa una decisión manual.
- Cronogramas: la altura se mide dentro del iframe anidado; se abre la pestaña
  de la fase.
- El menú del curso ya no se prepara con clics que Moodle guardaba como
  preferencia del usuario.
- Viewport fijo 2560×1200 por defecto, para capturas reproducibles.

## Diseño y rendimiento (ReOrganizacionFront)

- Menú lateral y paneles derechos fijos al desplazar; enlaces y acciones
  alineados en todas las páginas.
- Carga diferida por página: el paquete inicial pasa de 445 KB a ~310 KB.
- `GET /api/checklist/targets` pasa de ~1 s a ~12 ms (índice de rutas).
- `GET /api/evidences/{id}/thumbnail`: miniaturas JPEG en caché.
- Consulta adaptativa: 5 s con trabajos activos, 30 s en reposo.

## Seguridad local y trabajos (AnalisisProyect, HardeningCore)

- Sesión local por proceso: el lanzador pide un enlace de un solo uso que
  entrega una cookie por puerto y una cabecera `X-Zajuna-Capability`. Ninguna
  otra respuesta emite cookies y todo `/api/*` exige sesión salvo
  `/api/health` (mínimo) y las descargas y miniaturas (solo cookie).
- Las mutaciones exigen `Origin` loopback; se rechaza `Sec-Fetch-Site`
  same-site y cross-site.
- Un trabajo abandonado al encolarse queda cancelado, no fallido. Los fallos y
  cancelaciones guardan su resultado parcial y los mensajes se sanean.
- Dos capturas de la misma ficha, ítem o slot no corren a la vez; esperar se
  puede cancelar.

## Preferencias y conexión (ConexionPreferencias)

- Configuración › Probar conexión ejecuta una prueba real contra Zajuna.
- `capture.fullPage`, `capture.reuseSession` y `session.autoRenew` se aplican
  a cada captura (también programadas y de un solo ítem). `autoRenew` vuelve
  a iniciar sesión con una sesión nueva y reintenta una vez.

## Más reglas de captura (Pendientes013, CapturaHardening)

- 5.1 en ventanas de columnas de hasta 2560 px en vez de una imagen de
  ~28.000 px.
- Los foros que Moodle niega a la cuenta se marcan restringidos y nunca se
  eligen; la captura lo explica.
- Todo objetivo exige su selector semántico; sin `#page-content` ni perfil
  como respaldo. Las secciones solo caen a su sección padre.
- El crawler descarta páginas de otros cursos; los enlaces sin texto no
  heredan el título de la página de origen.
- Chromium solo navega al origen de Zajuna y a una lista HTTPS de Google.
- Perfil con el id del usuario autenticado; reporte HTML con imágenes.

## Evidencias (AnalisisEvidenceFixes)

- Borrar una evidencia solo borra el archivo si ninguna otra fila lo usa.
- La API expone `fileKey` en lugar de rutas absolutas.
- La galería por ficha muestra todas las evidencias de la ficha.

## Documentación y CI (CalidadDocsCI)

- Schema v14 documentado, tabla de workers y política de datos.
- CI con smoke de navegador en Linux.

## Ajustes de integración

- `/checklist` desbordaba a 125 % de escala (test WCAG); corregido.
- Líneas base visuales regeneradas y revisadas en escritorio, tablet y móvil.
- Regresión corregida: un filtro `Technical` en los grupos de foros dejaba
  sin rutas 9.x, 11.x, 14.x y 15.1 en cursos reales (el descubrimiento marca
  así todo foro sin código transversal).
- Las miniaturas de evidencias se sirven con la sesión de cookie, como las
  descargas.
- HardeningCore proponía conservar datos entre versiones; no se integró por
  la regla de producto de arriba.

## Prueba real en la ficha 3135429

La corrida completa (sincronizar, rutas, actividades, capturas y revisión) en
un curso real destapó y se corrigió:

- **Cronogramas en blanco (1.1.x, 1.2.x):** `expandEmbeddedSheets` dejaba la
  cuadrícula de Google Sheets sin pintar. Se desactiva; la hoja se captura
  completa con `prepareEmbeddedSheets`.
- **Revisor:** marca «casi en blanco» una captura alta con una franja vacía
  que ocupa más de la mitad (el cronograma sin pintar se aprobaba).
- **5.1:** cubría 2 de 146 filas del calificador; ahora 5 lotes de 30 filas.
- **10.1.x:** solo entregas «Calificado» (antes, aprendices sin entrega).
- **Secciones vacías:** una subsección con solo su título es «sin contenido en
  Zajuna» y no una evidencia (7.3.2, 7.3.3, 13.1.x, 12.1.x slot 4).
- **Revisión:** los ítems sin evidencia dicen qué falta en Zajuna.
- **15.1:** usa el foro de anuncios si no hay foro de netiqueta.
- **Pie fijo «Guardar cambios»** de Moodle ya no tapa filas.
- **Checklist automático:** cuando todas las evidencias de un ítem quedan
  aprobadas (revisión automática al terminar la captura, «Verificar de nuevo»
  o una aprobación manual), el ítem pasa a «Sí». Si luego una evidencia deja de
  estar aprobada, vuelve a «Pendiente» solo si ese «Sí» lo puso la revisión
  automática; un «Sí» o «No» manual nunca se cambia. El historial del ítem lo
  registra como «Revisión automática».
- **Guía de 5 pasos:** el paso 5 no queda hecho mientras haya ítems aprobados
  sin marcar en el checklist.
- **Evidencias:** la galería muestra el estado de Revisión (Aprobada, Por
  revisar, Rechazada) en vez de la confianza antigua de la captura.
- **Una sola instancia del core por carpeta de datos** (`.core.lock`): un
  segundo core espera hasta 15 s y luego se niega a arrancar.
- **Actualizador:** el error se registra una vez y en una línea. La última
  publicación en GitHub Releases es la v0.1.2 y no incluye `latest.yml`; para
  que la actualización automática funcione, la publicación de la 0.1.6 debe
  subir `Zajuna.App.Setup-0.1.6.exe`, su `.blockmap` y `latest.yml`.

