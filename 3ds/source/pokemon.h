/* Play time and trainer of the Pokémon games, to compare two Versions before a choice (Conflict screen,
 * Download). No libctru dependency: tested on the PC (test/test_pokemon.c). */
#ifndef POKEMON_H
#define POKEMON_H

#include <stddef.h>

typedef struct {
    int hours, minutes;
    char trainer[40]; /* UTF-8; empty: game never started */
} progress_t;

/* 0 if the save is a known Pokémon game: Red/Blue/Yellow, Gold/Silver/Crystal, Ruby/Sapphire/Emerald/FireRed/
 * LeafGreen, Diamond/Pearl/Platinum/HeartGold/SoulSilver, Black/White 1 and 2 (European and American versions),
 * then the 3DS games (main file). The save is the SRAM only, without Clock trailer. -1 otherwise. */
int pokemon_progress(const unsigned char *d, size_t size, progress_t *p);

#endif
