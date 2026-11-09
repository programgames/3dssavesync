/* Checks titres.ini (list of 3DS games) and the takeover of the old nds-save-sync folder.
 * Built on the PC: gcc -std=c99 -I source source/files.c source/settings.c test/test_settings.c -o /tmp/r && /tmp/r */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include "files.h"
#include "settings.h"

#define H1 "1111111111111111111111111111111111111111111111111111111111111111"
#define H2 "2222222222222222222222222222222222222222222222222222222222222222"

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

static void count_entry(void *ctx, const char *k, const char *v)
{
    (void)k, (void)v;
    ++*(int *)ctx;
}

/* config.ini, base.ini, alias.ini, maj.ini: formats on the SD card (read by the PC too), .tmp fallback, writes. */
static void test_ini_files(void)
{
    static config_t c;
    static bases_t b;
    static aliases_t a;
    update_info_t u;
    int lines = 0;

    remove("test.ini"), remove("test.ini.tmp");
    check(read_ini("test.ini", count_entry, &lines) == -1 && lines == 0, "read_ini: no file");
    write_text("test.ini.tmp", "a=1\nno equal sign\r\nb=\n");
    check(read_ini("test.ini", count_entry, &lines) == 0 && lines == 2, "read_ini: .tmp read, line without = skipped");
    remove("test.ini.tmp");

    read_config("test.ini", &c);
    check(!strcmp(c.roms, "sdmc:/roms/nds") && !strcmp(c.saves, "sdmc:/roms/nds/saves") && !strcmp(c.roms_gb, "sdmc:/roms/gb") &&
              !c.saves_gb[0] && !strcmp(c.roms_gba, "sdmc:/roms/gba") && !c.saves_gba[0] && !c.url[0] && !c.token[0],
          "config: defaults");
    write_text("test.ini", "url=https://x\r\njeton=j=k\nsaves=\nsaves_gba=sdmc:/agb\ninconnu=1\n");
    read_config("test.ini", &c);
    check(!strcmp(c.url, "https://x") && !strcmp(c.token, "j=k") && !c.saves[0] && !strcmp(c.saves_gba, "sdmc:/agb") &&
              !strcmp(c.roms, "sdmc:/roms/nds"), "config: read over the defaults");
    check(write_config("test.ini", &c) == 0 &&
              contains("test.ini", "url=https://x\njeton=j=k\nroms=sdmc:/roms/nds\nsaves=\nroms_gb=sdmc:/roms/gb\nsaves_gb=\n"
                                   "roms_gba=sdmc:/roms/gba\nsaves_gba=sdmc:/agb\ntri=etat\nverifier_maj=1\n"), "config: format");
    mkdir("test.ini.tmp", 0777);
    check(write_config("test.ini", &c) != 0 && contains("test.ini", "url=https://x\njeton=j=k\nroms=sdmc:/roms/nds\nsaves=\nroms_gb=sdmc:/roms/gb\n"
                                                                     "saves_gb=\nroms_gba=sdmc:/roms/gba\nsaves_gba=sdmc:/agb\ntri=etat\nverifier_maj=1\n"),
          "config: write failed, old file intact");
    remove("test.ini.tmp");
    check(c.order == 0, "config: no tri line (older file): to do first");
    write_text("test.ini", "url=https://x\ntri=nom\n");
    read_config("test.ini", &c);
    check(c.order == 1, "config: tri=nom");
    c.order = 2;
    check(write_config("test.ini", &c) == 0, "config: written with tri");
    read_config("test.ini", &c);
    check(c.order == 2, "config: tri=date read back");
    write_text("test.ini", "tri=inconnu\n");
    read_config("test.ini", &c);
    check(c.order == 0, "config: unknown tri value: default");
    check(c.check_update == 1, "config: no verifier_maj line (older file): daily check on");
    write_text("test.ini", "url=https://x\nverifier_maj=0\n");
    read_config("test.ini", &c);
    check(c.check_update == 0, "config: verifier_maj=0: daily check off");
    check(write_config("test.ini", &c) == 0 && contains("test.ini", "url=https://x\njeton=\nroms=sdmc:/roms/nds\nsaves=sdmc:/roms/nds/saves\nroms_gb=sdmc:/roms/gb\n"
                                                                     "saves_gb=\nroms_gba=sdmc:/roms/gba\nsaves_gba=\ntri=etat\nverifier_maj=0\n"),
          "config: written with verifier_maj=0");
    read_config("test.ini", &c);
    check(c.check_update == 0, "config: verifier_maj=0 read back");
    write_text("test.ini", "verifier_maj=1\n");
    read_config("test.ini", &c);
    check(c.check_update == 1, "config: verifier_maj=1: daily check on");

    write_text("test.ini", "IRBF=" H1 "\nABC=" H1 "\nGBA-BPEF=" H2 "x\nGB-POKEMON-1234=" H2 "\n");
    read_bases("test.ini", &b);
    check(b.n == 2 && !strcmp(base_of(&b, "IRBF"), H1) && !strcmp(base_of(&b, "GB-POKEMON-1234"), H2) && !base_of(&b, "ABC") &&
              !base_of(&b, "GBA-BPEF"), "bases: Key of 4 characters at least, Hash of 64");
    check(set_base("test.ini", &b, "IRBF", H2) == 0 && set_base("test.ini", &b, "3DS-0004000000055D00", H1) == 0 &&
              contains("test.ini", "IRBF=" H2 "\nGB-POKEMON-1234=" H2 "\n3DS-0004000000055D00=" H1 "\n"), "bases: replaced, added, format");
    mkdir("test.ini.tmp", 0777);
    check(set_base("test.ini", &b, "IRBF", H1) == -1 && !strcmp(base_of(&b, "IRBF"), H1) &&
              contains("test.ini", "IRBF=" H2 "\nGB-POKEMON-1234=" H2 "\n3DS-0004000000055D00=" H1 "\n"), "bases: write failed, old file intact");
    remove("test.ini.tmp");

    /* No limit: 600 Games (the old limit was 512), written and read back in order */
    {
        char k[16];
        int i, ok = 1;
        write_text("test.ini", "");
        read_bases("test.ini", &b);
        for (i = 0; i < 600 && ok; i++) {
            snprintf(k, sizeof k, "K%04d", i);
            ok = set_base("test.ini", &b, k, i % 2 ? H1 : H2) == 0;
        }
        check(ok && read_bases("test.ini", &b) == 0 && b.n == 600 && !strcmp(b.e[0].key, "K0000") && !strcmp(b.e[599].key, "K0599") &&
                  !strcmp(base_of(&b, "K0599"), H1) && !strcmp(base_of(&b, "K0512"), H2), "bases: 600 Games kept");
    }

    /* forget_base: the extdata Base of a reinstalled game (ADR 0031) */
    write_text("test.ini", "3DSX-0000055D=" H1 "\n3DS-0004000000055D00=" H2 "\n");
    read_bases("test.ini", &b);
    check(forget_base(&b, "3DSX-0000055D") == 1 && !base_of(&b, "3DSX-0000055D") && !strcmp(base_of(&b, "3DS-0004000000055D00"), H2) &&
              b.n == 1 && forget_base(&b, "3DSX-0000055D") == 0, "forget_base: removes only that Base, once");

    /* Reinstalled game (ADR 0027): its install date in base.ini; a new date removes its Base, a first date keeps it */
    write_text("test.ini", "3DS-0004000000055D00=" H1 "\nVC-0004000000172000=" H2 "\n");
    read_bases("test.ini", &b);
    check(note_install(&b, 0x0004000000055D00ULL, 0, "3DS-0004000000055D00") == 0 && b.n == 2, "install: unknown date, nothing changes");
    check(note_install(&b, 0x0004000000055D00ULL, 0x1234, "3DS-0004000000055D00") == 1 &&
              !strcmp(base_of(&b, "3DS-0004000000055D00"), H1), "install: first date, Base kept");
    check(note_install(&b, 0x0004000000055D00ULL, 0x1234, "3DS-0004000000055D00") == 0, "install: same date, nothing changes");
    check(write_bases("test.ini", &b) == 0 && read_bases("test.ini", &b) == 0 &&
              !strcmp(base_of(&b, "INSTALL-0004000000055D00"), "0000000000000000000000000000000000000000000000000000000000001234"),
          "install: date written as 64 characters and read back");
    check(note_install(&b, 0x0004000000055D00ULL, 0x5678, "3DS-0004000000055D00") == 2 && !base_of(&b, "3DS-0004000000055D00") &&
              !strcmp(base_of(&b, "VC-0004000000172000"), H2) && b.n == 2, "install: new date, Base removed, other Bases kept");
    check(note_install(&b, 0x0004000000172000ULL, 0x9, "VC-0004000000172000") == 1 && note_install(&b, 0x0004000000172000ULL, 0xA, "VC-0004000000172000") == 2 &&
              !base_of(&b, "VC-0004000000172000"), "install: Virtual Console game, Base of its base_id removed");

    write_text("test.ini", "Pokemon Radical Red = GBA-0RAD\nHack=GBA-0HAK\n=GBA-VIDE\n   =GBA-ESPACES\n");
    read_aliases("test.ini", &a);
    check(a.n == 2 && !strcmp(alias_of(&a, "Pokemon Radical Red"), " GBA-0RAD") && !strcmp(alias_of(&a, "Hack"), "GBA-0HAK") &&
              !alias_of(&a, "Pokemon Radical Red ") && !alias_of(&a, ""), "aliases: spaces before = dropped, empty name skipped");

    remove("test.ini");
    read_update_info("test.ini", &u);
    check(u.day == -1 && !u.latest[0] && !u.installed[0], "maj.ini: no file");
    write_text("test.ini", "jour=20395\nversion=3.7.1\ninstallee=3.7.0\n");
    read_update_info("test.ini", &u);
    check(u.day == 20395 && !strcmp(u.latest, "3.7.1") && !strcmp(u.installed, "3.7.0"), "maj.ini: read");
    check(write_update_info("test.ini", &u, "3.7.1") == 0 && contains("test.ini", "jour=20395\nversion=3.7.1\ninstallee=3.7.1\n"),
          "maj.ini: format, running version written");
    remove("test.ini");
}

