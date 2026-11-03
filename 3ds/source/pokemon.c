/* Play time and trainer of the Pokémon games, from Red/Blue to Ultra Moon. Offsets and character tables from PKHeX
 * (docs/notes/research-pokemon-temps-de-jeu.md); checked on real saves. The game is recognised by its size,
 * then by a checksum or a signature: another game of the same size is not taken for a Pokémon game. */
#include "pokemon.h"

static unsigned u16le(const unsigned char *d) { return d[0] | d[1] << 8; }
static unsigned long u32le(const unsigned char *d) { return d[0] | d[1] << 8 | (unsigned long)d[2] << 16 | (unsigned long)d[3] << 24; }

/* Appends a character (Basic Multilingual Plane code point) to the name, in UTF-8. */
static void append_char(progress_t *p, size_t *j, unsigned c)
{
    if (*j + 4 > sizeof p->trainer) return;
    if (c < 0x80) p->trainer[(*j)++] = (char)c;
    else if (c < 0x800) p->trainer[(*j)++] = (char)(0xC0 | c >> 6), p->trainer[(*j)++] = (char)(0x80 | (c & 0x3F));
    else p->trainer[(*j)++] = (char)(0xE0 | c >> 12), p->trainer[(*j)++] = (char)(0x80 | (c >> 6 & 0x3F)), p->trainer[(*j)++] = (char)(0x80 | (c & 0x3F));
    p->trainer[*j] = '\0';
}

/* Code -> character in a table of pairs; '?' if missing. */
static unsigned lookup(unsigned c, const unsigned short (*t)[2], size_t n)
{
    size_t i;
    for (i = 0; i < n; i++)
        if (t[i][0] == c) return t[i][1];
    return '?';
}

/* Gen 1 and 2 (StringConverter1, StringConverter2 TableFRE): letters, digits, French accents. */
static unsigned char_g12(unsigned c, int gen2)
{
    static const unsigned short common[][2] = { { 0x7F, ' ' }, { 0xBA, 0xE0 }, { 0xBB, 0xE8 }, { 0xBC, 0xE9 }, { 0xBD, 0xF9 } };
    static const unsigned short g1[][2] = { { 0xBE, 0xC0 }, { 0xC6, 0xC8 }, { 0xC7, 0xC9 } };
    static const unsigned short g2[][2] = { { 0xBF, 0xE7 }, { 0xC6, 0xEB }, { 0xC7, 0xEF }, { 0xC8, 0xE2 }, { 0xC9, 0xF4 }, { 0xCA, 0xFB }, { 0xCB, 0xEA }, { 0xCC, 0xEE } };
    unsigned r;
    if (c >= 0x80 && c <= 0x99) return 'A' + c - 0x80;
    if (c >= 0xA0 && c <= 0xB9) return 'a' + c - 0xA0;
    if (c >= 0xF6) return '0' + c - 0xF6;
    if ((r = lookup(c, common, sizeof common / sizeof *common)) != '?') return r;
    return gen2 ? lookup(c, g2, sizeof g2 / sizeof *g2) : lookup(c, g1, sizeof g1 / sizeof *g1);
}

/* Gen 3 (StringConverter3, G3_EN). */
static unsigned char_g3(unsigned c)
{
    static const unsigned short t[][2] = { { 0x00, ' ' }, { 0x01, 0xC0 }, { 0x04, 0xC7 }, { 0x05, 0xC8 }, { 0x06, 0xC9 }, { 0x16, 0xE0 }, { 0x19, 0xE7 }, { 0x1A, 0xE8 },
                                           { 0x1B, 0xE9 }, { 0x1C, 0xEA }, { 0x1D, 0xEB }, { 0x20, 0xEE }, { 0x21, 0xEF }, { 0x24, 0xF4 }, { 0x26, 0xF9 }, { 0x28, 0xFB } };
    if (c >= 0xBB && c <= 0xD4) return 'A' + c - 0xBB;
    if (c >= 0xD5 && c <= 0xEE) return 'a' + c - 0xD5;
    if (c >= 0xA1 && c <= 0xAA) return '0' + c - 0xA1;
    return lookup(c, t, sizeof t / sizeof *t);
}

