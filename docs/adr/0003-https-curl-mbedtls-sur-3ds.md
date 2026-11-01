# HTTPS sur la 3DS avec curl + mbedTLS, pas avec httpc

Le service système `httpc` de la 3DS s'arrête à TLS 1.1 (3dbrew, « SSL Services ») et ne connaît pas ISRG Root X1. Un reverse proxy courant (Traefik v2.11, configuration par défaut) exige TLS 1.2 avec ECDHE (test du 2026-10-31 : `handshake failure` sans ECDHE). Le homebrew utilise donc `3ds-curl` + `3ds-mbedtls` (portlibs devkitPro, présents dans l'image Docker `devkitpro/devkitarm`), avec ISRG Root X1 embarqué via `CURLOPT_CAINFO_BLOB`.

## Options écartées

- **HTTP clair** (avec `httpc`) : l'utilisateur synchronise aussi hors de chez lui ; le jeton passerait en clair sur les Wi-Fi publics, et un jeton volé permet des Envois.
- **Affaiblir le serveur** (TLS 1.0/1.1, `@SECLEVEL=0`) pour `httpc` : dégrade un point d'entrée public, et les suites acceptées par la 3DS ne sont pas documentées.

## Conséquences

- Le homebrew appelle `socInit` et `sslcInit` (mbedTLS tire son entropie de `sslcGenerateRandomData`).
- Le certificat embarqué est la racine (ISRG Root X1, valable jusqu'en 2035), pas le certificat du site : les renouvellements Let's Encrypt n'imposent pas de nouvelle version du homebrew.
