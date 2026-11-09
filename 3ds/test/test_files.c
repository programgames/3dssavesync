/* Checks move_save (switch between the mGBA forwarder and the .sav next to the ROM): never a loss.
 * Built on the PC: gcc -std=c99 -I source source/files.c test/test_files.c -o /tmp/f && /tmp/f */
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

/* 1 if the file holds exactly text. */
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

    /* journal.log: lines added at the end; past max, the file becomes .1 (the old .1 goes) */
    remove("test.log"), remove("test.log.1");
    check(append_log("test.log", "a", 10) == 0 && append_log("test.log", "b", 10) == 0 && contains("test.log", "a\nb\n"), "journal : lignes ajoutées");
    check(append_log("test.log", "0123456789", 10) == 0 && contains("test.log", "a\nb\n0123456789\n") && !exists("test.log.1"),
          "journal : sous la limite, rien ne tourne");
    check(append_log("test.log", "c", 10) == 0 && contains("test.log", "c\n") && contains("test.log.1", "a\nb\n0123456789\n"),
          "journal : au-delà, le fichier devient .1");
    write_text("test.log", "0123456789AB");
    check(append_log("test.log", "e", 10) == 0 && contains("test.log", "e\n") && contains("test.log.1", "0123456789AB"),
          "journal : l'ancien .1 est remplacé");
    check(append_log("dossier-absent/test.log", "x", 10) == -1, "journal : dossier absent, échec signalé");
    remove("test.log"), remove("test.log.1");

    /* base.ini, config.ini: replaced through a checked .tmp */
    check(replace_file(dst, "IRBF=ab\n", 8) == 0 && contains(dst, "IRBF=ab\n") && !exists("test-dst.sav.tmp"),
             "remplacement : fait, sans .tmp restant");
    check(replace_file("dossier-absent/base.ini", "x", 1) != 0, "remplacement impossible : échec signalé");
    mkdir("test-dst.sav.tmp", 0777); /* .tmp cannot be written */
    check(replace_file(dst, "nouveau", 7) == -1 && contains(dst, "IRBF=ab\n"), ".tmp impossible : -1, l'ancien reste intact");
    remove(src), remove(dst), remove(backup), remove("test-dst.sav.tmp");
    /* Rename impossible (target = non-empty folder): -2, the checked .tmp stays */
    mkdir("test-dossier.sav", 0777);
    write_text("test-dossier.sav/x", "x");
    check(replace_file("test-dossier.sav", "recu", 4) == -2 && contains("test-dossier.sav.tmp", "recu"), "renommage impossible : -2, .tmp gardé");
    remove("test-dossier.sav.tmp"), remove("test-dossier.sav/x"), remove("test-dossier.sav");

    /* .3dsx update: the old one becomes .bak, the new one takes its place, then .bak is removed */
    remove("test.3dsx.bak");
    write_text("test.3dsx", "v1");
    write_text("test.3dsx.bak", "reste d'avant");
    check(replace_program("test.3dsx", "v2", 2) == 0 && contains("test.3dsx", "v2") && !exists("test.3dsx.tmp") && !exists("test.3dsx.bak"),
             ".3dsx : remplacé, ni .tmp ni .bak restant");
    mkdir("test.3dsx.tmp", 0777); /* .tmp cannot be written */
    check(replace_program("test.3dsx", "v3", 2) == -1 && contains("test.3dsx", "v2") && !exists("test.3dsx.bak"), ".3dsx, .tmp impossible : -1, l'ancien en place");
    remove("test.3dsx.tmp"), remove("test.3dsx");
    write_text("test.3dsx.bak", "seule copie"); /* power cut between the two renames */
    check(replace_program("test.3dsx", "v3", 2) == -1 && contains("test.3dsx.bak", "seule copie") && !exists("test.3dsx.tmp"),
             ".3dsx absent : -1, .bak (seule copie) jamais retiré");
    remove("test.3dsx.bak");
    check(launched_from_sd("sdmc:/3ds/3dssavesync.3dsx") && launched_from_sd("sdmc:/3ds/apps/Sync.3DSX"), "Homebrew Launcher : .3dsx de la carte SD");
    check(!launched_from_sd("3dslink:/3dssavesync.3dsx") && !launched_from_sd("") && !launched_from_sd("sdmc:/.3dsx") &&
                 !launched_from_sd("sdmc:/3ds/3dssavesync.3dsx.tmp"),
             "3dslink, argv vide, chemin incomplet : refusé");

    /* Backup copy read back (download_save) */
    check(write_verified(backup, "partie", 6) == 0 && contains(backup, "partie"), "écriture relue : faite");
    check(write_verified("dossier-absent/copie.sav", "partie", 6) != 0, "écriture relue impossible : échec signalé");
    {
        static unsigned char big[200000]; /* read back in several pieces */
        size_t i;
        for (i = 0; i < sizeof big; i++) big[i] = (unsigned char)(i * 7);
        check(write_verified(backup, big, sizeof big) == 0, "grande écriture relue par morceaux : faite");
        check(write_verified(backup, "", 0) == 0, "fichier vide relu : fait");
    }
    remove(backup);

    /* Two ROMs, one Save file: Duplicate (mGBA forwarder named like the ROM, whatever the folder) */
    check(same_save(SYS_GBA, "Pokemon", "sdmc:/roms/gba/USA/Pokemon.sav", SYS_GBA, "Pokemon", "sdmc:/roms/gba/EUR/Pokemon.sav"),
             "GBA, même nom dans deux dossiers : forwarder partagé");
    check(same_save(SYS_GB, "Tetris", "sdmc:/roms/gb/Tetris.sav", SYS_GBA, "TETRIS", "sdmc:/roms/gba/TETRIS.sav"),
             "GB et GBA de même nom (casse ignorée) : forwarder partagé");
    check(same_save(SYS_DS, "A", "sdmc:/saves/A.sav", SYS_DS, "a", "sdmc:/saves/a.sav"), "DS, même .sav");
    check(!same_save(SYS_DS, "Pokemon", "sdmc:/roms/nds/USA/Pokemon.sav", SYS_DS, "Pokemon", "sdmc:/roms/nds/EUR/Pokemon.sav"),
             "DS, même nom dans deux dossiers à côté de la ROM : deux .sav");
    check(!same_save(SYS_DS, "Pokemon", "sdmc:/roms/nds/Pokemon.sav", SYS_GBA, "Pokemon", "sdmc:/roms/gba/Pokemon.sav"),
             "DS et GBA de même nom : pas de forwarder pour la DS");

    /* Save of a ROM in a subfolder (ADR 0015) */
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
    /* Backup copy: never overwrite a copy made in the same second */
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
    { /* old_copies: the 10 newest copies of one game stay, the copy just made included; nothing else is chosen */
        char *n[40], d[40], buf[40][64];
        int i, c = 0;
        for (i = 1; i <= 12; i++) { /* 12 copies of the game, 20261101..20261112 */
            snprintf(buf[c], sizeof buf[c], "Pokemon X-3DS-0004000000055D00-202611%02d-101500.main", i);
            n[c] = buf[c], c++;
        }
        n[c++] = "Pokemon X Hack-3DS-0004000000055D00-20260101-101500.main"; /* another game, older */
        n[c++] = "Pokemon X-20260101-101500.main";                           /* old naming: no Key */
        n[c++] = "Pokemon X-3DS-0004000000055D00-20260101-101500.main.tmp";  /* not a copy */
        n[c++] = "Pokemon X-3DS-0004000000055D00-2026010-1101500.main";      /* bad date */
        n[c++] = "Pokemon X-3DS-0004000000055D00-20260101-101500.txt";       /* other extension */
        n[c++] = "Pokemon X-3DS-0004000000055D00-20260101-101500-.main";     /* bad number */
        check(old_copies("Pokemon X-3DS-0004000000055D00", buf[11], n, c, 10, d) == 2 && d[0] && d[1], "copies : 12, les 2 plus vieilles partent");
        for (i = 2; i < c; i++) check(!d[i], "copies : le reste reste");
        check(old_copies("Pokemon X-3DS-0004000000055D00", buf[0], n, c, 10, d) == 2 && !d[0] && d[1] && d[2] && !d[3],
              "copies : l'horloge est revenue en arrière, la copie faite reste");
        check(old_copies("Pokemon X-3DS-0004000000055D00", "absente.main", n, c, 10, d) == 0, "copies : copie faite absente, rien");
        check(old_copies("Pokemon X-3DS-0004000000055D00", n[12], n, c, 10, d) == 0, "copies : copie faite d'un autre jeu, rien");
        check(old_copies("Pokemon X-3DS-0004000000055D00", buf[9], n, 10, 10, d) == 0, "copies : 10, rien");
        c = 0; /* same second: the number decides, -2 is newer than -1, newer than none */
        n[c++] = "Zelda-GBA-AZLE-20261101-101500-2.sav";
        n[c++] = "Zelda-GBA-AZLE-20261101-101500.sav";
        n[c++] = "Zelda-GBA-AZLE-20261101-101500-10.sav";
        n[c++] = "Zelda-GBA-AZLE-20261101-101500-1.sav";
        check(old_copies("Zelda-GBA-AZLE", n[2], n, c, 2, d) == 2 && !d[0] && d[1] && !d[2] && d[3], "copies : même seconde, le numéro décide");
    }
    { /* custom CA certificates (ADR 0029) */
        char *cas = NULL;
        size_t n = 0;
        remove("test-ca.pem");
        check(read_cas("test-ca.pem", "ROOT", 10, &cas, &n) == 1, "certificats : fichier absent, rien à ajouter");
        write_text("test-ca.pem", "");
        check(read_cas("test-ca.pem", "ROOT", 10, &cas, &n) == 1, "certificats : fichier vide, rien à ajouter");
        write_text("test-ca.pem", "0123456789A");
        check(read_cas("test-ca.pem", "ROOT", 10, &cas, &n) == -2, "certificats : trop gros, refusé");
        write_text("test-ca.pem", "PEM");
        check(read_cas("test-ca.pem", "ROOT", 10, &cas, &n) == 0 && n == 8 && !strcmp(cas, "ROOT\nPEM"), "certificats : racine puis fichier");
        free(cas);
        remove("test-ca.pem");
    }
    printf("%s\n", failures ? "ECHECS" : "fichiers : OK");
    return failures != 0;
}