int main(void)
{
    title_t t[MAX_TITLES], u[MAX_TITLES];
    int n;

    test_ini_files();

    remove("test-titres.ini");
    n = read_titles("test-titres.ini", t, MAX_TITLES);
    check(n == 8 && t[0].id == 0x0004000000055D00ULL && !strcmp(t[0].name, "Pokemon X"), "fichier absent : les 8 Pokémon");
    check(title_index(t, n, 0x00040000001B5100ULL) == 7 && title_index(t, n, 1) == -1, "titres_index");

    write_text("test-titres.ini", "0004000000055D00=Pokemon X\r\n00040000000EC300=Jeu a moi\nmauvaise ligne\n123=trop court\n"
                              "0004000000055D00=doublon\n");
    n = read_titles("test-titres.ini", t, MAX_TITLES);
    check(n == 2 && t[1].id == 0x00040000000EC300ULL && !strcmp(t[1].name, "Jeu a moi"), "lignes valides seulement, sans doublon");
    check(read_titles("test-titres.ini", t, 1) == 1, "au plus max titres");

    n = read_titles("test-titres.ini", t, MAX_TITLES);
    check(write_titles("test-titres.ini", t, n) == 0, "écriture");
    check(contains("test-titres.ini", "0004000000055D00=Pokemon X\n00040000000EC300=Jeu a moi\n"), "format : 16 chiffres hexadécimaux en majuscules");
    check(read_titles("test-titres.ini", u, MAX_TITLES) == n && u[1].id == t[1].id, "relecture identique");
    for (n = 0; n < 200; n++) {
        t[n].id = 0x0004000000100000ULL + n;
        snprintf(t[n].name, sizeof t[n].name, "Jeu %d", n);
    }
    check(write_titles("test-titres.ini", t, 200) == 0 && read_titles("test-titres.ini", u, MAX_TITLES) == 200 && u[199].id == 0x00040000001000C7ULL,
          "200 jeux 3DS (ancienne limite : 64)");
    write_text("test-titres.ini", "");
    check(read_titles("test-titres.ini", t, MAX_TITLES) == 0, "fichier vide : aucun jeu 3DS (tout décoché)");

    /* Write without loss (replace_file): .tmp cannot be written, the old titres.ini stays whole */
    write_text("test-titres.ini", "00040000000EC300=Jeu a moi\n");
    mkdir("test-titres.ini.tmp", 0777);
    check(write_titles("test-titres.ini", t, 0) != 0 && contains("test-titres.ini", "00040000000EC300=Jeu a moi\n"),
             "écriture impossible : ancien titres.ini intact");
    remove("test-titres.ini.tmp");
    /* Power cut between the removal of titres.ini and the rename: the checked .tmp is read */
    remove("test-titres.ini");
    write_text("test-titres.ini.tmp", "00040000000EC300=Jeu a moi\n");
    check(read_titles("test-titres.ini", t, MAX_TITLES) == 1 && t[0].id == 0x00040000000EC300ULL, "titres.ini absent : titres.ini.tmp lu");
    remove("test-titres.ini.tmp");

    /* titres-off.ini (ADR 0028): one title ID per line, read back, .tmp fallback */
    {
        static unsigned long long ids[MAX_OFF], back[MAX_OFF];
        remove("test-off.ini"), remove("test-off.ini.tmp");
        check(read_off_titles("test-off.ini", ids, MAX_OFF) == 0, "titres-off.ini absent : aucun jeu désactivé");
        write_text("test-off.ini", "00040000000EC300\r\n0004000000030800\nmauvaise ligne\n123\n00040000000EC300\n00040000000EC300=nom\n");
        check(read_off_titles("test-off.ini", ids, MAX_OFF) == 2 && ids[0] == 0x00040000000EC300ULL && ids[1] == 0x0004000000030800ULL,
              "titres-off.ini : lignes valides seulement, sans doublon");
        check(read_off_titles("test-off.ini", ids, 1) == 1, "titres-off.ini : au plus max titres");
        check(write_off_titles("test-off.ini", ids, 2) == 0 && contains("test-off.ini", "00040000000EC300\n0004000000030800\n"),
              "titres-off.ini : format 16 chiffres hexadécimaux en majuscules");
        check(read_off_titles("test-off.ini", back, MAX_OFF) == 2 && back[1] == ids[1], "titres-off.ini : relecture identique");
        check(write_off_titles("test-off.ini", ids, 0) == 0 && read_off_titles("test-off.ini", back, MAX_OFF) == 0, "titres-off.ini vide : tous les jeux");
        remove("test-off.ini");
        write_text("test-off.ini.tmp", "0004000000030800\n");
        check(read_off_titles("test-off.ini", back, MAX_OFF) == 1 && back[0] == 0x0004000000030800ULL, "titres-off.ini absent : .tmp lu");
        remove("test-off.ini.tmp");
    }

    /* Takeover of the old folder */
    remove("test-ancien/config.ini"), remove("test-ancien/base.ini"), remove("test-nouveau/config.ini"), remove("test-nouveau/base.ini");
    mkdir("test-ancien", 0777);
    check(migrate_dir("test-ancien", "test-nouveau") == 0, "ancien dossier sans config : rien à faire");
    write_text("test-ancien/config.ini", "url=https://x\njeton=j\n");
    write_text("test-ancien/base.ini", "CPUF=abc\n");
    check(migrate_dir("test-ancien", "test-nouveau") == 1, "reprise faite");
    check(contains("test-nouveau/config.ini", "url=https://x\njeton=j\n") && contains("test-nouveau/base.ini", "CPUF=abc\n"),
             "config.ini et base.ini copiés");
    check(contains("test-ancien/config.ini", "url=https://x\njeton=j\n"), "l'ancien dossier reste intact");
    write_text("test-nouveau/config.ini", "url=https://nouveau\n");
    check(migrate_dir("test-ancien", "test-nouveau") == 0 && contains("test-nouveau/config.ini", "url=https://nouveau\n"),
             "nouveau dossier déjà configuré : rien n'est remplacé");

    /* Power cut during a replace (replace_file): only config.ini.tmp is left; base.ini and titres.ini are recent */
    remove("test-nouveau/config.ini");
    write_text("test-nouveau/config.ini.tmp", "url=https://nouveau\n");
    write_text("test-nouveau/base.ini", "CPUF=recent\n");
    check(migrate_dir("test-ancien", "test-nouveau") == 0 && contains("test-nouveau/base.ini", "CPUF=recent\n"),
             "config.ini.tmp présent : rien n'est remplacé");
    remove("test-nouveau/config.ini.tmp");
    check(migrate_dir("test-ancien", "test-nouveau") == 0 && contains("test-nouveau/base.ini", "CPUF=recent\n"),
             "base.ini présent sans config.ini : rien n'est remplacé");
    remove("test-nouveau/base.ini");
    write_text("test-nouveau/base.ini.tmp", "CPUF=recent\n");
    check(migrate_dir("test-ancien", "test-nouveau") == 0 && contains("test-nouveau/base.ini.tmp", "CPUF=recent\n"),
             "base.ini.tmp présent : rien n'est remplacé");
    remove("test-nouveau/base.ini.tmp");
    write_text("test-nouveau/titres.ini", "00040000000EC300=Jeu a moi\n");
    check(migrate_dir("test-ancien", "test-nouveau") == 0 && contains("test-nouveau/titres.ini", "00040000000EC300=Jeu a moi\n"),
             "titres.ini présent : rien n'est remplacé");
    remove("test-nouveau/titres.ini");

    remove("test-titres.ini");
    remove("test-ancien/config.ini"), remove("test-ancien/base.ini"), remove("test-nouveau/config.ini"), remove("test-nouveau/base.ini");
    remove("test-ancien"), remove("test-nouveau"); /* remove() also removes an empty folder */
    printf("%s\n", failures ? "ECHECS" : "reglages : OK");
    return failures != 0;
}
