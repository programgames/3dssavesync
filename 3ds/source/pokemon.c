/* Positions de PKHeX (SaveBlockAccessor6XY, 6AO, 7SM, 7USUM ; PlayTime6 ; MyStatus6/7), choisies par la taille de main
 * (SaveUtil.cs : SIZE_G6XY, SIZE_G6ORAS, SIZE_G7SM, SIZE_G7USUM). Temps de jeu : u16 heures, u8 minutes, u8 secondes.
 * Dresseur : 13 caractères UTF-16 au plus. */
#include "pokemon.h"

static const struct { size_t taille, temps, dresseur; } FORMATS[] = {
    { 0x65600, 0x01800, 0x14048 }, /* X/Y */
    { 0x76000, 0x01800, 0x14048 }, /* Rubis Oméga/Saphir Alpha */
    { 0x6BE00, 0x40C00, 0x01238 }, /* Soleil/Lune */
    { 0x6CC00, 0x41000, 0x01438 }, /* Ultra-Soleil/Ultra-Lune */
};

int pokemon_partie(const unsigned char *d, size_t taille, partie_t *p)
{
    size_t i, k, j = 0;
    for (i = 0; i < sizeof FORMATS / sizeof *FORMATS && FORMATS[i].taille != taille; i++);
    if (i == sizeof FORMATS / sizeof *FORMATS) return -1;
    p->heures = d[FORMATS[i].temps] | d[FORMATS[i].temps + 1] << 8;
    p->minutes = d[FORMATS[i].temps + 2];
    for (k = 0; k < 13; k++) { /* UTF-16 (plan de base) -> UTF-8 */
        unsigned c = d[FORMATS[i].dresseur + 2 * k] | d[FORMATS[i].dresseur + 2 * k + 1] << 8;
        if (!c || j + 4 > sizeof p->dresseur) break;
        if (c < 0x80) p->dresseur[j++] = (char)c;
        else if (c < 0x800) p->dresseur[j++] = (char)(0xC0 | c >> 6), p->dresseur[j++] = (char)(0x80 | (c & 0x3F));
        else p->dresseur[j++] = (char)(0xE0 | c >> 12), p->dresseur[j++] = (char)(0x80 | (c >> 6 & 0x3F)), p->dresseur[j++] = (char)(0x80 | (c & 0x3F));
    }
    p->dresseur[j] = '\0';
    return 0;
}
