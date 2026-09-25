# Evidencia de pruebas contra Zajuna real

Artefactos generados por el E2E autenticado. Se versionan porque documentan lo
que la aplicación encontró en un curso real, no una expectativa de laboratorio.

| Archivo | Origen | Contenido |
|---|---|---|
| `mdl-33-selectors.json` | `TestAuthenticatedZajunaE2E` con `ZAJUNA_SELECTOR_REPORT` | Registro de selectores y reglas de captura del curso A real (MDL-33). |
| `mdl-33-selectors-curso-b.json` | Igual, con `ZAJUNA_SELECTOR_FICHA_INDEX=1` | Mismo registro en un segundo curso real, para separar una regla frágil de una particularidad del curso. |

## Qué no contienen

El registro se construye solo desde la metadata de evidencia, que el worker de
captura ya pasa por `security.RedactURL` y `security.RedactText`, y además
descarta el título de la página, la ruta absoluta del archivo, el host del
despliegue y el id de la ficha. No hay credenciales, cookies, nombres de
instructor, códigos de ficha ni rutas locales.

Antes de versionar un artefacto nuevo hay que revisarlo: es la única barrera
entre una corrida autenticada y el repositorio.

## Cómo leerlo

- `matchedUnits` — unidades donde un selector produjo la captura.
- `fallbackChainUnits` — de esas, cuántas usaron una regla más gruesa que la
  prevista. Un número alto significa recortes imprecisos, no un fallo.
- `fullPageUnits` — no coincidió ningún selector y se capturó la página completa.
- `skippedRecords` — registros cuya metadata no se pudo decodificar. Debe ser 0;
  cualquier otro valor indica que la metadata cambió de forma.
- `captureOutcome` — cómo terminó el job, incluido el diagnóstico textual de un
  fallo parcial.

El análisis de la corrida del 2026-08-26 está en
[`../mdl-33-2026-08-26.md`](../mdl-33-2026-08-26.md).

La auditoría manual de las 68 capturas de la ficha 3135429 (versión 0.1.5) y
las correcciones del revisor automático están en
[`auditoria-evidencias-2026-09-24.md`](auditoria-evidencias-2026-09-24.md).
