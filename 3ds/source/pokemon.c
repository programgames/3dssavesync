/* Temps de jeu et dresseur des Pokémon, de Rouge/Bleu à Ultra-Lune. Positions et tables de caractères de PKHeX
 * (docs/research-pokemon-temps-de-jeu.md) ; vérifiées sur de vraies sauvegardes. Le jeu est reconnu par la taille,
 * puis par une somme de contrôle ou une signature : un autre jeu de même taille n'est pas pris pour un Pokémon. */
#include "pokemon.h"

static unsigned u16le(const unsigned char *d) { return d[0] | d[1] << 8; }
static unsigned long u32le(const unsigned char *d) { return d[0] | d[1] << 8 | (unsigned long)d[2] << 16 | (unsigned long)d[3] << 24; }

/* Ajoute un caractère (point de code du plan de base) au nom, en UTF-8. */
static void ajoute(partie_t *p, size_t *j, unsigned c)
{
    if (*j + 4 > sizeof p->dresseur) return;
    if (c < 0x80) p->dresseur[(*j)++] = (char)c;
    else if (c < 0x800) p->dresseur[(*j)++] = (char)(0xC0 | c >> 6), p->dresseur[(*j)++] = (char)(0x80 | (c & 0x3F));
    else p->dresseur[(*j)++] = (char)(0xE0 | c >> 12), p->dresseur[(*j)++] = (char)(0x80 | (c >> 6 & 0x3F)), p->dresseur[(*j)++] = (char)(0x80 | (c & 0x3F));
    p->dresseur[*j] = '\0';
}

/* Code -> caractère dans une table de paires ; '?' si absent. */
static unsigned cherche(unsigned c, const unsigned short (*t)[2], size_t n)
{
    size_t i;
    for (i = 0; i < n; i++)
        if (t[i][0] == c) return t[i][1];
    return '?';
}

/* Gen 1 et 2 (StringConverter1, StringConverter2 TableFRE) : lettres, chiffres, accents français. */
static unsigned car_g12(unsigned c, int gen2)
{
    static const unsigned short commun[][2] = { { 0x7F, ' ' }, { 0xBA, 0xE0 }, { 0xBB, 0xE8 }, { 0xBC, 0xE9 }, { 0xBD, 0xF9 } };
    static const unsigned short g1[][2] = { { 0xBE, 0xC0 }, { 0xC6, 0xC8 }, { 0xC7, 0xC9 } };
    static const unsigned short g2[][2] = { { 0xBF, 0xE7 }, { 0xC6, 0xEB }, { 0xC7, 0xEF }, { 0xC8, 0xE2 }, { 0xC9, 0xF4 }, { 0xCA, 0xFB }, { 0xCB, 0xEA }, { 0xCC, 0xEE } };
    unsigned r;
    if (c >= 0x80 && c <= 0x99) return 'A' + c - 0x80;
    if (c >= 0xA0 && c <= 0xB9) return 'a' + c - 0xA0;
    if (c >= 0xF6) return '0' + c - 0xF6;
    if ((r = cherche(c, commun, sizeof commun / sizeof *commun)) != '?') return r;
    return gen2 ? cherche(c, g2, sizeof g2 / sizeof *g2) : cherche(c, g1, sizeof g1 / sizeof *g1);
}

/* Gen 3 (StringConverter3, G3_EN). */
static unsigned car_g3(unsigned c)
{
    static const unsigned short t[][2] = { { 0x00, ' ' }, { 0x01, 0xC0 }, { 0x04, 0xC7 }, { 0x05, 0xC8 }, { 0x06, 0xC9 }, { 0x16, 0xE0 }, { 0x19, 0xE7 }, { 0x1A, 0xE8 },
                                           { 0x1B, 0xE9 }, { 0x1C, 0xEA }, { 0x1D, 0xEB }, { 0x20, 0xEE }, { 0x21, 0xEF }, { 0x24, 0xF4 }, { 0x26, 0xF9 }, { 0x28, 0xFB } };
    if (c >= 0xBB && c <= 0xD4) return 'A' + c - 0xBB;
    if (c >= 0xD5 && c <= 0xEE) return 'a' + c - 0xD5;
    if (c >= 0xA1 && c <= 0xAA) return '0' + c - 0xA1;
    return cherche(c, t, sizeof t / sizeof *t);
}

