/* Settings on the SD card: list of synced 3DS games (titres.ini) and takeover of the old folder.
 * No libctru dependency: tested on the PC (test/test_settings.c). */
#ifndef SETTINGS_H
#define SETTINGS_H

#include <stddef.h>

typedef struct {
    unsigned long long id;
    char name[48];
} title_t;

#define MAX_TITLES 64

/* The 8 3DS Pokémon games (ADR 0009): the list when titres.ini does not exist. */
int default_titles(title_t *t, int max);

/* titres.ini: one line "0004000000055D00=Pokemon X" per game. File missing: titres.ini.tmp (power cut during
 * write_titles), then default_titles. Returns the number of games. */
int read_titles(const char *path, title_t *t, int max);

/* Replace without loss (replace_file). 0 if written. */
int write_titles(const char *path, const title_t *t, int n);

/* Index of id in t, or -1. */
int title_index(const title_t *t, int n, unsigned long long id);

/* Takeover of an install under the old name: if the new folder has no config.ini, base.ini or titres.ini (nor their
 * .tmp) and old_dir/config.ini exists, copies config.ini, base.ini and titres.ini (the old folder stays intact). 1 if copied, 0 if nothing to do, -1 on failure. */
int migrate_dir(const char *old_dir, const char *new_dir);

#endif
