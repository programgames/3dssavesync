/* Checks vc.c: GB/GBC Virtual Console product codes, content of the Save archive.
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
    check(vc_product("CTR-N-RCQA"), 1, "Pokemon Red (GB)");
    check(vc_product("CTR-N-RA3P"), 1, "Donkey Kong Land (GB)");
    check(vc_product("CTR-N-QBXA"), 1, "Pokemon Cristal FR (GBC)");
    check(vc_product("CTR-N-QA2P"), 1, "Mega Man Xtreme 2 (GBC)");
    check(vc_product("CTR-P-RCSA"), 1, "Pokemon Rote Edition (CTR-P-)");
    check(vc_product("CTR-P-QBKA"), 1, "Pokemon Versione Gialla (CTR-P-)");
    check(vc_product("CTR-P-BFWP"), 0, "Fire Emblem Fates (card)");
    check(vc_product("CTR-U-RCQA"), 0, "update code");
    check(vc_product("KTR-N-RAAA"), 0, "New 3DS title");
    check(vc_product("CTR-N-TD3P"), 0, "Adventures of Lolo (NES)");
    check(vc_product("CTR-N-JF8P"), 0, "3D Fantasy Zone (Master System)");
    check(vc_product("CTR-P-EKJA"), 0, "Pokemon X (3DS card)");
    check(vc_product("CTR-N-R"), 1, "prefix only");
    check(vc_product("CTR-N-"), 0, "too short");
    check(vc_product(""), 0, "empty (not read)");

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

    printf("%d/%d cas OK\n", total - failures, total);
    return failures != 0;
}