/* Gen 4 (StringConverter4, TableINT). */
static unsigned char_g4(unsigned c)
{
    static const unsigned short t[][2] = { { 0x1DE, ' ' }, { 0x15F, 0xC0 }, { 0x166, 0xC7 }, { 0x167, 0xC8 }, { 0x168, 0xC9 }, { 0x17F, 0xE0 }, { 0x181, 0xE2 }, { 0x186, 0xE7 },
                                           { 0x187, 0xE8 }, { 0x188, 0xE9 }, { 0x189, 0xEA }, { 0x18A, 0xEB }, { 0x18D, 0xEE }, { 0x18E, 0xEF }, { 0x193, 0xF4 }, { 0x198, 0xF9 }, { 0x19A, 0xFB } };
    if (c >= 0x121 && c <= 0x12A) return '0' + c - 0x121;
    if (c >= 0x12B && c <= 0x144) return 'A' + c - 0x12B;
    if (c >= 0x145 && c <= 0x15E) return 'a' + c - 0x145;
    return lookup(c, t, sizeof t / sizeof *t);
}

/* Name in bytes (Gen 1 to 3): n characters at most, up to the end code. gen: 1, 2 or 3. */
static void name8(progress_t *p, const unsigned char *d, size_t n, unsigned end, int gen)
{
    size_t i, j = 0;
    for (i = 0; i < n && d[i] != end; i++) append_char(p, &j, gen == 3 ? char_g3(d[i]) : char_g12(d[i], gen == 2));
}

/* Name in 16-bit words (Gen 4: own table; Gen 5 to 7: UTF-16), up to 0 or 0xFFFF. */
static void name16(progress_t *p, const unsigned char *d, size_t n, int gen4)
{
    size_t i, j = 0;
    for (i = 0; i < n; i++) {
        unsigned c = u16le(d + 2 * i);
        if (!c || c == 0xFFFF) break;
        append_char(p, &j, gen4 ? char_g4(c) : c);
    }
}

/* Gen 1 and 2 (32 KiB): recognised by their checksum (SAV1, SAV2: Gold/Silver then Crystal, INT versions). */
static int gen12(const unsigned char *d, progress_t *p)
{
    static const struct { unsigned end, checksum, hours; } g2[] = { { 0x2D68, 0x2D69, 0x2053 }, { 0x2B82, 0x2D0D, 0x2052 } };
    unsigned s = 0, i, k;
    for (i = 0x2598; i < 0x3523; i++) s += d[i];
    if ((~s & 0xFF) == d[0x3523]) {
        p->hours = d[0x2CED];
        p->minutes = d[0x2CEF];
        name8(p, d + 0x2598, 7, 0x50, 1);
        return 0;
    }
    for (k = 0; k < 2; k++) {
        for (s = 0, i = 0x2009; i <= g2[k].end; i++) s += d[i];
        if (s && (s & 0xFFFF) == u16le(d + g2[k].checksum)) { /* zero sum: blank file, not a game in progress */
            p->hours = d[g2[k].hours] << 8 | d[g2[k].hours + 1]; /* big-endian */
            p->minutes = d[g2[k].hours + 2];
            name8(p, d + 0x200B, 7, 0x50, 2);
            return 0;
        }
    }
    return -1;
}

/* Gen 3 (128 KiB): two halves of 14 sectors; the valid half with the highest counter; section 0: name and time. */
static int gen3(const unsigned char *d, progress_t *p)
{
    long s0 = -1;
    unsigned long best = 0;
    int m, k;
    for (m = 0; m < 2; m++) {
        int seen_sectors = 0;
        long zero = -1;
        for (k = 0; k < 14; k++) {
            unsigned id = u16le(d + m * 0xE000 + k * 0x1000 + 0xFF4);
            if (id >= 14 || seen_sectors & 1 << id) break;
            seen_sectors |= 1 << id;
            if (!id) zero = m * 0xE000 + k * 0x1000;
        }
        if (seen_sectors == 0x3FFF && (s0 < 0 || u32le(d + zero + 0xFFC) > best)) {
            s0 = zero;
            best = u32le(d + zero + 0xFFC);
        }
    }
    if (s0 < 0) return -1;
    p->hours = u16le(d + s0 + 0x0E);
    p->minutes = d[s0 + 0x10];
    name8(p, d + s0, 7, 0xFF, 3);
    return 0;
}

