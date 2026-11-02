/* Tests de pokemon.c : un main synthétique par format, plus une taille inconnue. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "pokemon.h"

static int echecs;

static void verifie(int ok, const char *quoi)
{
    if (!ok) {
        printf("ÉCHEC : %s\n", quoi);
        echecs++;
    }
}

/* main de la taille donnée, temps de jeu et dresseur (UTF-16) aux positions données. */
static unsigned char *main_factice(size_t taille, size_t temps, size_t dresseur, int h, int m, const unsigned short *nom)
{
    unsigned char *d = calloc(taille, 1);
    size_t k;
    d[temps] = h & 0xFF;
    d[temps + 1] = h >> 8;
    d[temps + 2] = (unsigned char)m;
    for (k = 0; nom[k]; k++) {
        d[dresseur + 2 * k] = nom[k] & 0xFF;
        d[dresseur + 2 * k + 1] = nom[k] >> 8;
    }
    return d;
}

int main(void)
{
    static const unsigned short julien[] = { 'J', 'u', 'l', 'i', 'e', 'n', 0 };
    static const unsigned short eloise[] = { 0xC9, 'l', 'o', 'i', 's', 'e', 0 }; /* É */
    static const unsigned short long_nom[] = { 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 0 };
    static const unsigned short vide[] = { 0 };
    const struct { size_t taille, temps, dresseur; int h, m; const unsigned short *nom; const char *attendu; } cas[] = {
        { 0x65600, 0x01800, 0x14048, 312, 7, julien, "Julien" },        /* X/Y */
        { 0x76000, 0x01800, 0x14048, 1, 59, eloise, "\xC3\x89loise" },  /* Rubis Oméga/Saphir Alpha */
        { 0x6BE00, 0x40C00, 0x01238, 0, 0, vide, "" },                  /* Soleil/Lune : partie jamais commencée */
        { 0x6CC00, 0x41000, 0x01438, 999, 30, long_nom, "ABCDEFGHIJKLM" }, /* Ultra : 13 caractères au plus */
    };
    partie_t p;
    size_t i;
    unsigned char inconnu[16] = { 0 };

    for (i = 0; i < sizeof cas / sizeof *cas; i++) {
        unsigned char *d = main_factice(cas[i].taille, cas[i].temps, cas[i].dresseur, cas[i].h, cas[i].m, cas[i].nom);
        memset(&p, 0x55, sizeof p);
        verifie(pokemon_partie(d, cas[i].taille, &p) == 0, "format reconnu");
        verifie(p.heures == cas[i].h && p.minutes == cas[i].m, "temps de jeu");
        verifie(strcmp(p.dresseur, cas[i].attendu) == 0, "dresseur");
        free(d);
    }
    verifie(pokemon_partie(inconnu, sizeof inconnu, &p) == -1, "taille inconnue refusée");
    printf(echecs ? "%d échec(s)\n" : "pokemon : OK\n", echecs);
    return echecs != 0;
}
