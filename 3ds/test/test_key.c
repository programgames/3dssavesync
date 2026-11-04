/* Checks rom_key, key_3ds, sram_size, EEPROM and MBC2 (ADR 0007, 0008, 0010, 0012). Same Keys as the PC program (client/).
 * Built on the PC: gcc -I source source/key.c test/test_key.c -o test_key && ./test_key */
#include <stdio.h>
#include <string.h>
#include "key.h"

static int failures;

static void check(int ok, const char *what)
{
    if (!ok) {
        printf("ECHEC  %s\n", what);
        failures++;
    }
}

static void check_rom(system_t s, int offset, const char *text, int checksum, const char *expected)
{
    unsigned char e[0x200] = { 0 };
    char key[KEY_MAX] = "";
    memcpy(e + offset, text, strlen(text));
    e[0x14E] = (unsigned char)(checksum >> 8);
    e[0x14F] = (unsigned char)checksum;
    if (expected) check(rom_key(s, e, key) == 0 && !strcmp(key, expected), expected);
    else check(rom_key(s, e, key) != 0, text);
}

int main(void)
{
    char key[KEY_MAX];

    check_rom(SYS_GB, 0x134, "POKEMON RED", 0x91E6, "GB-POKEMON_RED-91E6");
    check_rom(SYS_GB, 0x134, "PM_CRYSTAL", 0xF2E2, "GB-PM_CRYSTAL-F2E2");
    check_rom(SYS_GB, 0x134, "pokemon yel", 0x0001, "GB-POKEMON_YEL-0001");
    check_rom(SYS_GB, 0x134, "POKEMON_GLDAAUF", 0x6FC6, "GB-POKEMON_GLDAAUF-6FC6"); /* 15 characters, no 0x00 */
    check_rom(SYS_GB, 0x134, "Pok\xe9mon\x99", 0x91E6, "GB-POK_MON_-91E6"); /* bytes >= 0x80: same Key as client/profiles_test.go */
    check_rom(SYS_GB, 0x134, "", 0x1234, NULL);
    check_rom(SYS_GBA, 0xAC, "BPEF", 0, "GBA-BPEF");
    check_rom(SYS_GBA, 0xAC, "bp f", 0, NULL);
    check_rom(SYS_DS, 0x0C, "ADAF", 0, "ADAF");
    check_rom(SYS_DS, 0x0C, "####", 0, NULL);

    key_3ds(0x00040000001B5000ULL, key);
    check(!strcmp(key, "3DS-00040000001B5000"), "3DS-00040000001B5000");

    /* Alias (ADR 0014): same cases as TestAliasKey in client/cases_test.go */
    check(alias_key(SYS_DS, "0rad", key) == 0 && !strcmp(key, "0RAD"), "alias DS 0rad -> 0RAD");
    check(alias_key(SYS_GBA, " GBA-0RAD ", key) == 0 && !strcmp(key, "GBA-0RAD"), "alias GBA avec espaces");
    check(alias_key(SYS_GB, "GB-MONHACK-0001", key) == 0 && !strcmp(key, "GB-MONHACK-0001"), "alias GB");
    check(alias_key(SYS_GB, "GB-POKEMON_GLDAAUF-6FC6", key) == 0, "alias GB, titre de 15 caractères");
    check(alias_key(SYS_GBA, "GBA-BPRE-RADICALRED", key) != 0, "alias GBA trop long : refusé");
    check(alias_key(SYS_GBA, "0RAD", key) != 0, "alias d'un autre Système : refusé");
    check(alias_key(SYS_GB, "GB-MON-HACK-0001", key) != 0, "alias GB avec un tiret dans le titre : refusé");
    check(alias_key(SYS_GB, "GB--0001", key) != 0, "alias GB sans titre : refusé");
    check(alias_key(SYS_GB, "GB-MONHACK-00G1", key) != 0, "alias GB, somme non hexadécimale : refusée");
    check(alias_key(SYS_DS, "", key) != 0, "alias vide : refusé");
    check(alias_key(SYS_3DS, "3DS-00040000001B5000", key) != 0, "pas d'alias 3DS");

    check(sram_size(SYS_GB, 32816) == 32768, "GB 32816 -> 32768 (trailer mGBA 48 o)");
    check(sram_size(SYS_GB, 32768) == 32768, "GB 32768 inchangé");
    check(sram_size(SYS_GBA, 131088) == 131072, "GBA 131088 -> 131072 (trailer mGBA 16 o)");
    check(sram_size(SYS_GB, 32812) == 32768, "GB 32812 -> 32768 (trailer VBA 44 o)");
    check(sram_size(SYS_GB, 512) == 512, "GB 512 inchangé (MBC2)");
    check(sram_size(SYS_GBA, 528) == 512, "GBA 528 -> 512 (EEPROM 4 Kbit + trailer 16 o)");
    check(sram_size(SYS_GB, 560) == 512, "GB 560 -> 512 (trailer 48 o)");
    check(sram_size(SYS_GBA, 8208) == 8192, "GBA 8208 -> 8192 (EEPROM 64 Kbit + trailer 16 o)");
    check(sram_size(SYS_GB, 1536) == 1536, "GB 1536 inchangé (pas un trailer)");
    check(sram_size(SYS_GB, 48) == 48, "GB 48 inchangé (pas de SRAM)");
    check(sram_size(SYS_DS, 524298) == 524298, "DS jamais coupé");

    {
        static unsigned char s[1024];
        check(is_blank_save(SYS_GB, s, sizeof s), "GB que des 0x00 : vierge");
        memset(s, 0xFF, sizeof s);
        check(is_blank_save(SYS_DS, s, sizeof s), "DS que des 0xFF : vierge");
        check(!is_blank_save(SYS_3DS, s, sizeof s), "3DS jamais vierge");
        s[700] = 0;
        check(!is_blank_save(SYS_GBA, s, sizeof s), "GBA avec un octet différent : pas vierge");
        memset(s, 0x42, sizeof s);
        check(!is_blank_save(SYS_DS, s, sizeof s), "DS que des 0x42 : pas vierge");
        check(is_blank_save(SYS_DS, s, 0), "fichier vide : vierge");
    }

    { /* EEPROM (ADR 0012): open_agb_firm 01..08 = mGBA 08..01 */
        static unsigned char e[8192];
        static const unsigned char agb[8] = { 1, 2, 3, 4, 5, 6, 7, 8 }, mgba[8] = { 8, 7, 6, 5, 4, 3, 2, 1 };
        memcpy(e, agb, 8);
        memcpy(e + 8184, agb, 8);
        check(eeprom_swap(e, 512) && !memcmp(e, mgba, 8), "EEPROM 512 o : bloc inversé");
        check(!memcmp(e + 8184, agb, 8), "EEPROM 512 o : rien après 512 o");
        check(eeprom_swap(e, 512) && !memcmp(e, agb, 8), "EEPROM : son propre inverse");
        check(eeprom_swap(e, 8192) && !memcmp(e + 8184, mgba, 8), "EEPROM 8 Kio : dernier bloc inversé");
        check(!eeprom_swap(e, 32768), "SRAM 32 Kio : pas une EEPROM");
    }

    { /* MBC2 (ADR 0012): server form = 512 B, high nibble 0 */
        static unsigned char f[8192], s[MBC2_SERVER], g[8192];
        unsigned char e[0x200] = { 0 };
        e[0x147] = 0x06;
        check(rom_mbc2(e), "0x147 = 0x06 : MBC2 + pile");
        e[0x147] = 0x03;
        check(!rom_mbc2(e), "0x147 = 0x03 : MBC1, pas MBC2");

        f[0] = 0x21; f[255] = 0xA5; /* mGBA: even address in the low nibble */
        check(!mbc2_normalize(f, 256, s) && s[0] == 1 && s[1] == 2 && s[510] == 5 && s[511] == 0xA, "MBC2 mGBA 256 o -> serveur");
        check(!mbc2_to_file(s, g, 256) && !memcmp(f, g, 256), "MBC2 serveur -> mGBA 256 o");
        check(!mbc2_to_file(s, g, 512) && g[0] == 0xF1 && g[1] == 0xF2 && g[511] == 0xFA, "MBC2 serveur -> 512 o (quartet haut 0xF, VBA-M)");
        check(!mbc2_normalize(g, 512, f) && !memcmp(f, s, MBC2_SERVER), "MBC2 512 o -> serveur");
        check(!mbc2_to_file(s, g, 8192) && g[0] == 1 && g[511] == 0xA && g[512] == 0 && g[8191] == 0, "MBC2 serveur -> GameYob 8 Kio");
        check(!mbc2_normalize(g, 8192, f) && !memcmp(f, s, MBC2_SERVER), "MBC2 GameYob 8 Kio -> serveur");
        check(mbc2_normalize(g, 1024, f) && mbc2_to_file(s, g, 32768), "MBC2 : autre taille refusée");
    }

    printf("%s\n", failures ? "ECHECS" : "cle : OK");
    return failures != 0;
}
