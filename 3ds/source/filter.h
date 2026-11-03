/* Tabs, search and order of the game list. No libctru dependency: tested on the PC (test/test_filter.c). */
#ifndef FILTER_H
#define FILTER_H

#include "games.h"

/* Tabs of the list, cycled with L and R. DS to 3DS in the order of system_t (key.h). */
typedef enum { TAB_ALL, TAB_TODO, TAB_DS, TAB_GB, TAB_GBA, TAB_3DS, TAB_COUNT } tab_t;

/* Order of the list (setting "tri" of config.ini). */
typedef enum { ORDER_TODO, ORDER_NAME, ORDER_RECENT } order_t;

/* What the list needs of a Game to filter and sort it. */
typedef struct {
    status_t status;
    int system;      /* system_t */
    const char *name;
    long long modified; /* date of the Local save, 0 if unknown */
} row_t;

/* 1 if the Game goes in the tab. To do: Upload, Download, Conflict, Duplicate (the user must rename or remove a ROM,
 * or the game is never synced) and Not synced. */
int in_tab(tab_t t, const row_t *r);

/* Next tab in the direction step (+1 R, -1 L), wrapping; a tab with no game (counts) is skipped, except All. */
tab_t next_tab(tab_t t, int step, const int *counts);

/* < 0, 0, > 0 like strcmp, case and accent insensitive ("Émeraude" next to "Emeraude"). */
int compare_names(const char *a, const char *b);

/* ORDER_TODO: status_priority first; ORDER_RECENT: the most recent Local save first, without a date at the end;
 * then by name. */
int compare_rows(order_t o, const row_t *a, const row_t *b);

/* 1 if name contains query, case and accent insensitive (UTF-8 Latin-1 letters: "pokemon" finds "Pokémon"). An empty
 * query matches everything. */
int name_matches(const char *name, const char *query);

#endif
