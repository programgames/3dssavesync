/* Checks the status of a Game in the list: Duplicate, hidden, Unknown, the Blank save rule, the Base to write and the
 * display order. Built on the PC: gcc -std=c99 -I source source/state.c source/games.c test/test_games.c -o /tmp/g && /tmp/g */
#include <stdio.h>
#include "games.h"

static int failures;

static void check(int ok, const char *what)
{
    if (!ok) {
        printf("ECHEC  %s\n", what);
        failures++;
    }
}

#define H1 "1111111111111111111111111111111111111111111111111111111111111111"
#define H2 "2222222222222222222222222222222222222222222222222222222222222222"

int main(void)
{
    const obs_t a = { H1, 100 }, b = { H2, 100 }, a_big = { H1, 200 };
    observation_t o;

    o = (observation_t){ 2, &a, 0, &b };
    check(game_status(&o, NULL, 1) == STATUS_DUPLICATE, "two ROMs: Duplicate");
    check(game_status(&o, NULL, 0) == STATUS_DUPLICATE, "two ROMs, server not read: Duplicate");

    o = (observation_t){ 1, NULL, 0, NULL };
    check(game_status(&o, NULL, 1) == STATUS_HIDDEN, "no save, no version: hidden");
    check(game_status(&o, NULL, 0) == STATUS_HIDDEN, "no save, server not read: hidden");
    o = (observation_t){ 1, &a, 1, NULL };
    check(game_status(&o, NULL, 1) == STATUS_HIDDEN, "Blank save, no version: hidden");
    check(game_status(&o, H1, 1) == STATUS_HIDDEN, "Blank save, no version, with a Base: hidden");

    o = (observation_t){ 1, &a, 1, &a };
    check(game_status(&o, NULL, 1) == STATUS_IN_SYNC, "Blank save equal to the Current version: In sync");
    o = (observation_t){ 1, &a, 1, &b };
    check(game_status(&o, NULL, 1) == STATUS_TO_DOWNLOAD, "Blank save, other Current version: To download");
    check(game_status(&o, H2, 1) == STATUS_TO_DOWNLOAD, "Blank save, Base = server: To download");

    o = (observation_t){ 1, &a, 0, NULL };
    check(game_status(&o, NULL, 0) == STATUS_OFFLINE, "server not read: Unknown");
    check(game_status(&o, NULL, 1) == STATUS_TO_UPLOAD, "no Current version: To upload");
    o = (observation_t){ 1, NULL, 0, &b };
    check(game_status(&o, NULL, 1) == STATUS_TO_DOWNLOAD, "no Local save: To download");
    o = (observation_t){ 1, &a, 0, &a };
    check(game_status(&o, H2, 1) == STATUS_IN_SYNC, "same Hash: In sync");
    o = (observation_t){ 1, &a, 0, &b };
    check(game_status(&o, H2, 1) == STATUS_TO_UPLOAD, "Base = server: To upload");
    check(game_status(&o, H1, 1) == STATUS_TO_DOWNLOAD, "Base = 3DS: To download");
    check(game_status(&o, NULL, 1) == STATUS_CONFLICT, "no Base: Conflict");
    o = (observation_t){ 1, &a_big, 0, &b };
    check(game_status(&o, H2, 1) == STATUS_CONFLICT, "sizes differ: Conflict");

    check(base_outdated(STATUS_IN_SYNC, NULL, H1), "In sync without a Base: written");
    check(base_outdated(STATUS_IN_SYNC, H2, H1), "In sync, other Base: written");
    check(!base_outdated(STATUS_IN_SYNC, H1, H1), "In sync, same Base: not written");
    check(!base_outdated(STATUS_TO_UPLOAD, NULL, H1) && !base_outdated(STATUS_OFFLINE, NULL, H1) &&
              !base_outdated(STATUS_DUPLICATE, NULL, H1), "not In sync: Base not written");

    check(status_priority(STATUS_CONFLICT) == 0 && status_priority(STATUS_TO_DOWNLOAD) == 1 && status_priority(STATUS_TO_UPLOAD) == 2 &&
              status_priority(STATUS_DUPLICATE) == 3 && status_priority(STATUS_IN_SYNC) == 4 && status_priority(STATUS_OFFLINE) == 4,
          "display order");

    printf("%s\n", failures ? "FAILED" : "games: OK");
    return failures != 0;
}
