#include <string.h>
#include "state.h"

static int equal(const char *a, const char *b)
{
    return a && b && strcmp(a, b) == 0;
}

state_t sync_state(const obs_t *local, const obs_t *server, const char *base)
{
    if (local && server && equal(local->hash, server->hash)) return IN_SYNC;
    if (!server) return TO_UPLOAD;
    if (!local) return TO_DOWNLOAD;
    if (local->size != server->size) return CONFLICT;
    if (equal(local->hash, base)) return TO_DOWNLOAD;
    if (equal(server->hash, base)) return TO_UPLOAD;
    return CONFLICT;
}
