/* Vérifie titres.ini (liste des jeux 3DS) et la reprise de l'ancien dossier nds-save-sync.
 * Compilé sur PC : gcc -std=c99 -I source source/fichiers.c source/reglages.c test/test_reglages.c -o /tmp/r && /tmp/r */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include "fichiers.h"
#include "reglages.h"

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

int main(void)
{
    titre_t t[MAX_TITRES], u[MAX_TITRES];
    int n;

    remove("test-titres.ini");
    n = titres_lire("test-titres.ini", t, MAX_TITRES);
    verifier(n == 8 && t[0].id == 0x0004000000055D00ULL && !strcmp(t[0].nom, "Pokemon X"), "fichier absent : les 8 Pokémon");
    verifier(titres_index(t, n, 0x00040000001B5100ULL) == 7 && titres_index(t, n, 1) == -1, "titres_index");

    ecrire("test-titres.ini", "0004000000055D00=Pokemon X\r\n00040000000EC300=Jeu a moi\nmauvaise ligne\n123=trop court\n"
                              "0004000000055D00=doublon\n");
    n = titres_lire("test-titres.ini", t, MAX_TITRES);
    verifier(n == 2 && t[1].id == 0x00040000000EC300ULL && !strcmp(t[1].nom, "Jeu a moi"), "lignes valides seulement, sans doublon");
    verifier(titres_lire("test-titres.ini", t, 1) == 1, "au plus max titres");

    n = titres_lire("test-titres.ini", t, MAX_TITRES);
    verifier(titres_ecrire("test-titres.ini", t, n) == 0, "écriture");
    verifier(contient("test-titres.ini", "0004000000055D00=Pokemon X\n00040000000EC300=Jeu a moi\n"), "format : 16 chiffres hexadécimaux en majuscules");
    verifier(titres_lire("test-titres.ini", u, MAX_TITRES) == n && u[1].id == t[1].id, "relecture identique");
    ecrire("test-titres.ini", "");
    verifier(titres_lire("test-titres.ini", t, MAX_TITRES) == 0, "fichier vide : aucun jeu 3DS (tout décoché)");

    /* Reprise de l'ancien dossier */
    remove("test-ancien/config.ini"), remove("test-ancien/base.ini"), remove("test-nouveau/config.ini"), remove("test-nouveau/base.ini");
    mkdir("test-ancien", 0777);
    verifier(migrer_dossier("test-ancien", "test-nouveau") == 0, "ancien dossier sans config : rien à faire");
    ecrire("test-ancien/config.ini", "url=https://x\njeton=j\n");
    ecrire("test-ancien/base.ini", "CPUF=abc\n");
    verifier(migrer_dossier("test-ancien", "test-nouveau") == 1, "reprise faite");
    verifier(contient("test-nouveau/config.ini", "url=https://x\njeton=j\n") && contient("test-nouveau/base.ini", "CPUF=abc\n"),
             "config.ini et base.ini copiés");
    verifier(contient("test-ancien/config.ini", "url=https://x\njeton=j\n"), "l'ancien dossier reste intact");
    ecrire("test-nouveau/config.ini", "url=https://nouveau\n");
    verifier(migrer_dossier("test-ancien", "test-nouveau") == 0 && contient("test-nouveau/config.ini", "url=https://nouveau\n"),
             "nouveau dossier déjà configuré : rien n'est remplacé");

    remove("test-titres.ini");
    remove("test-ancien/config.ini"), remove("test-ancien/base.ini"), remove("test-nouveau/config.ini"), remove("test-nouveau/base.ini");
    remove("test-ancien"), remove("test-nouveau"); /* remove() retire aussi un dossier vide */
    printf("%s\n", echecs ? "ECHECS" : "reglages : OK");
    return echecs != 0;
}
