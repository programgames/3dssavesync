#include <string.h>
#include "vc.h"

/* Product codes of the 3DS eShop Virtual Console (3dsdb, list_GB.json): CTR-N-Rxxx = Game Boy (CTR-N-RCQA Pokémon Red,
 * CTR-N-RA3P Donkey Kong Land), CTR-N-Qxxx = Game Boy Color (CTR-N-QBXA Pokémon Crystal FR, CTR-N-QA2P Mega Man Xtreme
 * 2). Other letters are other systems (T: NES, J: Master System, K: Mega Drive) or eShop games. */
int vc_product(const char *code)
{
    return !strncmp(code, "CTR-N-R", 7) || !strncmp(code, "CTR-N-Q", 7);
}

int vc_root_ok(const char *root)
{
    return !strcmp(root, "sav.dat") || !strcmp(root, "sav.dat, SecureValue") || !strcmp(root, "SecureValue, sav.dat");
}
