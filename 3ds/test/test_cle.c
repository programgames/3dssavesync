/* Vérifie cle_rom, cle_3ds et sram_taille (ADR 0007, 0008). Mêmes Clés que pc/sync.ps1 (test_lanceur.ps1).
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
    verifier(sram_taille(SYS_GB, 512) == 512, "GB 512 inchangé (moins de 1 Kio)");
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

    printf("%s\n", echecs ? "ECHECS" : "cle : OK");
    return echecs != 0;
}
