/* Tests de pokemon.c : une sauvegarde synthétique par génération (sommes de contrôle et signatures valides),
 * plus des fichiers à refuser. Vérifié aussi, hors dépôt, sur de vraies sauvegardes de chaque génération. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "pokemon.h"

static int echecs;

static void verifie(const unsigned char *d, size_t taille, int h, int m, const char *nom, const char *quoi)
{
    partie_t p;
    int r = pokemon_partie(d, taille, &p);
    if (r != 0 || p.heures != h || p.minutes != m || strcmp(p.dresseur, nom)) {
        printf("ÉCHEC : %s (%d, %d h %d, « %s »)\n", quoi, r, p.heures, p.minutes, p.dresseur);
        echecs++;
    }
}

static void refuse(const unsigned char *d, size_t taille, const char *quoi)
{
    partie_t p;
    if (pokemon_partie(d, taille, &p) != -1) {
        printf("ÉCHEC : %s reconnu à tort\n", quoi);
        echecs++;
    }
}

static void le16(unsigned char *d, unsigned v) { d[0] = v & 0xFF, d[1] = v >> 8; }
static void le32(unsigned char *d, unsigned long v) { le16(d, v & 0xFFFF), le16(d + 2, v >> 16); }

static unsigned crc16_ccitt(const unsigned char *d, size_t n)
{
    unsigned c = 0xFFFF;
    int k;
    while (n--)
        for (c ^= (unsigned)*d++ << 8, k = 0; k < 8; k++) c = (c & 0x8000 ? c << 1 ^ 0x1021 : c << 1) & 0xFFFF;
    return c;
}

int main(void)
{
    unsigned char *d = calloc(0x80000, 1);
    unsigned s, i;
    int k;

    /* Gen 1 : « Jüé » -> J, ?, é ; 57 h 32 ; somme de contrôle à 0x3523. */
    memset(d, 0, 0x8000);
    d[0x2598] = 0x89, d[0x2599] = 0xE0, d[0x259A] = 0xBC, d[0x259B] = 0x50;
    d[0x2CED] = 57, d[0x2CEF] = 32;
    for (s = 0, i = 0x2598; i < 0x3523; i++) s += d[i];
    d[0x3523] = ~s & 0xFF;
    verifie(d, 0x8000, 57, 32, "J?\xC3\xA9", "Gen 1");

    /* Gen 2 Cristal : « Jean » ; 295 h 10 (heures big-endian) ; somme 0x2009..0x2B82 à 0x2D0D. */
    memset(d, 0, 0x8000);
    d[0x200B] = 0x89, d[0x200C] = 0xA4, d[0x200D] = 0xA0, d[0x200E] = 0xAD, d[0x200F] = 0x50;
    d[0x2052] = 295 >> 8, d[0x2053] = 295 & 0xFF, d[0x2054] = 10;
    for (s = 0, i = 0x2009; i <= 0x2B82; i++) s += d[i];
    le16(d + 0x2D0D, s & 0xFFFF);
    verifie(d, 0x8000, 295, 10, "Jean", "Gen 2 Cristal");
    memset(d, 0, 0x8000);
    refuse(d, 0x8000, "32 Kio vierges");

    /* Gen 3 : deux moitiés valides, secteurs décalés ; la moitié 1 a le plus grand compteur. */
    memset(d, 0, 0x20000);
    for (k = 0; k < 28; k++) {
        unsigned m = k / 14, id = (k % 14 + 3 + m) % 14; /* rotation des secteurs */
        unsigned char *sec = d + m * 0xE000 + (k % 14) * 0x1000;
        le16(sec + 0xFF4, id);
        le32(sec + 0xFFC, m ? 9 : 4);
        if (id == 0) {
            sec[0] = m ? 0xC4 : 0xBB; /* J ou A */
            sec[1] = m ? 0x1B : 0xFF; /* é */
            sec[2] = 0xFF;
            le16(sec + 0x0E, m ? 576 : 1);
            sec[0x10] = m ? 48 : 2;
        }
    }
    verifie(d, 0x20000, 576, 48, "J\xC3\xA9", "Gen 3, moitié au plus grand compteur");
    memset(d, 0, 0x20000);
    refuse(d, 0x20000, "128 Kio vierges");

    /* Gen 4 Platine : pied de la partition 1 (taille, signature) ; partition 1 plus récente. */
    memset(d, 0, 0x80000);
    le32(d + 0x40000 + 0xCF2C - 0x0C, 0xCF2C);
    le32(d + 0x40000 + 0xCF2C - 0x08, 0x20060623);
    le32(d + 0xCF2C - 0x14, 1);
    le32(d + 0x40000 + 0xCF2C - 0x14, 2);
    le16(d + 0x40000 + 0x68, 0x134), le16(d + 0x40000 + 0x6A, 0x188), le16(d + 0x40000 + 0x6C, 0xFFFF); /* J, é */
    le16(d + 0x40000 + 0x68 + 0x22, 35);
    d[0x40000 + 0x68 + 0x24] = 7;
    verifie(d, 0x80000, 35, 7, "J\xC3\xA9", "Gen 4 Platine");

    /* Gen 5 Noir 2/Blanc 2 : CRC16-CCITT du bloc 0x25F00 ; nom UTF-16. */
    memset(d, 0, 0x80000);
    for (i = 0; i < 0x94; i++) d[0x25F00 + i] = (unsigned char)(i * 7);
    le16(d + 0x25FA2, crc16_ccitt(d + 0x25F00, 0x94));
    le16(d + 0x19404, 'J'), le16(d + 0x19406, 0xE9), le16(d + 0x19408, 0xFFFF);
    le16(d + 0x19424, 15);
    d[0x19426] = 41;
    verifie(d, 0x80000, 15, 41, "J\xC3\xA9", "Gen 5 Noir 2/Blanc 2");
    memset(d, 0, 0x80000);
    refuse(d, 0x80000, "512 Kio vierges");

    /* Gen 6/7 : reconnues par la taille ; nom de 13 caractères au plus. */
    memset(d, 0, 0x6CC00);
    le16(d + 0x41000, 999), d[0x41002] = 30;
    for (i = 0; i < 15; i++) le16(d + 0x1438 + 2 * i, 'A' + i);
    verifie(d, 0x6CC00, 999, 30, "ABCDEFGHIJKLM", "Ultra-Soleil/Ultra-Lune");
    memset(d, 0, 0x6BE00);
    verifie(d, 0x6BE00, 0, 0, "", "Soleil/Lune jamais commencé");

    refuse(d, 16, "taille inconnue");
    free(d);
    printf(echecs ? "%d échec(s)\n" : "pokemon : OK\n", echecs);
    return echecs != 0;
}
