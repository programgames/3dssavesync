/* Status of a Game in the list: its State (state.h), or why it has none. No libctru dependency: tested on the PC
 * (test/test_games.c). */
#ifndef GAMES_H
#define GAMES_H

#include "state.h"

typedef enum {
    STATUS_IN_SYNC = IN_SYNC,
    STATUS_TO_UPLOAD = TO_UPLOAD,
    STATUS_TO_DOWNLOAD = TO_DOWNLOAD,
    STATUS_CONFLICT = CONFLICT,
    STATUS_DUPLICATE, /* several ROMs for one Key (ADR 0005): not synced */
    STATUS_NOT_SYNCED, /* Virtual Console archive with other files than sav.dat (vc.h): not synced */
    STATUS_OFFLINE,   /* server not read: State unknown ("Unknown"), no action offered */
    STATUS_HIDDEN,    /* neither Local save nor Current version: not shown */
} status_t;

/* What a Client sees of a Game. */
typedef struct {
    int rom_count;       /* ROMs with this Key (or this Save file) */
    const obs_t *local;  /* NULL: no Local save */
    int blank;           /* the Local save is a Blank save */
    const obs_t *server; /* NULL: no Current version, or server not read */
} observation_t;

/* A Blank save counts as absent, unless the Current version is the same Save. server_read: 0 if the last read
 * of the server failed. */
status_t game_status(const observation_t *o, const char *base, int server_read);

/* 1 if the Base must become local_hash: In sync, and the Base is missing or another Hash (design §1). */
int base_outdated(status_t s, const char *base, const char *local_hash);

/* Display order: what needs a decision first (Conflict, To download), then To upload, Duplicate and Not synced, the
 * rest. 0 to 4. */
int status_priority(status_t s);

#endif
