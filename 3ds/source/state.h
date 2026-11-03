/* État d'un Jeu (docs/design.md §1, ADR 0001). Aucune dépendance libctru. */
#ifndef STATE_H
#define STATE_H

typedef enum { IN_SYNC, TO_UPLOAD, TO_DOWNLOAD, CONFLICT } state_t;

/* Une Sauvegarde observée : Hash SHA-256 hexadécimal minuscule et taille en octets. */
typedef struct {
    const char *hash;
    unsigned long size;
} obs_t;

/* local, serveur : NULL = absent. base : NULL = absente.
 * Interdit si local et serveur sont tous deux NULL. */
state_t sync_state(const obs_t *local, const obs_t *server, const char *base);

#endif
