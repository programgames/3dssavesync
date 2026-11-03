/* État d'un Jeu (docs/design.md §1, ADR 0001). Aucune dépendance libctru. */
#ifndef ETAT_H
#define ETAT_H

typedef enum { A_JOUR, A_ENVOYER, A_RECUPERER, CONFLIT } etat_t;

/* Une Sauvegarde observée : Hash SHA-256 hexadécimal minuscule et taille en octets. */
typedef struct {
    const char *hash;
    unsigned long taille;
} obs_t;

/* local, serveur : NULL = absent. base : NULL = absente.
 * Interdit si local et serveur sont tous deux NULL. */
etat_t etat(const obs_t *local, const obs_t *serveur, const char *base);

#endif
