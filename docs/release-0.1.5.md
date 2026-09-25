# Zajuna App 0.1.5

Issue: [MDL-224](https://linear.app/medialab-sena/issue/MDL-224) ·
PR [#33](https://github.com/medialabctm-hub/Zajuna-App/pull/33).
Base: 0.1.4 ([release-0.1.4.md](release-0.1.4.md)).

## Qué cambia

- **Resumen sin ficha activa se quedaba cargando.** Recién instalada, la app
  no tiene ficha activa y `/api/checklist/dashboard` responde 404. El Resumen
  mostraba el botón «Sincronizar fichas», que al montarse volvía a pedir el
  dashboard. La consulta volvía a «cargando», el botón se desmontaba, llegaba
  otro 404 y así en bucle: más de 900 peticiones en 5 segundos y la página
  nunca terminaba de cargar.
  - Ahora esa consulta no se repite al montar un componente
    (`retryOnMount: false`). Se vuelve a pedir al elegir una ficha o cuando
    termina un trabajo.
  - Se revisaron las demás páginas: ninguna repite peticiones.
- **Tarjeta «Primeros pasos» del Resumen:** ahora muestra los mismos 5 pasos
  que la barra de arriba; antes mostraba 3.

## Instalador

`dist/Zajuna.App.Setup-0.1.5.exe` (sin firmar, SHA256 `209a0e1bffe2bf87470abefbb5436d7bdc2f3e0365991c32fc5d384234dc1463`).

Se instaló encima de la 0.1.4 en el equipo del usuario: borró los datos,
arrancó limpia y `/api/health` reporta `0.1.5`.

Sigue pendiente la corrida completa de los 5 pasos en la ficha 3135429 (ver
release-0.1.4.md).
