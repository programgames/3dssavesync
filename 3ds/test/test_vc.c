/* Checks vc.c: Virtual Console Pokémon title IDs, content of the Save archive.
 * Built on the PC: gcc -std=c99 -Wall -I source source/vc.c test/test_vc.c -o /tmp/v && /tmp/v */
#include <stdio.h>
#include <string.h>
#include "vc.h"

static int failures, total;

static void check(int actual, int expected, const char *what)
{
    total++;
    if (actual != expected) {
        printf("ECHEC : %s : attendu %d, obtenu %d\n", what, expected, actual);
        failures++;
    }
}

int main(void)
{
    check(vc_title(0x0004000000170C00ULL), 1, "Red JP (first)");
    check(vc_title(0x0004000000171000ULL), 1, "Red EN");
    check(vc_title(0x0004000000171600ULL), 1, "Rouge FR");
    check(vc_title(0x0004000000171E00ULL), 1, "Yellow IT (last Gen 1)");
    check(vc_title(0x0004000000172300ULL), 1, "Gold JP (first Gen 2)");
    check(vc_title(0x0004000000172E00ULL), 1, "Cristal FR");
    check(vc_title(0x0004000000173600ULL), 1, "Silver KR (last)");
    check(vc_title(0x0004000000170B00ULL), 0, "before Red JP");
    check(vc_title(0x0004000000171F00ULL), 0, "between Gen 1 and Gen 2");
    check(vc_title(0x0004000000172200ULL), 0, "between Gen 1 and Gen 2");
    check(vc_title(0x0004000000173700ULL), 0, "after Silver KR");
    check(vc_title(0x0004000000171601ULL), 0, "low byte not 0");
    check(vc_title(0x0004000E00171600ULL), 0, "update, not the application");
    check(vc_title(0x00040000001B5000ULL), 0, "Pokémon X (3DS)");

    check(vc_root_ok("sav.dat"), 1, "sav.dat");
    check(vc_root_ok("sav.dat, SecureValue"), 1, "sav.dat, SecureValue");
    check(vc_root_ok("SecureValue, sav.dat"), 1, "SecureValue, sav.dat");
    check(vc_root_ok("sav.dat, SecureValue, rsm2.dat"), 0, "savestate");
    check(vc_root_ok("rsm1.dat, sav.dat"), 0, "restore point");
    check(vc_root_ok("sav.dat/"), 0, "folder named sav.dat");
    check(vc_root_ok("main"), 0, "native 3DS save");
    check(vc_root_ok("SecureValue"), 0, "no sav.dat");
    check(vc_root_ok(""), 0, "empty archive");
    check(vc_root_ok("SAV.DAT"), 0, "other case");

    check(!strcmp(vc_known_key(0x0004000000171000ULL), "GB-POKEMON_RED-91E6"), 1, "Red EN: key from the table");
    check(!strcmp(vc_known_key(0x0004000000171200ULL), "GB-POKEMON_YELLOW-047C"), 1, "Yellow EN: key from the table");
    check(vc_known_key(0x0004000000172E00ULL) == NULL, 1, "Cristal FR: no source, no key");
    check(vc_known_key(0x0004000000171600ULL) == NULL, 1, "Rouge FR: no source, no key");

    printf("%d/%d cas OK\n", total - failures, total);
    return failures != 0;
}
