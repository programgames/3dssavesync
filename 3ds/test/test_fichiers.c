/* Vérifie deplacer_sauvegarde (bascule forwarder mGBA <-> .sav à côté de la ROM) : jamais de perte.
 * Compilé sur PC : gcc -std=c99 -I source source/fichiers.c test/test_fichiers.c -o /tmp/f && /tmp/f */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include "fichiers.h"

static int echecs;

static void verifier(int ok, const char *quoi)
{
    if (!ok) {
        printf("ECHEC  %s\n", quoi);
        echecs++;
    }
}

static void ecrire(const char *chemin, const char *texte)
{
    ecrire_fichier(chemin, texte, strlen(texte));
}

/* 1 si le fichier contient exactement texte. */
static int contient(const char *chemin, const char *texte)
{
    unsigned char *o;
    size_t n;
    int ok;
    if (lire_fichier(chemin, &o, &n)) return 0;
    ok = n == strlen(texte) && !memcmp(o, texte, n);
    free(o);
    return ok;
}

static int existe(const char *chemin)
{
    FILE *f = fopen(chemin, "rb");
    if (f) fclose(f);
    return f != NULL;
}

int main(void)
{
    const char *src = "test-src.sav", *dst = "test-dst.sav", *copie = "test-copie.sav";
    remove(src), remove(dst), remove(copie);

    ecrire(src, "partie forwarder");
    ecrire(dst, "ancienne partie rom");
    verifier(deplacer_sauvegarde(src, dst, copie, 1) == 0, "vers la ROM : fait");
    verifier(contient(dst, "partie forwarder"), "vers la ROM : la ROM a la partie du forwarder");
    verifier(contient(copie, "ancienne partie rom"), "vers la ROM : copie de l'ancien .sav");
    verifier(!existe(src), "vers la ROM : le fichier du forwarder est retiré");

    remove(copie);
    ecrire(src, "partie rom");
    verifier(deplacer_sauvegarde(src, dst, copie, 0) == 0, "vers le forwarder : fait");
    verifier(contient(dst, "partie rom") && contient(src, "partie rom"), "vers le forwarder : les deux fichiers ont la partie");
    verifier(contient(copie, "partie forwarder"), "vers le forwarder : copie de l'ancien fichier");

    remove(copie);
    remove(dst);
    verifier(deplacer_sauvegarde(src, dst, copie, 1) == 0 && contient(dst, "partie rom") && !existe(copie),
             "cible absente : pas de copie nécessaire");

    verifier(deplacer_sauvegarde("absent.sav", dst, copie, 1) != 0 && contient(dst, "partie rom"),
             "source absente : refus, rien ne change");

    ecrire(src, "nouvelle");
    verifier(deplacer_sauvegarde(src, dst, "dossier-absent/copie.sav", 1) != 0, "copie impossible : refus");
    verifier(contient(dst, "partie rom") && contient(src, "nouvelle"), "copie impossible : rien ne change");

    /* base.ini, config.ini : remplacés par un .tmp vérifié */
    verifier(remplacer_fichier(dst, "IRBF=ab\n", 8) == 0 && contient(dst, "IRBF=ab\n") && !existe("test-dst.sav.tmp"),
             "remplacement : fait, sans .tmp restant");
    verifier(remplacer_fichier("dossier-absent/base.ini", "x", 1) != 0, "remplacement impossible : échec signalé");
    mkdir("test-dst.sav.tmp", 0777); /* .tmp impossible à écrire */
    verifier(remplacer_fichier(dst, "nouveau", 7) == -1 && contient(dst, "IRBF=ab\n"), ".tmp impossible : -1, l'ancien reste intact");
    remove(src), remove(dst), remove(copie), remove("test-dst.sav.tmp");
    /* Renommage impossible (cible = dossier non vide) : -2, le .tmp vérifié reste */
    mkdir("test-dossier.sav", 0777);
    ecrire("test-dossier.sav/x", "x");
    verifier(remplacer_fichier("test-dossier.sav", "recu", 4) == -2 && contient("test-dossier.sav.tmp", "recu"), "renommage impossible : -2, .tmp gardé");
    remove("test-dossier.sav.tmp"), remove("test-dossier.sav/x"), remove("test-dossier.sav");

    /* Mise à jour du .3dsx : l'ancien devient .bak, le nouveau prend sa place, puis .bak est retiré */
    remove("test.3dsx.bak");
    ecrire("test.3dsx", "v1");
    ecrire("test.3dsx.bak", "reste d'avant");
    verifier(remplacer_programme("test.3dsx", "v2", 2) == 0 && contient("test.3dsx", "v2") && !existe("test.3dsx.tmp") && !existe("test.3dsx.bak"),
             ".3dsx : remplacé, ni .tmp ni .bak restant");
    mkdir("test.3dsx.tmp", 0777); /* .tmp impossible à écrire */
    verifier(remplacer_programme("test.3dsx", "v3", 2) == -1 && contient("test.3dsx", "v2") && !existe("test.3dsx.bak"), ".3dsx, .tmp impossible : -1, l'ancien en place");
    remove("test.3dsx.tmp"), remove("test.3dsx");
    ecrire("test.3dsx.bak", "seule copie"); /* coupure entre les deux renommages */
    verifier(remplacer_programme("test.3dsx", "v3", 2) == -1 && contient("test.3dsx.bak", "seule copie") && !existe("test.3dsx.tmp"),
             ".3dsx absent : -1, .bak (seule copie) jamais retiré");
    remove("test.3dsx.bak");
    verifier(lance_depuis_sd("sdmc:/3ds/3dssavesync.3dsx") && lance_depuis_sd("sdmc:/3ds/apps/Sync.3DSX"), "Homebrew Launcher : .3dsx de la carte SD");
    verifier(!lance_depuis_sd("3dslink:/3dssavesync.3dsx") && !lance_depuis_sd("") && !lance_depuis_sd("sdmc:/.3dsx") &&
                 !lance_depuis_sd("sdmc:/3ds/3dssavesync.3dsx.tmp"),
             "3dslink, argv vide, chemin incomplet : refusé");

    /* Copie de sécurité relue (recuperer) */
    verifier(ecrire_relu(copie, "partie", 6) == 0 && contient(copie, "partie"), "écriture relue : faite");
    verifier(ecrire_relu("dossier-absent/copie.sav", "partie", 6) != 0, "écriture relue impossible : échec signalé");
    remove(copie);

    /* Deux ROM, un fichier de Sauvegarde : Doublon (forwarder mGBA nommé comme la ROM, quel que soit le dossier) */
    verifier(meme_sauvegarde(SYS_GBA, "Pokemon", "sdmc:/roms/gba/USA/Pokemon.sav", SYS_GBA, "Pokemon", "sdmc:/roms/gba/EUR/Pokemon.sav"),
             "GBA, même nom dans deux dossiers : forwarder partagé");
    verifier(meme_sauvegarde(SYS_GB, "Tetris", "sdmc:/roms/gb/Tetris.sav", SYS_GBA, "TETRIS", "sdmc:/roms/gba/TETRIS.sav"),
             "GB et GBA de même nom (casse ignorée) : forwarder partagé");
    verifier(meme_sauvegarde(SYS_DS, "A", "sdmc:/saves/A.sav", SYS_DS, "a", "sdmc:/saves/a.sav"), "DS, même .sav");
    verifier(!meme_sauvegarde(SYS_DS, "Pokemon", "sdmc:/roms/nds/USA/Pokemon.sav", SYS_DS, "Pokemon", "sdmc:/roms/nds/EUR/Pokemon.sav"),
             "DS, même nom dans deux dossiers à côté de la ROM : deux .sav");
    verifier(!meme_sauvegarde(SYS_DS, "Pokemon", "sdmc:/roms/nds/Pokemon.sav", SYS_GBA, "Pokemon", "sdmc:/roms/gba/Pokemon.sav"),
             "DS et GBA de même nom : pas de forwarder pour la DS");

    /* Sauvegarde d'une ROM en sous-dossier (ADR 0015) */
    {
        char c[64];
        const char *r = "sdmc:/roms/nds", *sous = "sdmc:/roms/nds/Pokemon";
        verifier(!chemin_sauvegarde(r, "sdmc:/roms/nds/saves", r, "X", c, sizeof c) && !strcmp(c, "sdmc:/roms/nds/saves/X.sav"), "racine, saves de TWiLight");
        verifier(!chemin_sauvegarde(r, "sdmc:/roms/nds/saves", sous, "X", c, sizeof c) && !strcmp(c, "sdmc:/roms/nds/Pokemon/saves/X.sav"),
                 "sous-dossier, saves de TWiLight : saves du sous-dossier");
        verifier(!chemin_sauvegarde(r, "", sous, "X", c, sizeof c) && !strcmp(c, "sdmc:/roms/nds/Pokemon/X.sav"), "sous-dossier, à côté de la ROM");
        verifier(!chemin_sauvegarde(r, r, sous, "X", c, sizeof c) && !strcmp(c, "sdmc:/roms/nds/Pokemon/X.sav"), "saves = roms : à côté de la ROM");
        verifier(!chemin_sauvegarde("sdmc:/roms/gba", "sdmc:/3ds/open_agb_firm/saves", "sdmc:/roms/gba/Hacks", "X", c, sizeof c) &&
                     !strcmp(c, "sdmc:/3ds/open_agb_firm/saves/X.sav"), "autre dossier (open_agb_firm) : gardé");
        verifier(chemin_sauvegarde(r, "", sous, "un nom bien trop long pour ce petit tampon", c, sizeof c) != 0, "chemin trop long : refusé");
    }
    /* Copie de sécurité : jamais d'écrasement d'une copie de la même seconde */
    {
        char c[64];
        mkdir("test-copies", 0777);
        verifier(!chemin_copie("test-copies", "Pokemon", "20261103-101500", "sav", c, sizeof c) && !strcmp(c, "test-copies/Pokemon-20261103-101500.sav"),
                 "première copie : nom-date.ext");
        ecrire("test-copies/Pokemon-20261103-101500.sav", "a");
        verifier(!chemin_copie("test-copies", "Pokemon", "20261103-101500", "sav", c, sizeof c) && !strcmp(c, "test-copies/Pokemon-20261103-101500-1.sav"),
                 "copie de la même seconde : -1");
        ecrire("test-copies/Pokemon-20261103-101500-1.sav", "b");
        verifier(!chemin_copie("test-copies", "Pokemon", "20261103-101500", "main", c, sizeof c) && !strcmp(c, "test-copies/Pokemon-20261103-101500.main"),
                 "autre extension : pas de collision");
        verifier(!chemin_copie("test-copies", "Pokemon", "20261103-101500", "sav", c, sizeof c) && !strcmp(c, "test-copies/Pokemon-20261103-101500-2.sav"),
                 "troisième copie : -2");
        verifier(chemin_copie("test-copies", "un nom bien trop long pour ce petit tampon", "20261103-101500", "sav", c, sizeof c) != 0,
                 "chemin trop long : refusé");
        verifier(contient("test-copies/Pokemon-20261103-101500.sav", "a") && contient("test-copies/Pokemon-20261103-101500-1.sav", "b"),
                 "copies existantes intactes");
        remove("test-copies/Pokemon-20261103-101500.sav"), remove("test-copies/Pokemon-20261103-101500-1.sav"), remove("test-copies");
    }
    printf("%s\n", echecs ? "ECHECS" : "fichiers : OK");
    return echecs != 0;
}
