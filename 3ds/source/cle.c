#include <stdio.h>
#include "cle.h"

static int alnum_maj(unsigned char c)
{
    return (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9');
}

/* 4 caractères [A-Z0-9] à partir de p. */
static int code4(const unsigned char *p)
{
    return alnum_maj(p[0]) && alnum_maj(p[1]) && alnum_maj(p[2]) && alnum_maj(p[3]);
}

int cle_rom(systeme_t s, const unsigned char *e, char cle[CLE_MAX])
{
    char titre[16];
    int i;

    switch (s) {
    case SYS_DS:
        if (!code4(e + 0x0C)) return -1;
        snprintf(cle, CLE_MAX, "%.4s", (const char *)e + 0x0C);
        return 0;
    case SYS_GBA:
        if (!code4(e + 0xAC)) return -1;
        snprintf(cle, CLE_MAX, "GBA-%.4s", (const char *)e + 0xAC);
        return 0;
    case SYS_GB:
        /* titre 0x134-0x142 jusqu'au premier 0x00, en majuscules, autre caractère -> '_' */
        for (i = 0; i < 15 && e[0x134 + i]; i++) {
            unsigned char c = e[0x134 + i];
            if (c >= 'a' && c <= 'z') c -= 'a' - 'A';
            titre[i] = alnum_maj(c) ? (char)c : '_';
        }
        titre[i] = '\0';
        if (!i) return -1;
        snprintf(cle, CLE_MAX, "GB-%s-%02X%02X", titre, e[0x14E], e[0x14F]);
        return 0;
    default:
        return -1;
    }
}

void cle_3ds(unsigned long long title_id, char cle[CLE_MAX])
{
    snprintf(cle, CLE_MAX, "3DS-%016llX", title_id);
}

size_t sram_taille(systeme_t s, size_t t)
{
    size_t n = t - t % 1024;
    if ((s == SYS_GB || s == SYS_GBA) && n > 0 && n < t) return n;
    return t;
}

int sauvegarde_vide(systeme_t s, const unsigned char *o, size_t t)
{
    size_t i;
    if (s == SYS_3DS) return 0;
    if (!t) return 1;
    if (o[0] != 0x00 && o[0] != 0xFF) return 0;
    for (i = 1; i < t; i++)
        if (o[i] != o[0]) return 0;
    return 1;
}
