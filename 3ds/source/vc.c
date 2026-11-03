#include <string.h>
#include "vc.h"

/* eShop title IDs 00040000 0017xx00 (nus-info, 3dsdb): Gen 1 from 00170C00 (Red JP) to 00171E00 (Yellow IT),
 * Gen 2 from 00172300 (Gold JP) to 00173600 (Silver KR), one title per step of 0x100, no gap. */
int vc_title(unsigned long long id)
{
    unsigned long low = (unsigned long)(id & 0xFFFFFFFF);
    if ((id >> 32) != 0x00040000 || (low & 0xFF)) return 0;
    return (low >= 0x00170C00 && low <= 0x00171E00) || (low >= 0x00172300 && low <= 0x00173600);
}

int vc_root_ok(const char *root)
{
    return !strcmp(root, "sav.dat") || !strcmp(root, "sav.dat, SecureValue") || !strcmp(root, "SecureValue, sav.dat");
}

/* Key of a VC ROM, for when the 3DS can't read the ROM (title_rom_header). Only a source for both the header title and
 * the global checksum (0x14E) of that exact ROM gives an entry; a key is never guessed.
 * The English VC ROM is the cartridge ROM that pret builds: the VC .patch is made from it (pokered/pokeyellow Makefile,
 * rule %.patch; tools/make_patch.c). Header title: pret Makefile, rgbfix -t. Checksum: Data Crystal infobox
 * "ROM Checksum" (Red 0x91E6, Yellow 0x047C "U.S.A. & Europe"). Gold, Silver, Crystal and the other languages: no
 * source found for the checksum. */
static const struct { unsigned long long id; const char *key; } KNOWN[] = {
    { 0x0004000000171000ULL, "GB-POKEMON_RED-91E6" },    /* Red, English */
    { 0x0004000000171200ULL, "GB-POKEMON_YELLOW-047C" }, /* Yellow, English */
};

const char *vc_known_key(unsigned long long id)
{
    size_t i;
    for (i = 0; i < sizeof KNOWN / sizeof *KNOWN; i++)
        if (KNOWN[i].id == id) return KNOWN[i].key;
    return NULL;
}
