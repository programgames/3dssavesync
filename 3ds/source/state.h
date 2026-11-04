/* State of a Game (docs/design.md §1, ADR 0001). No libctru dependency. */
#ifndef STATE_H
#define STATE_H

typedef enum { IN_SYNC, TO_UPLOAD, TO_DOWNLOAD, CONFLICT } state_t;

/* An observed Save: lowercase hex SHA-256 Hash and size in bytes. */
typedef struct {
    const char *hash;
    unsigned long size;
} obs_t;

/* local, server: NULL = absent. base: NULL = absent.
 * Not allowed if local and server are both NULL. */
state_t sync_state(const obs_t *local, const obs_t *server, const char *base);

#endif
