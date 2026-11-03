/* Vérifie titres.ini (liste des jeux 3DS) et la reprise de l'ancien dossier nds-save-sync.
 * Compilé sur PC : gcc -std=c99 -I source source/files.c source/settings.c test/test_settings.c -o /tmp/r && /tmp/r */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include "files.h"
#include "settings.h"

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

int main(void)
{
    title_t t[MAX_TITLES], u[MAX_TITLES];
    int n;

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
    write_text("test-titres.ini", "");
    check(read_titles("test-titres.ini", t, MAX_TITLES) == 0, "fichier vide : aucun jeu 3DS (tout décoché)");

    /* Écriture sans perte (replace_file) : .tmp impossible à écrire, l'ancien titres.ini reste entier */
    write_text("test-titres.ini", "00040000000EC300=Jeu a moi\n");
    mkdir("test-titres.ini.tmp", 0777);
    check(write_titles("test-titres.ini", t, 0) != 0 && contains("test-titres.ini", "00040000000EC300=Jeu a moi\n"),
             "écriture impossible : ancien titres.ini intact");
    remove("test-titres.ini.tmp");
    /* Coupure entre le retrait de titres.ini et le renommage : le .tmp, vérifié, est lu */
    remove("test-titres.ini");
    write_text("test-titres.ini.tmp", "00040000000EC300=Jeu a moi\n");
    check(read_titles("test-titres.ini", t, MAX_TITLES) == 1 && t[0].id == 0x00040000000EC300ULL, "titres.ini absent : titres.ini.tmp lu");
    remove("test-titres.ini.tmp");

    /* Reprise de l'ancien dossier */
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

    /* Coupure pendant un remplacement (replace_file) : seul config.ini.tmp reste ; base.ini et titres.ini sont récents */
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
    remove("test-ancien"), remove("test-nouveau"); /* remove() retire aussi un dossier vide */
    printf("%s\n", failures ? "ECHECS" : "reglages : OK");
    return failures != 0;
}