/* Gen 4 (StringConverter4, TableINT). */
static unsigned car_g4(unsigned c)
{
    static const unsigned short t[][2] = { { 0x1DE, ' ' }, { 0x15F, 0xC0 }, { 0x166, 0xC7 }, { 0x167, 0xC8 }, { 0x168, 0xC9 }, { 0x17F, 0xE0 }, { 0x181, 0xE2 }, { 0x186, 0xE7 },
                                           { 0x187, 0xE8 }, { 0x188, 0xE9 }, { 0x189, 0xEA }, { 0x18A, 0xEB }, { 0x18D, 0xEE }, { 0x18E, 0xEF }, { 0x193, 0xF4 }, { 0x198, 0xF9 }, { 0x19A, 0xFB } };
    if (c >= 0x121 && c <= 0x12A) return '0' + c - 0x121;
    if (c >= 0x12B && c <= 0x144) return 'A' + c - 0x12B;
    if (c >= 0x145 && c <= 0x15E) return 'a' + c - 0x145;
    return cherche(c, t, sizeof t / sizeof *t);
}

/* Nom en octets (Gen 1 à 3) : n caractères au plus, jusqu'au code de fin. gen : 1, 2 ou 3. */
static void nom8(partie_t *p, const unsigned char *d, size_t n, unsigned fin, int gen)
{
    size_t i, j = 0;
    for (i = 0; i < n && d[i] != fin; i++) ajoute(p, &j, gen == 3 ? car_g3(d[i]) : car_g12(d[i], gen == 2));
}

/* Nom en mots de 16 bits (Gen 4 : table propre ; Gen 5 à 7 : UTF-16), jusqu'à 0 ou 0xFFFF. */
static void nom16(partie_t *p, const unsigned char *d, size_t n, int gen4)
{
    size_t i, j = 0;
    for (i = 0; i < n; i++) {
        unsigned c = u16le(d + 2 * i);
        if (!c || c == 0xFFFF) break;
        ajoute(p, &j, gen4 ? car_g4(c) : c);
    }
}

/* Gen 1 et 2 (32 Kio) : reconnues par leur somme de contrôle (SAV1, SAV2 : Or/Argent puis Cristal, versions INT). */
static int gen12(const unsigned char *d, partie_t *p)
{
    static const struct { unsigned fin, somme, heures; } g2[] = { { 0x2D68, 0x2D69, 0x2053 }, { 0x2B82, 0x2D0D, 0x2052 } };
    unsigned s = 0, i, k;
    for (i = 0x2598; i < 0x3523; i++) s += d[i];
    if ((~s & 0xFF) == d[0x3523]) {
        p->heures = d[0x2CED];
        p->minutes = d[0x2CEF];
        nom8(p, d + 0x2598, 7, 0x50, 1);
        return 0;
    }
    for (k = 0; k < 2; k++) {
        for (s = 0, i = 0x2009; i <= g2[k].fin; i++) s += d[i];
        if (s && (s & 0xFFFF) == u16le(d + g2[k].somme)) { /* somme nulle : fichier vierge, pas une partie */
            p->heures = d[g2[k].heures] << 8 | d[g2[k].heures + 1]; /* big-endian */
            p->minutes = d[g2[k].heures + 2];
            nom8(p, d + 0x200B, 7, 0x50, 2);
            return 0;
        }
    }
    return -1;
}

/* Gen 3 (128 Kio) : deux moitiés de 14 secteurs ; la moitié valide au plus grand compteur ; section 0 : nom et temps. */
static int gen3(const unsigned char *d, partie_t *p)
{
    long s0 = -1;
    unsigned long meilleur = 0;
    int m, k;
    for (m = 0; m < 2; m++) {
        int vus = 0;
        long zero = -1;
        for (k = 0; k < 14; k++) {
            unsigned id = u16le(d + m * 0xE000 + k * 0x1000 + 0xFF4);
            if (id >= 14 || vus & 1 << id) break;
            vus |= 1 << id;
            if (!id) zero = m * 0xE000 + k * 0x1000;
        }
        if (vus == 0x3FFF && (s0 < 0 || u32le(d + zero + 0xFFC) > meilleur)) {
            s0 = zero;
            meilleur = u32le(d + zero + 0xFFC);
        }
    }
    if (s0 < 0) return -1;
    p->heures = u16le(d + s0 + 0x0E);
    p->minutes = d[s0 + 0x10];
    nom8(p, d + s0, 7, 0xFF, 3);
    return 0;
}

