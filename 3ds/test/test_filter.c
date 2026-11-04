/* Checks the tabs, the search and the order of the game list.
 * Built on the PC: gcc -std=c99 -I source source/state.c source/games.c source/filter.c test/test_filter.c -o /tmp/i && /tmp/i */
#include <stdio.h>
#include "filter.h"

static int failures;

static void check(int ok, const char *what)
{
    if (!ok) {
        printf("ECHEC  %s\n", what);
        failures++;
    }
}

int main(void)
{
    /* system: 0 DS, 1 GB, 2 GBA, 3 3DS (key.h) */
    const row_t up = { STATUS_TO_UPLOAD, 2, "Pok\xC3\xA9mon \xC3\x89meraude", 300 }, down = { STATUS_TO_DOWNLOAD, 0, "Mario Kart DS", 100 },
                conflict = { STATUS_CONFLICT, 1, "Tetris", 0 }, dup = { STATUS_DUPLICATE, 0, "Tetris DS", 0 },
                sync = { STATUS_IN_SYNC, 3, "pokemon X", 200 }, offline = { STATUS_OFFLINE, 0, "Zelda", 0 },
                vc = { STATUS_NOT_SYNCED, 1, "Pokemon Red VC", 0 };
    int none[TAB_COUNT] = { 0 }, some[TAB_COUNT] = { 5, 0, 3, 0, 2, 0 };

    /* tabs */
    check(in_tab(TAB_ALL, &sync) && in_tab(TAB_ALL, &offline), "All: every game");
    check(in_tab(TAB_TODO, &up) && in_tab(TAB_TODO, &down) && in_tab(TAB_TODO, &conflict) && in_tab(TAB_TODO, &dup) && in_tab(TAB_TODO, &vc),
          "To do: upload, download, conflict, duplicate, not synced");
    check(!in_tab(TAB_TODO, &sync) && !in_tab(TAB_TODO, &offline), "To do: not in sync, not unknown");
    check(in_tab(TAB_DS, &down) && !in_tab(TAB_DS, &up) && in_tab(TAB_GB, &conflict) && in_tab(TAB_GBA, &up) && in_tab(TAB_3DS, &sync) &&
              !in_tab(TAB_3DS, &up), "system tabs");
    check(next_tab(TAB_ALL, 1, some) == TAB_DS && next_tab(TAB_DS, 1, some) == TAB_GBA && next_tab(TAB_GBA, 1, some) == TAB_ALL,
          "R: empty tabs skipped, wraps to All");
    check(next_tab(TAB_ALL, -1, some) == TAB_GBA && next_tab(TAB_DS, -1, some) == TAB_ALL, "L: backwards");
    check(next_tab(TAB_ALL, 1, none) == TAB_ALL && next_tab(TAB_3DS, 1, none) == TAB_ALL, "no game: All only");

    /* search */
    check(name_matches("Pok\xC3\xA9mon \xC3\x89meraude", "pokemon"), "pokemon finds Pokémon");
    check(name_matches("Pok\xC3\xA9mon \xC3\x89meraude", "EMER") && name_matches("Pokemon Emeraude", "\xC3\xA9mer"), "accents and case both ways");
    check(name_matches("Fran\xC3\xA7" "ais \xC3\x80 \xC3\xB4 \xC3\xBC \xC3\x8F", "francais a o u i"), "ç À ô ü Ï");
    check(name_matches("Zelda", "") && name_matches("", ""), "empty query: everything");
    check(!name_matches("Zelda", "zeldaa") && !name_matches("", "a") && !name_matches("Mario", "ario kart"), "no match");
    check(name_matches("Mario Kart DS", "kart ds") && name_matches("aab", "ab"), "match in the middle, after a false start");
    check(!name_matches("\xC3\x97", "x") && name_matches("a\xC3\x97" "b", "\xC3\x97"), "× is not folded");

    /* order */
    check(compare_names("\xC3\x89meraude", "emeraude") == 0 && compare_names("\xC3\x89meraude", "Feu") < 0 && compare_names("abc", "ab") > 0,
          "names: accents ignored, shorter first");
    check(compare_rows(ORDER_TODO, &conflict, &down) < 0 && compare_rows(ORDER_TODO, &down, &up) < 0 && compare_rows(ORDER_TODO, &dup, &sync) < 0,
          "to do first: conflict, download, upload, duplicate, the rest");
    check(compare_rows(ORDER_TODO, &sync, &offline) < 0, "to do first: same priority, by name");
    check(compare_rows(ORDER_NAME, &down, &up) < 0 && compare_rows(ORDER_NAME, &up, &sync) < 0 && compare_rows(ORDER_NAME, &sync, &conflict) < 0,
          "by name: Mario, Pokémon, pokemon X, Tetris");
    check(compare_rows(ORDER_RECENT, &up, &sync) < 0 && compare_rows(ORDER_RECENT, &sync, &down) < 0 && compare_rows(ORDER_RECENT, &down, &conflict) < 0,
          "last played: most recent first, no date last");
    check(compare_rows(ORDER_RECENT, &conflict, &dup) < 0 && compare_rows(ORDER_RECENT, &dup, &offline) < 0, "last played: no date, by name");

    printf("%s\n", failures ? "ECHECS" : "filtre : OK");
    return failures != 0;
}
