/* Text of a failed request for the 3DS screen: no HTTP code and no English curl text. */
#ifndef ERRORS_H
#define ERRORS_H

#include <stddef.h>

/* status: HTTP status (> 0) or -<curl code> (< 0), as net_get and net_put return. wifi: 0 if the 3DS is connected to
 * no access point. A short sentence that says what to do. */
const char *network_text(long status, int wifi, int english);

/* The same for GitHub (update check and download, ADR 0021). */
const char *github_text(long status, int wifi, int english);

/* Technical line of a failure, under the text, so the user can report it: "HTTP 401 · http://…" or "curl 7 · http://…"
 * (address cut at 50 characters). */
void network_detail(long status, const char *url, char *out, size_t n);

#endif
