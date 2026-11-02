/* Vérifie cle_rom, cle_3ds, sram_taille, EEPROM et MBC2 (ADR 0007, 0008, 0010, 0012). Mêmes Clés que pc/sync.ps1 (test_lanceur.ps1).
 * Compilé sur PC : gcc -I source source/cle.c test/test_cle.c -o test_cle && ./test_cle */
#include <stdio.h>
#include <string.h>
#include "cle.h"

static int echecs;

static void verifier(int ok, const char *quoi)
{
    if (!ok) {
        printf("ECHEC  %s\n", quoi);
        echecs++;
    }
}

static void cas_rom(systeme_t s, int offset, const char *texte, int somme, const char *attendu)
{
    unsigned char e[0x200] = { 0 };
    char cle[CLE_MAX] = "";
    memcpy(e + offset, texte, strlen(texte));
    e[0x14E] = (unsigned char)(somme >> 8);
    e[0x14F] = (unsigned char)somme;
    if (attendu) verifier(cle_rom(s, e, cle) == 0 && !strcmp(cle, attendu), attendu);
    else verifier(cle_rom(s, e, cle) != 0, texte);
}

int main(void)
{
    char cle[CLE_MAX];

    cas_rom(SYS_GB, 0x134, "POKEMON RED", 0x91E6, "GB-POKEMON_RED-91E6");
    cas_rom(SYS_GB, 0x134, "PM_CRYSTAL", 0xF2E2, "GB-PM_CRYSTAL-F2E2");
    cas_rom(SYS_GB, 0x134, "pokemon yel", 0x0001, "GB-POKEMON_YEL-0001");
    cas_rom(SYS_GB, 0x134, "POKEMON_GLDAAUF", 0x6FC6, "GB-POKEMON_GLDAAUF-6FC6"); /* 15 caractères, sans 0x00 */
    cas_rom(SYS_GB, 0x134, "", 0x1234, NULL);
    cas_rom(SYS_GBA, 0xAC, "BPEF", 0, "GBA-BPEF");
    cas_rom(SYS_GBA, 0xAC, "bp f", 0, NULL);
    cas_rom(SYS_DS, 0x0C, "ADAF", 0, "ADAF");
    cas_rom(SYS_DS, 0x0C, "####", 0, NULL);

    cle_3ds(0x00040000001B5000ULL, cle);
    verifier(!strcmp(cle, "3DS-00040000001B5000"), "3DS-00040000001B5000");

    verifier(sram_taille(SYS_GB, 32816) == 32768, "GB 32816 -> 32768 (trailer mGBA 48 o)");
    verifier(sram_taille(SYS_GB, 32768) == 32768, "GB 32768 inchangé");
    verifier(sram_taille(SYS_GBA, 131088) == 131072, "GBA 131088 -> 131072 (trailer mGBA 16 o)");
    verifier(sram_taille(SYS_GB, 32812) == 32768, "GB 32812 -> 32768 (trailer VBA 44 o)");
    verifier(sram_taille(SYS_GB, 512) == 512, "GB 512 inchangé (MBC2)");
    verifier(sram_taille(SYS_GBA, 528) == 512, "GBA 528 -> 512 (EEPROM 4 Kbit + trailer 16 o)");
    verifier(sram_taille(SYS_GB, 560) == 512, "GB 560 -> 512 (trailer 48 o)");
    verifier(sram_taille(SYS_GBA, 8208) == 8192, "GBA 8208 -> 8192 (EEPROM 64 Kbit + trailer 16 o)");
    verifier(sram_taille(SYS_GB, 1536) == 1536, "GB 1536 inchangé (pas un trailer)");
    verifier(sram_taille(SYS_GB, 48) == 48, "GB 48 inchangé (pas de SRAM)");
    verifier(sram_taille(SYS_DS, 524298) == 524298, "DS jamais coupé");

    {
        static unsigned char s[1024];
        verifier(sauvegarde_vide(SYS_GB, s, sizeof s), "GB que des 0x00 : vierge");
        memset(s, 0xFF, sizeof s);
        verifier(sauvegarde_vide(SYS_DS, s, sizeof s), "DS que des 0xFF : vierge");
        verifier(!sauvegarde_vide(SYS_3DS, s, sizeof s), "3DS jamais vierge");
        s[700] = 0;
        verifier(!sauvegarde_vide(SYS_GBA, s, sizeof s), "GBA avec un octet différent : pas vierge");
        memset(s, 0x42, sizeof s);
        verifier(!sauvegarde_vide(SYS_DS, s, sizeof s), "DS que des 0x42 : pas vierge");
        verifier(sauvegarde_vide(SYS_DS, s, 0), "fichier vide : vierge");
    }

    { /* EEPROM (ADR 0012) : open_agb_firm 01..08 = mGBA 08..01 */
        static unsigned char e[8192];
        static const unsigned char agb[8] = { 1, 2, 3, 4, 5, 6, 7, 8 }, mgba[8] = { 8, 7, 6, 5, 4, 3, 2, 1 };
        memcpy(e, agb, 8);
        memcpy(e + 8184, agb, 8);
        verifier(eeprom_inverser(e, 512) && !memcmp(e, mgba, 8), "EEPROM 512 o : bloc inversé");
        verifier(!memcmp(e + 8184, agb, 8), "EEPROM 512 o : rien après 512 o");
        verifier(eeprom_inverser(e, 512) && !memcmp(e, agb, 8), "EEPROM : son propre inverse");
        verifier(eeprom_inverser(e, 8192) && !memcmp(e + 8184, mgba, 8), "EEPROM 8 Kio : dernier bloc inversé");
        verifier(!eeprom_inverser(e, 32768), "SRAM 32 Kio : pas une EEPROM");
    }

    { /* MBC2 (ADR 0012) : forme du serveur = 512 o, quartet haut à 0 */
        static unsigned char f[8192], s[MBC2_SERVEUR], g[8192];
        unsigned char e[0x200] = { 0 };
        e[0x147] = 0x06;
        verifier(rom_mbc2(e), "0x147 = 0x06 : MBC2 + pile");
        e[0x147] = 0x03;
        verifier(!rom_mbc2(e), "0x147 = 0x03 : MBC1, pas MBC2");

        f[0] = 0x21; f[255] = 0xA5; /* mGBA : adresse paire dans le quartet bas */
        verifier(!mbc2_normaliser(f, 256, s) && s[0] == 1 && s[1] == 2 && s[510] == 5 && s[511] == 0xA, "MBC2 mGBA 256 o -> serveur");
        verifier(!mbc2_fichier(s, g, 256) && !memcmp(f, g, 256), "MBC2 serveur -> mGBA 256 o");
        verifier(!mbc2_fichier(s, g, 512) && g[0] == 0xF1 && g[1] == 0xF2 && g[511] == 0xFA, "MBC2 serveur -> 512 o (quartet haut 0xF, VBA-M)");
        verifier(!mbc2_normaliser(g, 512, f) && !memcmp(f, s, MBC2_SERVEUR), "MBC2 512 o -> serveur");
        verifier(!mbc2_fichier(s, g, 8192) && g[0] == 1 && g[511] == 0xA && g[512] == 0 && g[8191] == 0, "MBC2 serveur -> GameYob 8 Kio");
        verifier(!mbc2_normaliser(g, 8192, f) && !memcmp(f, s, MBC2_SERVEUR), "MBC2 GameYob 8 Kio -> serveur");
        verifier(mbc2_normaliser(g, 1024, f) && mbc2_fichier(s, g, 32768), "MBC2 : autre taille refusée");
    }

    printf("%s\n", echecs ? "ECHECS" : "cle : OK");
    return echecs != 0;
}
