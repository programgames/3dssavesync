/* Settings on the SD card: config.ini, base.ini, alias.ini, titres.ini and maj.ini (their names and keys stay French,
 * CONTRIBUTING.md), and the takeover of the old folder. Every file is read by read_ini (with its .tmp fallback) and
 * written by replace_file (files.h): a power cut never empties one.
 * No libctru dependency: tested on the PC (test/test_settings.c). */
#ifndef SETTINGS_H
#define SETTINGS_H

#include <stddef.h>
#include "key.h"

#define APP_DIR "sdmc:/3ds/3dssavesync" /* folder of the settings files */

/* "key=value" lines; a line without "=" is skipped. path missing: path.tmp (power cut between the removal of the old
 * file and the rename, replace_file: the .tmp is complete). 0 if a file was read, -1 if neither exists. */
int read_ini(const char *path, void (*entry)(void *ctx, const char *key, const char *value), void *ctx);

/* config.ini: server, token, ROM and Save folders. */
typedef struct {
    char url[256], token[256], roms[256], saves[256], roms_gb[256], saves_gb[256], roms_gba[256], saves_gba[256];
} config_t;

/* Defaults (TWiLight Menu++, open_agb_firm), then the lines of path. */
void read_config(const char *path, config_t *c);

/* 0 if written. */
int write_config(const char *path, const config_t *c);

/* base.ini: "KEY=hash", the Base of each Game. */
#define MAX_BASES 512
typedef struct {
    struct { char key[KEY_MAX], hash[65]; } e[MAX_BASES];
    int n;
} bases_t;

void read_bases(const char *path, bases_t *b);

/* Base of key, NULL if none. */
const char *base_of(const bases_t *b, const char *key);

/* Sets the Base of key, then writes path. 0 if written; -1 if path is not written (the Base is set in memory).
 * ponytail: beyond MAX_BASES Games, a new Key is not kept (0, nothing written); raise MAX_BASES with MAX_GAMES. */
int set_base(const char *path, bases_t *b, const char *key, const char *hash);

/* alias.ini (ADR 0014): "ROM name without extension=KEY", so that a ROM hack has its own Key. */
#define MAX_ALIAS 64
typedef struct {
    struct { char name[256], value[KEY_MAX]; } e[MAX_ALIAS];
    int n;
} aliases_t;

/* Spaces before "=" are dropped; the value is kept as is (alias_key checks it). */
void read_aliases(const char *path, aliases_t *a);

/* Alias of the ROM name, NULL if none. */
const char *alias_of(const aliases_t *a, const char *name);

/* maj.ini (ADR 0021): day of the last update check, last version seen on GitHub, version that ran last (read by the PC
 * before it copies a .cia). */
typedef struct {
    long day; /* -1 if never */
    char latest[32], installed[32];
} update_info_t;

void read_update_info(const char *path, update_info_t *u);

/* Writes u with running as the version that ran last. 0 if written. */
int write_update_info(const char *path, const update_info_t *u, const char *running);

typedef struct {
    unsigned long long id;
    char name[48];
} title_t;

#define MAX_TITLES 64

/* The 8 3DS Pokémon games (ADR 0009): the list when titres.ini does not exist. */
int default_titles(title_t *t, int max);

/* titres.ini: one line "0004000000055D00=Pokemon X" per game. File missing (and its .tmp): default_titles. Returns the
 * number of games. */
int read_titles(const char *path, title_t *t, int max);

/* 0 if written. */
int write_titles(const char *path, const title_t *t, int n);

/* Index of id in t, or -1. */
int title_index(const title_t *t, int n, unsigned long long id);

/* Takeover of an install under the old name: if the new folder has no config.ini, base.ini or titres.ini (nor their
 * .tmp) and old_dir/config.ini exists, copies config.ini, base.ini and titres.ini (the old folder stays intact). 1 if copied, 0 if nothing to do, -1 on failure. */
int migrate_dir(const char *old_dir, const char *new_dir);

#endif
