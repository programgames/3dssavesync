#include <stdio.h>
#include <string.h>
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

static int hex4(const char *p)
{
    int i;
    for (i = 0; i < 4; i++)
        if (!((p[i] >= '0' && p[i] <= '9') || (p[i] >= 'A' && p[i] <= 'F'))) return 0;
    return 1;
}

int cle_alias(systeme_t s, const char *v, char cle[CLE_MAX])
{
    char t[CLE_MAX];
    size_t n = 0, i;
    int ok = 0;

    while (*v == ' ' || *v == '\t') v++;
    for (; v[n] && n < CLE_MAX - 1; n++) t[n] = v[n] >= 'a' && v[n] <= 'z' ? (char)(v[n] - ('a' - 'A')) : v[n];
    if (v[n]) return -1; /* trop long pour une Clé */
    while (n && (t[n - 1] == ' ' || t[n - 1] == '\t')) n--;
    t[n] = '\0';
    switch (s) {
    case SYS_DS:
        ok = n == 4 && code4((const unsigned char *)t);
        break;
    case SYS_GBA:
        ok = n == 8 && !strncmp(t, "GBA-", 4) && code4((const unsigned char *)t + 4);
        break;
    case SYS_GB: /* GB-<titre de 1 à 15 [A-Z0-9_]>-<4 hexa> */
        ok = n >= 9 && n <= 23 && !strncmp(t, "GB-", 3) && t[n - 5] == '-' && hex4(t + n - 4);
        for (i = 3; ok && i < n - 5; i++) ok = alnum_maj((unsigned char)t[i]) || t[i] == '_';
        break;
    default:
        break;
    }
    if (!ok) return -1;
    memcpy(cle, t, n + 1);
    return 0;
}

void cle_3ds(unsigned long long title_id, char cle[CLE_MAX])
{
    snprintf(cle, CLE_MAX, "3DS-%016llX", title_id);
}

size_t sram_taille(systeme_t s, size_t t)
{
    size_t r = t % 512; /* Trailer d'horloge : 16, 44 ou 48 octets après la SRAM (ADR 0010) */
    if ((s == SYS_GB || s == SYS_GBA) && t > r && (r == 16 || r == 44 || r == 48)) return t - r;
    return t;
}

int eeprom_inverser(unsigned char *o, size_t t)
{
    size_t i, k;
    if (t != 512 && t != 8192) return 0; /* SRAM 32 Kio, Flash 64 ou 128 Kio : pas une EEPROM */
    for (i = 0; i < t; i += 8)
        for (k = 0; k < 4; k++) {
            unsigned char c = o[i + k];
            o[i + k] = o[i + 7 - k];
            o[i + 7 - k] = c;
        }
    return 1;
}

int rom_mbc2(const unsigned char *e) { return e[0x147] == 0x05 || e[0x147] == 0x06; }

/* docs/research-eeprom-mbc2.md : mGBA, deux valeurs par octet, adresse paire dans le quartet bas ; SameBoy, VBA-M, BGB,
 * une valeur par octet (quartet haut sans valeur) ; GameYob, les 512 premiers octets d'un fichier de 8 Kio. */
int mbc2_normaliser(const unsigned char *f, size_t t, unsigned char s[MBC2_SERVEUR])
{
    size_t i;
    if (t != 256 && t != 512 && t != 8192) return -1;
    for (i = 0; i < MBC2_SERVEUR; i++) s[i] = (t == 256 ? f[i / 2] >> (i % 2 * 4) : f[i]) & 0x0F;
    return 0;
}

int mbc2_fichier(const unsigned char s[MBC2_SERVEUR], unsigned char *f, size_t t)
{
    size_t i;
    if (t != 256 && t != 512 && t != 8192) return -1;
    memset(f, 0, t);
    for (i = 0; i < MBC2_SERVEUR; i++) {
        if (t == 256) f[i / 2] |= (unsigned char)((s[i] & 0x0F) << (i % 2 * 4));
        else f[i] = (unsigned char)((s[i] & 0x0F) | (t == 512 ? 0xF0 : 0)); /* VBA-M écrit 0xF, GameYob 0 */
    }
    return 0;
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
