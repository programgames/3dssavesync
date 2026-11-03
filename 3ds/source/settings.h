/* Réglages sur la SD : liste des jeux 3DS synchronisés (titres.ini) et reprise de l'ancien dossier.
 * Aucune dépendance libctru : testé sur PC (test/test_settings.c). */
#ifndef SETTINGS_H
#define SETTINGS_H

#include <stddef.h>

typedef struct {
    unsigned long long id;
    char name[48];
} title_t;

#define MAX_TITLES 64

/* Les 8 Pokémon 3DS (ADR 0009) : la liste quand titres.ini n'existe pas. */
int default_titles(title_t *t, int max);

/* titres.ini : une ligne "0004000000055D00=Pokemon X" par jeu. Fichier absent : titres.ini.tmp (coupure pendant
 * write_titles), puis default_titles. Nombre de jeux. */
int read_titles(const char *path, title_t *t, int max);

/* Remplacement sans perte (replace_file). 0 si écrit. */
int write_titles(const char *path, const title_t *t, int n);

/* Index de id dans t, ou -1. */
int title_index(const title_t *t, int n, unsigned long long id);

/* Reprise d'une installation sous l'ancien nom : si le nouveau dossier n'a ni config.ini, ni base.ini, ni titres.ini (ni leur
 * .tmp) et ancien/config.ini existe, copie config.ini, base.ini et titres.ini (l'ancien dossier reste intact). 1 si copié, 0 si rien à faire, -1 si échec. */
int migrate_dir(const char *old_dir, const char *new_dir);

#endif
