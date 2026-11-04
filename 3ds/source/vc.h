/* GB/GBC Virtual Console titles (3DS eShop): which titles, and which Save archives can be synced.
 * No libctru dependency: tested on the PC (test/test_vc.c). */
#ifndef VC_H
#define VC_H

/* 1 if the product code (AM_GetTitleProductCode, "CTR-N-QBXA") is the one of a Game Boy or Game Boy Color Virtual
 * Console title, 0 otherwise. */
int vc_product(const char *code);

/* root: names at the root of the Save archive, as title_root() writes them ("sav.dat, SecureValue").
 * 1 if it holds only /sav.dat and optionally SecureValue, 0 otherwise (rsm*.dat restore points or savestates, which
 * could bring an old SRAM back; folders, other files: not synced). */
int vc_root_ok(const char *root);

#endif
