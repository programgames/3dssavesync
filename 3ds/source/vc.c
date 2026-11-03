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
