/* GB/GBC Virtual Console Pokémon titles (3DS eShop): which titles, and which Save archives can be synced.
 * No libctru dependency: tested on the PC (test/test_vc.c). */
#ifndef VC_H
#define VC_H

/* 1 if the title ID is a Pokémon Red, Green, Blue, Yellow, Gold, Silver or Crystal Virtual Console title (JP, EN, DE,
 * FR, ES, IT, KR), 0 otherwise. */
int vc_title(unsigned long long id);

/* root: names at the root of the Save archive, as title_root() writes them ("sav.dat, SecureValue").
 * 1 if it holds only /sav.dat and optionally SecureValue (what the VC Pokémon emulator writes), 0 otherwise
 * (rsm*.dat savestates, folders, other files: not synced). */
int vc_root_ok(const char *root);

/* Key of the VC title id from a fixed table with sources (vc.c), for when its ROM can't be read. NULL if unknown: the
 * game is then not synced. */
const char *vc_known_key(unsigned long long id);

#endif
