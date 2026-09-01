# Firma y smoke de instaladores

Zajuna App distribuye instaladores únicamente para Windows y Linux. macOS no se
empaqueta ni se publica mientras no exista una identidad Developer ID.

## Windows

El job nativo recibe `CSC_LINK` y `CSC_KEY_PASSWORD` exclusivamente desde los
secretos de GitHub Actions. Cuando ambos estén configurados, electron-builder
firma el instalador NSIS y el workflow comprueba que Authenticode sea `Valid`.
No se debe copiar el certificado, su contraseña ni valores de secretos en
issues, logs o artefactos.

Sin `CSC_LINK` el job puede construir y ejecutar el smoke, pero el instalador
no debe considerarse publicable: falta la firma Authenticode.

## Linux

La plataforma Linux objetivo es **Linux Mint 22.3 "Zena"**, construida sobre
Ubuntu 24.04.3 LTS. GitHub no ofrece runners de Mint, así que el job compila en
`ubuntu-24.04`: es la base de Mint 22.3, de modo que el AppImage queda contra la
misma línea base de glibc y librerías del sistema que el objetivo. El runner se
fija explícitamente en vez de `ubuntu-latest` para que un salto futuro de esa
etiqueta no desalinee el artefacto respecto a Mint.

El job genera el AppImage, ejecuta el smoke contra la aplicación empaquetada y
publica el manifiesto de release con SHA-256 junto al SBOM CycloneDX. El
checksum publicado es el mecanismo de integridad del artefacto Linux.

El AppImage no lleva firma: la integridad se verifica con el SHA-256 del
manifiesto.

## Evidencia de runner nativo

El workflow manual `Native installers` ejecuta el empaquetado y smoke en
`windows-latest` y `ubuntu-24.04`. Conserva como artefactos el instalador,
`release-manifest.json` y `sbom.cyclonedx.json`; esos son los insumos que se
deben adjuntar al gate de release. No se declara un release aprobado si falta
el artefacto, el smoke o, en Windows, la firma válida.
