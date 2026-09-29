# Firma y smoke de instaladores

Zajuna Sync distribuye instaladores únicamente para Windows y Linux. macOS no es
una plataforma soportada.

## Windows

El job nativo recibe `CSC_LINK` y `CSC_KEY_PASSWORD` exclusivamente desde los
secretos de GitHub Actions. Cuando ambos estén configurados, electron-builder
firma el instalador NSIS y el workflow comprueba que Authenticode sea `Valid`.
No se debe copiar el certificado, su contraseña ni valores de secretos en
issues, logs o artefactos.

Sin `CSC_LINK` el job puede construir y ejecutar el smoke, pero el instalador
no debe considerarse publicable: falta la firma Authenticode.

## Linux

El job genera el AppImage en `ubuntu-latest`, ejecuta el smoke contra la
aplicación empaquetada y publica el manifiesto de release con SHA-256 junto al
SBOM CycloneDX. El checksum publicado es el mecanismo de integridad del
artefacto Linux.

## Evidencia de runner nativo

El workflow manual `Native installers` ejecuta el empaquetado y smoke en
`windows-latest` y `ubuntu-latest`. Conserva como artefactos el instalador,
`release-manifest.json` y `sbom.cyclonedx.json`; esos son los insumos que se
deben adjuntar al gate de release. No se declara un release aprobado si falta
el artefacto, el smoke o, en Windows, la firma válida.


## Placeholders CI (sin secretos en el repositorio)

El workflow `native-installers` / `packaging` espera estos secretos de GitHub Actions
cuando exista un certificado real. **No** se deben commitear archivos `.pfx` ni
contraseñas.

| Secreto | Uso |
|---|---|
| `CSC_LINK` | Ruta o base64 del certificado Authenticode (electron-builder) |
| `CSC_KEY_PASSWORD` | Contraseña del certificado |

Si faltan, el job **sigue** empaquetando NSIS/AppImage y sube artefactos, pero el
paso «Verify Windows Authenticode» se omite (`HAS_CSC_LINK != true`). El instalador
sin firma no debe publicarse a canal amplio (SmartScreen).

No se generan firmas falsas ni se simula Authenticode en CI.

## Auto-update y firma

`electron-updater` usa el proveedor GitHub Releases del mismo repositorio. Sin
`CSC_LINK` / Authenticode, Windows puede bloquear la descarga o la aplicación del
update (SmartScreen). Eso no es un fallo del pipeline: la firma sigue siendo
opcional; documenta el bloqueo y publica el instalador firmado cuando exista
certificado (MDL-29).

