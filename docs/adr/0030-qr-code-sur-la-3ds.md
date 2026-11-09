# Pairing QR code on the 3DS

The Android app connects by scanning a QR code (ADR 0026): `3dssavesync://serveur?adresse=<url>&jeton=<token>`, made by the PC settings page (`client/setup_web.go`, `phonePayload`). A user with a Docker server and no PC settings page at hand has no QR code. The 3DS already holds the address and the token.

## Decision

- **The 3DS draws the same QR code.** Settings > **Phone** > **A** shows it on the top screen (the larger one) until **B**. It is enabled only when **Server** and **Token** are both set.
- **The exact text of the PC.** `pairing.c` encodes like Go `url.QueryEscape`: `A-Z a-z 0-9 - . _ ~` kept, a space as `+`, every other byte as `%XX`. The host test compares it byte for byte with the string of `TestPhonePayload` (`client/setup_web_test.go`). Error correction M, like the PC.
- **Vendored encoder.** Nayuki's QR Code generator for C, `qrcodegen.c` and `qrcodegen.h` from https://github.com/nayuki/QR-Code-generator, tag v1.8.0, folder `c/` (MIT), unchanged in `3ds/source/` (the Makefile builds only that folder).
- **Token shown only on request.** The QR code buffers are cleared when it closes, as the PC hides it until asked.

## Rejected options

- **Space as `%20`**: `URLDecoder` reads it the same, but the test could then not compare with the PC's exact string.
- **Another QR library or our own encoder**: Nayuki's is small, has no dependency, and needs no heap.

## Consequences

- One rectangle per run of black modules: a long address and token (up to 255 bytes each) give a large version that may exceed the 4096 citro2d objects per frame, or be hard to scan at 1 pixel per module. A usual address and a 32-character token give version 6 to 8, at 4 pixels per module.
