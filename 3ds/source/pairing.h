/* Text of the pairing QR code that the Android app scans (core/Pairing.kt, parsePairing), the same as the PC
 * (client/setup_web.go, phonePayload). No libctru: tested on the host (test/test_pairing.c). */
#ifndef PAIRING_H
#define PAIRING_H

#include <stddef.h>

/* "3dssavesync://serveur?adresse=<url>&jeton=<token>", both values URL-encoded. 0 if it does not fit in n. */
int pairing_payload(const char *url, const char *token, char *out, size_t n);

#endif
