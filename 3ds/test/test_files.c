/* Vérifie move_save (bascule forwarder mGBA <-> .sav à côté de la ROM) : jamais de perte.
 * Compilé sur PC : gcc -std=c99 -I source source/files.c test/test_files.c -o /tmp/f && /tmp/f */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include "files.h"

static int failures;

static void check(int ok, const char *what)
{
    if (!ok) {
        printf("ECHEC  %s\n", what);
        failures++;
    }
}

static void write_text(const char *path, const char *text)
{
    write_file(path, text, strlen(text));
}

/* 1 si le fichier contient exactement texte. */
static int contains(const char *path, const char *text)
{
    unsigned char *o;
    size_t n;
    int ok;
    if (read_file(path, &o, &n)) return 0;
    ok = n == strlen(text) && !memcmp(o, text, n);
    free(o);
    return ok;
}

static int exists(const char *path)
{
    FILE *f = fopen(path, "rb");
    if (f) fclose(f);
    return f != NULL;
}

int main(void)
{
    const char *src = "test-src.sav", *dst = "test-dst.sav", *backup = "test-copie.sav";
    remove(src), remove(dst), remove(backup);

    write_text(src, "partie forwarder");
    write_text(dst, "ancienne partie rom");
    check(move_save(src, dst, backup, 1) == 0, "vers la ROM : fait");
    check(contains(dst, "partie forwarder"), "vers la ROM : la ROM a la partie du forwarder");
    check(contains(backup, "ancienne partie rom"), "vers la ROM : copie de l'ancien .sav");
    check(!exists(src), "vers la ROM : le fichier du forwarder est retiré");

    remove(backup);
    write_text(src, "partie rom");
    check(move_save(src, dst, backup, 0) == 0, "vers le forwarder : fait");
    check(contains(dst, "partie rom") && contains(src, "partie rom"), "vers le forwarder : les deux fichiers ont la partie");
    check(contains(backup, "partie forwarder"), "vers le forwarder : copie de l'ancien fichier");

    remove(backup);
    remove(dst);
    check(move_save(src, dst, backup, 1) == 0 && contains(dst, "partie rom") && !exists(backup),
             "cible absente : pas de copie nécessaire");

    check(move_save("absent.sav", dst, backup, 1) != 0 && contains(dst, "partie rom"),
             "source absente : refus, rien ne change");

    write_text(src, "nouvelle");
    check(move_save(src, dst, "dossier-absent/copie.sav", 1) != 0, "copie impossible : refus");
    check(contains(dst, "partie rom") && contains(src, "nouvelle"), "copie impossible : rien ne change");

    /* base.ini, config.ini : remplacés par un .tmp vérifié */
    check(replace_file(dst, "IRBF=ab\n", 8) == 0 && contains(dst, "IRBF=ab\n") && !exists("test-dst.sav.tmp"),
             "remplacement : fait, sans .tmp restant");
    check(replace_file("dossier-absent/base.ini", "x", 1) != 0, "remplacement impossible : échec signalé");
    mkdir("test-dst.sav.tmp", 0777); /* .tmp impossible à écrire */
    check(replace_file(dst, "nouveau", 7) == -1 && contains(dst, "IRBF=ab\n"), ".tmp impossible : -1, l'ancien reste intact");
    remove(src), remove(dst), remove(backup), remove("test-dst.sav.tmp");
    /* Renommage impossible (cible = dossier non vide) : -2, le .tmp vérifié reste */
    mkdir("test-dossier.sav", 0777);
    write_text("test-dossier.sav/x", "x");
    check(replace_file("test-dossier.sav", "recu", 4) == -2 && contains("test-dossier.sav.tmp", "recu"), "renommage impossible : -2, .tmp gardé");
    remove("test-dossier.sav.tmp"), remove("test-dossier.sav/x"), remove("test-dossier.sav");

    /* Mise à jour du .3dsx : l'ancien devient .bak, le nouveau prend sa place, puis .bak est retiré */
    remove("test.3dsx.bak");
    write_text("test.3dsx", "v1");
    write_text("test.3dsx.bak", "reste d'avant");
    check(replace_program("test.3dsx", "v2", 2) == 0 && contains("test.3dsx", "v2") && !exists("test.3dsx.tmp") && !exists("test.3dsx.bak"),
             ".3dsx : remplacé, ni .tmp ni .bak restant");
    mkdir("test.3dsx.tmp", 0777); /* .tmp impossible à écrire */
    check(replace_program("test.3dsx", "v3", 2) == -1 && contains("test.3dsx", "v2") && !exists("test.3dsx.bak"), ".3dsx, .tmp impossible : -1, l'ancien en place");
    remove("test.3dsx.tmp"), remove("test.3dsx");
    write_text("test.3dsx.bak", "seule copie"); /* coupure entre les deux renommages */
    check(replace_program("test.3dsx", "v3", 2) == -1 && contains("test.3dsx.bak", "seule copie") && !exists("test.3dsx.tmp"),
             ".3dsx absent : -1, .bak (seule copie) jamais retiré");
    remove("test.3dsx.bak");
    check(launched_from_sd("sdmc:/3ds/3dssavesync.3dsx") && launched_from_sd("sdmc:/3ds/apps/Sync.3DSX"), "Homebrew Launcher : .3dsx de la carte SD");
    check(!launched_from_sd("3dslink:/3dssavesync.3dsx") && !launched_from_sd("") && !launched_from_sd("sdmc:/.3dsx") &&
                 !launched_from_sd("sdmc:/3ds/3dssavesync.3dsx.tmp"),
             "3dslink, argv vide, chemin incomplet : refusé");

    /* Copie de sécurité relue (recuperer) */
    check(write_verified(backup, "partie", 6) == 0 && contains(backup, "partie"), "écriture relue : faite");
    check(write_verified("dossier-absent/copie.sav", "partie", 6) != 0, "écriture relue impossible : échec signalé");
    remove(backup);

    /* Deux ROM, un fichier de Sauvegarde : Doublon (forwarder mGBA nommé comme la ROM, quel que soit le dossier) */
    check(same_save(SYS_GBA, "Pokemon", "sdmc:/roms/gba/USA/Pokemon.sav", SYS_GBA, "Pokemon", "sdmc:/roms/gba/EUR/Pokemon.sav"),
             "GBA, même nom dans deux dossiers : forwarder partagé");
    check(same_save(SYS_GB, "Tetris", "sdmc:/roms/gb/Tetris.sav", SYS_GBA, "TETRIS", "sdmc:/roms/gba/TETRIS.sav"),
             "GB et GBA de même nom (casse ignorée) : forwarder partagé");
    check(same_save(SYS_DS, "A", "sdmc:/saves/A.sav", SYS_DS, "a", "sdmc:/saves/a.sav"), "DS, même .sav");
    check(!same_save(SYS_DS, "Pokemon", "sdmc:/roms/nds/USA/Pokemon.sav", SYS_DS, "Pokemon", "sdmc:/roms/nds/EUR/Pokemon.sav"),
             "DS, même nom dans deux dossiers à côté de la ROM : deux .sav");
    check(!same_save(SYS_DS, "Pokemon", "sdmc:/roms/nds/Pokemon.sav", SYS_GBA, "Pokemon", "sdmc:/roms/gba/Pokemon.sav"),
             "DS et GBA de même nom : pas de forwarder pour la DS");

    /* Sauvegarde d'une ROM en sous-dossier (ADR 0015) */
    {
        char c[64];
        const char *r = "sdmc:/roms/nds", *full_tex = "sdmc:/roms/nds/Pokemon";
        check(!save_path(r, "sdmc:/roms/nds/saves", r, "X", c, sizeof c) && !strcmp(c, "sdmc:/roms/nds/saves/X.sav"), "racine, saves de TWiLight");
        check(!save_path(r, "sdmc:/roms/nds/saves", full_tex, "X", c, sizeof c) && !strcmp(c, "sdmc:/roms/nds/Pokemon/saves/X.sav"),
                 "sous-dossier, saves de TWiLight : saves du sous-dossier");
        check(!save_path(r, "", full_tex, "X", c, sizeof c) && !strcmp(c, "sdmc:/roms/nds/Pokemon/X.sav"), "sous-dossier, à côté de la ROM");
        check(!save_path(r, r, full_tex, "X", c, sizeof c) && !strcmp(c, "sdmc:/roms/nds/Pokemon/X.sav"), "saves = roms : à côté de la ROM");
        check(!save_path("sdmc:/roms/gba", "sdmc:/3ds/open_agb_firm/saves", "sdmc:/roms/gba/Hacks", "X", c, sizeof c) &&
                     !strcmp(c, "sdmc:/3ds/open_agb_firm/saves/X.sav"), "autre dossier (open_agb_firm) : gardé");
        check(save_path(r, "", full_tex, "un nom bien trop long pour ce petit tampon", c, sizeof c) != 0, "chemin trop long : refusé");
    }
    /* Copie de sécurité : jamais d'écrasement d'une copie de la même seconde */
    {
        char c[64];
        mkdir("test-copies", 0777);
        check(!backup_path("test-copies", "Pokemon", "20261103-101500", "sav", c, sizeof c) && !strcmp(c, "test-copies/Pokemon-20261103-101500.sav"),
                 "première copie : nom-date.ext");
        write_text("test-copies/Pokemon-20261103-101500.sav", "a");
        check(!backup_path("test-copies", "Pokemon", "20261103-101500", "sav", c, sizeof c) && !strcmp(c, "test-copies/Pokemon-20261103-101500-1.sav"),
                 "copie de la même seconde : -1");
        write_text("test-copies/Pokemon-20261103-101500-1.sav", "b");
        check(!backup_path("test-copies", "Pokemon", "20261103-101500", "main", c, sizeof c) && !strcmp(c, "test-copies/Pokemon-20261103-101500.main"),
                 "autre extension : pas de collision");
        check(!backup_path("test-copies", "Pokemon", "20261103-101500", "sav", c, sizeof c) && !strcmp(c, "test-copies/Pokemon-20261103-101500-2.sav"),
                 "troisième copie : -2");
        check(backup_path("test-copies", "un nom bien trop long pour ce petit tampon", "20261103-101500", "sav", c, sizeof c) != 0,
                 "chemin trop long : refusé");
        check(contains("test-copies/Pokemon-20261103-101500.sav", "a") && contains("test-copies/Pokemon-20261103-101500-1.sav", "b"),
                 "copies existantes intactes");
        remove("test-copies/Pokemon-20261103-101500.sav"), remove("test-copies/Pokemon-20261103-101500-1.sav"), remove("test-copies");
    }
    printf("%s\n", failures ? "ECHECS" : "fichiers : OK");
    return failures != 0;
}
