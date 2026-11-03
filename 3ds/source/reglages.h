/* Réglages sur la SD : liste des jeux 3DS synchronisés (titres.ini) et reprise de l'ancien dossier.
 * Aucune dépendance libctru : testé sur PC (test/test_reglages.c). */
#ifndef REGLAGES_H
#define REGLAGES_H

#include <stddef.h>

typedef struct {
    unsigned long long id;
    char nom[48];
} titre_t;

#define MAX_TITRES 64

/* Les 8 Pokémon 3DS (ADR 0009) : la liste quand titres.ini n'existe pas. */
int titres_defaut(titre_t *t, int max);

/* titres.ini : une ligne "0004000000055D00=Pokemon X" par jeu. Fichier absent : titres.ini.tmp (coupure pendant
 * titres_ecrire), puis titres_defaut. Nombre de jeux. */
int titres_lire(const char *chemin, titre_t *t, int max);

/* Remplacement sans perte (remplacer_fichier). 0 si écrit. */
int titres_ecrire(const char *chemin, const titre_t *t, int n);

/* Index de id dans t, ou -1. */
int titres_index(const titre_t *t, int n, unsigned long long id);

/* Reprise d'une installation sous l'ancien nom : si le nouveau dossier n'a ni config.ini, ni base.ini, ni titres.ini (ni leur
 * .tmp) et ancien/config.ini existe, copie config.ini, base.ini et titres.ini (l'ancien dossier reste intact). 1 si copié, 0 si rien à faire, -1 si échec. */
int migrer_dossier(const char *ancien, const char *nouveau);

#endif
