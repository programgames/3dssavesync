#include <stdio.h>
#include <string.h>
#include "key.h"

static int is_upper_alnum(unsigned char c)
{
    return (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9');
}

/* 4 characters [A-Z0-9] from p. */
static int code4(const unsigned char *p)
{
    return is_upper_alnum(p[0]) && is_upper_alnum(p[1]) && is_upper_alnum(p[2]) && is_upper_alnum(p[3]);
}

int rom_key(system_t s, const unsigned char *e, char key[KEY_MAX])
{
    char title[16];
    int i;

    switch (s) {
    case SYS_DS:
        if (!code4(e + 0x0C)) return -1;
        snprintf(key, KEY_MAX, "%.4s", (const char *)e + 0x0C);
        return 0;
    case SYS_GBA:
        if (!code4(e + 0xAC)) return -1;
        snprintf(key, KEY_MAX, "GBA-%.4s", (const char *)e + 0xAC);
        return 0;
    case SYS_GB:
        /* title 0x134-0x142 up to the first 0x00, uppercased, any other character -> '_' */
        for (i = 0; i < 15 && e[0x134 + i]; i++) {
            unsigned char c = e[0x134 + i];
            if (c >= 'a' && c <= 'z') c -= 'a' - 'A';
            title[i] = is_upper_alnum(c) ? (char)c : '_';
        }
        title[i] = '\0';
        if (!i) return -1;
        snprintf(key, KEY_MAX, "GB-%s-%02X%02X", title, e[0x14E], e[0x14F]);
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

int alias_key(system_t s, const char *v, char key[KEY_MAX])
{
    char t[KEY_MAX];
    size_t n = 0, i;
    int ok = 0;

    while (*v == ' ' || *v == '\t') v++;
    for (; v[n] && n < KEY_MAX - 1; n++) t[n] = v[n] >= 'a' && v[n] <= 'z' ? (char)(v[n] - ('a' - 'A')) : v[n];
    if (v[n]) return -1; /* too long for a Key */
    while (n && (t[n - 1] == ' ' || t[n - 1] == '\t')) n--;
    t[n] = '\0';
    switch (s) {
    case SYS_DS:
        ok = n == 4 && code4((const unsigned char *)t);
        break;
    case SYS_GBA:
        ok = n == 8 && !strncmp(t, "GBA-", 4) && code4((const unsigned char *)t + 4);
        break;
    case SYS_GB: /* GB-<title of 1 to 15 [A-Z0-9_]>-<4 hex digits> */
        ok = n >= 9 && n <= 23 && !strncmp(t, "GB-", 3) && t[n - 5] == '-' && hex4(t + n - 4);
        for (i = 3; ok && i < n - 5; i++) ok = is_upper_alnum((unsigned char)t[i]) || t[i] == '_';
        break;
    default:
        break;
    }
    if (!ok) return -1;
    memcpy(key, t, n + 1);
    return 0;
}

void key_3ds(unsigned long long title_id, char key[KEY_MAX])
{
    snprintf(key, KEY_MAX, "3DS-%016llX", title_id);
}

size_t sram_size(system_t s, size_t t)
{
    size_t r = t % 512; /* Clock trailer: 16, 44 or 48 bytes after the SRAM (ADR 0010) */
    if ((s == SYS_GB || s == SYS_GBA) && t > r && (r == 16 || r == 44 || r == 48)) return t - r;
    return t;
}

int eeprom_swap(unsigned char *o, size_t t)
{
    size_t i, k;
    if (t != 512 && t != 8192) return 0; /* SRAM 32 KiB, Flash 64 or 128 KiB: not an EEPROM */
    for (i = 0; i < t; i += 8)
        for (k = 0; k < 4; k++) {
            unsigned char c = o[i + k];
            o[i + k] = o[i + 7 - k];
            o[i + 7 - k] = c;
        }
    return 1;
}

int rom_mbc2(const unsigned char *e) { return e[0x147] == 0x05 || e[0x147] == 0x06; }

/* docs/notes/research-eeprom-mbc2.md: mGBA, two values per byte, even address in the low nibble; SameBoy, VBA-M, BGB,
 * one value per byte (high nibble unused); GameYob, the first 512 bytes of an 8 KiB file. */
int mbc2_normalize(const unsigned char *f, size_t t, unsigned char s[MBC2_SERVER])
{
    size_t i;
    if (t != 256 && t != 512 && t != 8192) return -1;
    for (i = 0; i < MBC2_SERVER; i++) s[i] = (t == 256 ? f[i / 2] >> (i % 2 * 4) : f[i]) & 0x0F;
    return 0;
}

int mbc2_to_file(const unsigned char s[MBC2_SERVER], unsigned char *f, size_t t)
{
    size_t i;
    if (t != 256 && t != 512 && t != 8192) return -1;
    memset(f, 0, t);
    for (i = 0; i < MBC2_SERVER; i++) {
        if (t == 256) f[i / 2] |= (unsigned char)((s[i] & 0x0F) << (i % 2 * 4));
        else f[i] = (unsigned char)((s[i] & 0x0F) | (t == 512 ? 0xF0 : 0)); /* VBA-M writes 0xF, GameYob 0 */
    }
    return 0;
}

int is_blank_save(system_t s, const unsigned char *o, size_t t)
{
    size_t i;
    if (s == SYS_3DS) return 0;
    if (!t) return 1;
    if (o[0] != 0x00 && o[0] != 0xFF) return 0;
    for (i = 1; i < t; i++)
        if (o[i] != o[0]) return 0;
    return 1;
}