/* Gen 4 (512 KiB): footer of the general block of partition 1 (size, signature); active block by the counters. */
static int gen4(const unsigned char *d, progress_t *p)
{
    static const struct { unsigned long g; unsigned trainer; } games[] = { { 0xC100, 0x64 }, { 0xCF2C, 0x68 }, { 0xF628, 0x64 } }; /* DP, Pt, HGSS */
    size_t i;
    for (i = 0; i < 3; i++) {
        unsigned long g = games[i].g, b;
        const unsigned char *p0 = d + g - 0x14, *p1 = d + 0x40000 + g - 0x14;
        unsigned long sig = u32le(p1 + 0x0C);
        if (u32le(p1 + 0x08) != g || (sig != 0x20060623 && sig != 0x20070903)) continue;
        /* ponytail: highest major then minor counter; PKHeX also handles 0xFFFFFFFE */
        b = u32le(p0) == 0xFFFFFFFF || (u32le(p1) != 0xFFFFFFFF && (u32le(p1) > u32le(p0) || (u32le(p1) == u32le(p0) && u32le(p1 + 4) > u32le(p0 + 4)))) ? 0x40000 : 0;
        p->hours = u16le(d + b + games[i].trainer + 0x22);
        p->minutes = d[b + games[i].trainer + 0x24];
        name16(p, d + b + games[i].trainer, 8, 1);
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

/* Gen 5 (512 KiB): valid checksum block (Black/White or Black 2/White 2); trainer data at 0x19400. */
static int gen5(const unsigned char *d, progress_t *p)
{
    if (crc16_ccitt(d + 0x23F00, 0x8C) != u16le(d + 0x23F9A) && crc16_ccitt(d + 0x25F00, 0x94) != u16le(d + 0x25FA2)) return -1;
    p->hours = u16le(d + 0x19424);
    p->minutes = d[0x19426];
    name16(p, d + 0x19404, 8, 0);
    return 0;
}

/* Gen 6 and 7 (main of the 3DS games): recognised by size (SaveUtil.cs: SIZE_G6XY, SIZE_G6ORAS, SIZE_G7SM, SIZE_G7USUM). */
static int gen67(const unsigned char *d, size_t size, progress_t *p)
{
    static const struct { size_t size, play_time, trainer; } f[] = {
        { 0x65600, 0x01800, 0x14048 }, /* X/Y */
        { 0x76000, 0x01800, 0x14048 }, /* Omega Ruby/Alpha Sapphire */
        { 0x6BE00, 0x40C00, 0x01238 }, /* Sun/Moon */
        { 0x6CC00, 0x41000, 0x01438 }, /* Ultra Sun/Ultra Moon */
    };
    size_t i;
    for (i = 0; i < 4 && f[i].size != size; i++);
    if (i == 4) return -1;
    p->hours = u16le(d + f[i].play_time);
    p->minutes = d[f[i].play_time + 2];
    name16(p, d + f[i].trainer, 13, 0);
    return 0;
}

int pokemon_progress(const unsigned char *d, size_t size, progress_t *p)
{
    int r;
    p->hours = p->minutes = 0;
    p->trainer[0] = '\0';
    switch (size) {
    case 0x8000: r = gen12(d, p); break;
    case 0x20000: r = gen3(d, p); break;
    case 0x80000: r = gen4(d, p) == 0 ? 0 : gen5(d, p); break;
    default: r = gen67(d, size, p);
    }
    /* The games count up to 999 h 59: beyond that, another file passed the checksum by chance
     * ("196 h 246" on some GB save). */
    if (r == 0 && (p->minutes > 59 || p->hours > 999)) {
        p->hours = p->minutes = 0;
        p->trainer[0] = '\0';
        r = -1;
    }
    return r;
}
