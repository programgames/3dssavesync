#include <string.h>
#include "games.h"

status_t game_status(const observation_t *o, const char *base, int server_read)
{
    /* Blank save = absent, unless the server has the same; download_save() copies it anyway. */
    const obs_t *local = o->local && (!o->blank || (o->server && !strcmp(o->local->hash, o->server->hash))) ? o->local : NULL;
    if (o->rom_count > 1) return STATUS_DUPLICATE;
    if (!local && !o->server) return STATUS_HIDDEN;
    if (!server_read) return STATUS_OFFLINE;
    return (status_t)sync_state(local, o->server, base);
}

int base_outdated(status_t s, const char *base, const char *local_hash)
{
    return s == STATUS_IN_SYNC && (!base || strcmp(base, local_hash));
}

int status_priority(status_t s)
{
    switch (s) {
    case STATUS_CONFLICT: return 0;
    case STATUS_TO_DOWNLOAD: return 1;
    case STATUS_TO_UPLOAD: return 2;
    case STATUS_DUPLICATE:
    case STATUS_NOT_SYNCED: return 3;
    default: return 4;
    }
}
