/* Pairing QR code text (pairing.h). */
#include "pairing.h"

/* Appends c if it fits; at counts every char, so at >= n means the text was cut. */
static void add(char *out, size_t n, size_t *at, char c)
{
    if (*at + 1 < n) out[*at] = c;
    (*at)++;
}

/* Like Go url.QueryEscape: A-Z a-z 0-9 - . _ ~ as they are, a space as "+" (the PC's exact text; URLDecoder reads
 * "+" and "%20" both as a space), every other byte as %XX. */
static void add_encoded(char *out, size_t n, size_t *at, const char *s)
{
    static const char hex[] = "0123456789ABCDEF";
    for (; *s; s++) {
        unsigned char c = (unsigned char)*s;
        if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~') {
            add(out, n, at, (char)c);
        } else if (c == ' ') {
            add(out, n, at, '+');
        } else {
            add(out, n, at, '%');
            add(out, n, at, hex[c >> 4]);
            add(out, n, at, hex[c & 15]);
        }
    }
}

int pairing_payload(const char *url, const char *token, char *out, size_t n)
{
    const char *s;
    size_t at = 0;
    for (s = "3dssavesync://serveur?adresse="; *s; s++) add(out, n, &at, *s);
    add_encoded(out, n, &at, url);
    for (s = "&jeton="; *s; s++) add(out, n, &at, *s);
    add_encoded(out, n, &at, token);
    if (n) out[at < n ? at : n - 1] = '\0';
    return at < n;
}