/* Gen 4 (512 Kio) : pied du bloc général de la partition 1 (taille, signature) ; bloc actif par les compteurs. */
static int gen4(const unsigned char *d, partie_t *p)
{
    static const struct { unsigned long g; unsigned dresseur; } jeux[] = { { 0xC100, 0x64 }, { 0xCF2C, 0x68 }, { 0xF628, 0x64 } }; /* DP, Pt, HGSS */
    size_t i;
    for (i = 0; i < 3; i++) {
        unsigned long g = jeux[i].g, b;
        const unsigned char *p0 = d + g - 0x14, *p1 = d + 0x40000 + g - 0x14;
        unsigned long sig = u32le(p1 + 0x0C);
        if (u32le(p1 + 0x08) != g || (sig != 0x20060623 && sig != 0x20070903)) continue;
        /* ponytail : plus grand compteur majeur puis mineur ; PKHeX traite aussi 0xFFFFFFFE */
        b = u32le(p0) == 0xFFFFFFFF || (u32le(p1) != 0xFFFFFFFF && (u32le(p1) > u32le(p0) || (u32le(p1) == u32le(p0) && u32le(p1 + 4) > u32le(p0 + 4)))) ? 0x40000 : 0;
        p->heures = u16le(d + b + jeux[i].dresseur + 0x22);
        p->minutes = d[b + jeux[i].dresseur + 0x24];
        nom16(p, d + b + jeux[i].dresseur, 8, 1);
        return 0;
    }
    return -1;
}

static unsigned crc16_ccitt(const unsigned char *d, size_t n)
{
    unsigned c = 0xFFFF;
    int k;
    while (n--)
        for (c ^= (unsigned)*d++ << 8, k = 0; k < 8; k++) c = (c & 0x8000 ? c << 1 ^ 0x1021 : c << 1) & 0xFFFF;
    return c;
}

/* Gen 5 (512 Kio) : bloc des sommes de contrôle valide (Noir/Blanc ou Noir 2/Blanc 2) ; données du dresseur à 0x19400. */
static int gen5(const unsigned char *d, partie_t *p)
{
    if (crc16_ccitt(d + 0x23F00, 0x8C) != u16le(d + 0x23F9A) && crc16_ccitt(d + 0x25F00, 0x94) != u16le(d + 0x25FA2)) return -1;
    p->heures = u16le(d + 0x19424);
    p->minutes = d[0x19426];
    nom16(p, d + 0x19404, 8, 0);
    return 0;
}

/* Gen 6 et 7 (main des jeux 3DS) : reconnus par la taille (SaveUtil.cs : SIZE_G6XY, SIZE_G6ORAS, SIZE_G7SM, SIZE_G7USUM). */
static int gen67(const unsigned char *d, size_t taille, partie_t *p)
{
    static const struct { size_t taille, temps, dresseur; } f[] = {
        { 0x65600, 0x01800, 0x14048 }, /* X/Y */
        { 0x76000, 0x01800, 0x14048 }, /* Rubis Oméga/Saphir Alpha */
        { 0x6BE00, 0x40C00, 0x01238 }, /* Soleil/Lune */
        { 0x6CC00, 0x41000, 0x01438 }, /* Ultra-Soleil/Ultra-Lune */
    };
    size_t i;
    for (i = 0; i < 4 && f[i].taille != taille; i++);
    if (i == 4) return -1;
    p->heures = u16le(d + f[i].temps);
    p->minutes = d[f[i].temps + 2];
    nom16(p, d + f[i].dresseur, 13, 0);
    return 0;
}

int pokemon_partie(const unsigned char *d, size_t taille, partie_t *p)
{
    p->heures = p->minutes = 0;
    p->dresseur[0] = '\0';
    switch (taille) {
    case 0x8000: return gen12(d, p);
    case 0x20000: return gen3(d, p);
    case 0x80000: return gen4(d, p) == 0 ? 0 : gen5(d, p);
    }
    return gen67(d, taille, p);
}
