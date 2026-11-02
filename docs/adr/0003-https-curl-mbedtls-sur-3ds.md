# HTTPS on the 3DS with curl + mbedTLS, not with httpc

The 3DS system service `httpc` stops at TLS 1.1 (3dbrew, "SSL Services") and does not know ISRG Root X1. A common reverse proxy (Traefik v2.11, default configuration) requires TLS 1.2 with ECDHE (test of 2026-10-31: `handshake failure` without ECDHE). The homebrew therefore uses `3ds-curl` + `3ds-mbedtls` (devkitPro portlibs, present in the `devkitpro/devkitarm` Docker image), with ISRG Root X1 embedded through `CURLOPT_CAINFO_BLOB`.

## Rejected options

- **Plain HTTP** (with `httpc`): the user also syncs away from home; the token would travel in clear on public Wi-Fi, and a stolen token allows Uploads.
- **Weaken the server** (TLS 1.0/1.1, `@SECLEVEL=0`) for `httpc`: degrades a public entry point, and the cipher suites the 3DS accepts are not documented.

## Consequences

- The homebrew calls `socInit` and `sslcInit` (mbedTLS takes its entropy from `sslcGenerateRandomData`).
- The embedded certificate is the root (ISRG Root X1, valid until 2035), not the site certificate: Let's Encrypt renewals do not require a new homebrew version.
