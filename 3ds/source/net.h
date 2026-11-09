/* HTTPS calls to the Server with curl + mbedTLS (ADR 0003, docs/design.md §4). */
#ifndef NET_H
#define NET_H

#include <stddef.h>

/* socInit + sslcInit (mbedTLS entropy) + curl. 0 if all went well. */
int net_init(void);
void net_exit(void);

/* GET url. Returns the HTTP status (> 0) or -<curl code> (< 0).
 * If status > 0: *body is allocated, ends with '\0', to free with free(). */
long net_get(const char *url, const char *token, char **body, size_t *size);

/* GET of a GitHub address (updates, ADR 0021): no token, GitHub roots on top of ISRG. Returns and allocates
 * like net_get. on_progress != NULL: a release file, redirects followed, NET_MAX_FILE bytes at most (else
 * -63, CURLE_FILESIZE_EXCEEDED), on_progress(received, total) called during the transfer. */
#define NET_MAX_FILE (8 * 1024 * 1024)
long net_github(const char *url, void (*on_progress)(size_t done, size_t total), char **body, size_t *size);

/* PUT url with a binary body. client: X-Client, the device the History shows ("3ds", or "restauration" for a
 * Restore, ADR 0016). condition = "If-Match: \"<hash>\"", "If-None-Match: *" or NULL (name of a game).
 * Returns the HTTP status (> 0) or -<curl code> (< 0). */
long net_put(const char *url, const char *token, const char *client, const char *condition, const void *bytes, size_t size);

/* Server requests trust ISRG Root X1 and the PEM certificates of path too (ADR 0029). Returns read_cas (files.h):
 * 0 added; 1 none; -1 unreadable, -2 larger than NET_MAX_CAS: ISRG only, as before. */
#define NET_MAX_CAS (256 * 1024)
int net_read_cas(const char *path);

/* f() is called once in a Server request (net_get, net_put with a token) that has run for more than 5 s. */
void net_on_slow(void (*f)(void));

/* SHA-256 of bytes in lowercase hex: the Hash of a Save, the check of a download. */
void hash_hex(const unsigned char *bytes, size_t size, char out[65]);

/* 0 if the 3DS is connected to no Wi-Fi access point (network_text, errors.h). */
int net_wifi(void);

#endif
