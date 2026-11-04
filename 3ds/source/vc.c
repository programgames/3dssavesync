#include <string.h>
#include "vc.h"

/* Product codes of the 3DS eShop Virtual Console (3dsdb, list_GB/US/JP.json): game code Rxxx = Game Boy (CTR-N-RCQA
 * Pokémon Red, CTR-N-RA3P Donkey Kong Land), Qxxx = Game Boy Color (CTR-N-QBXA Pokémon Crystal FR, CTR-N-QA2P Mega
 * Man Xtreme 2). All 271 R/Q titles of 3dsdb are GB/GBC VC; 9 Gen 1 Pokémon have CTR-P- instead of CTR-N- (CTR-P-RCSA
 * Rote Edition). Other letters are other systems (T: NES, J: Master System, K: Mega Drive) or eShop games. */
int vc_product(const char *code)
{
    return !strncmp(code, "CTR-", 4) && (code[4] == 'N' || code[4] == 'P') && code[5] == '-' && (code[6] == 'R' || code[6] == 'Q');
}

int vc_root_ok(const char *root)
{
    return !strcmp(root, "sav.dat") || !strcmp(root, "sav.dat, SecureValue") || !strcmp(root, "SecureValue, sav.dat");
}
