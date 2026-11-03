/* Vérifie liste.c : liste GET /saves (tests/saves.json, écrite par server/server.py, même structure que le serveur Go),
 * champ absent jamais lu dans le jeu suivant, espaces, fin et texte mal formé.
 * Compilé sur PC : gcc -std=c99 -Wall -I source source/fichiers.c source/liste.c test/test_liste.c -o /tmp/l && /tmp/l ../tests/saves.json */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "files.h"
#include "list.h"

static int echecs;

static void verifier(int ok, const char *quoi)
{
    if (!ok) {
        printf("ECHEC  %s\n", quoi);
        echecs++;
    }
}

#define H1 "49abd65bbf7f7e40c7055093ed2e3fd75f2f602f2c5fcf955c213e3135eb03f7"
#define H2 "07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541"

int main(int argc, char **argv)
{
    unsigned char *json;
    size_t taille;
    const char *p;
    entree_t e;

    if (lire_fichier(argc > 1 ? argv[1] : "../tests/saves.json", &json, &taille)) {
        printf("saves.json illisible\n");
        return 1;
    }
    json = realloc(json, taille + 1);
    json[taille] = '\0';
    p = (const char *)json;
    verifier(liste_serveur(&p, &e) && !strcmp(e.code, "3DS-0004000000055D00") && !strcmp(e.hash, H1) && e.taille == 1024 &&
                 !strcmp(e.date, "20260101T120000Z") && !strcmp(e.client, "3ds") && e.age > 0 && e.a_nom,
             "saves.json : jeu 3DS, nommé");
    verifier(liste_serveur(&p, &e) && !strcmp(e.code, "ADAF") && !strcmp(e.hash, H2) && e.taille == 524288 &&
                 !strcmp(e.date, "20260102T081011Z") && !strcmp(e.client, "pc") && e.age > 0 && !e.a_nom,
             "saves.json : ADAF, sans nom (pas celui du jeu suivant)");
    verifier(liste_serveur(&p, &e) && !strcmp(e.code, "GBA-BPEF") && e.taille == 131072 && !strcmp(e.client, "3ds") && e.a_nom,
             "saves.json : GBA, nom avec } et \"");
    verifier(!liste_serveur(&p, &e), "saves.json : fin de la liste");
    free(json);

    p = "[{\"code\": \"AAAA\", \"taille\": 10}, {\"code\": \"BBBB\", \"hash\": \"" H1 "\", \"taille\": 20, \"date\": \"d\", "
        "\"client\": \"pc\", \"age\": 5}]";
    verifier(liste_serveur(&p, &e) && !strcmp(e.code, "AAAA") && !e.hash[0] && e.taille == 10 && !e.date[0] && !e.client[0] && e.age == -1,
             "champs absents : vides, jamais lus dans le jeu suivant");
    verifier(liste_serveur(&p, &e) && !strcmp(e.code, "BBBB") && !strcmp(e.hash, H1) && e.age == 5, "jeu suivant lu en entier");

    p = "[{\"code\":\"CCCC\",\"hash\":\"" H2 "\",\"taille\":30}, {  \"code\"  :  \"DDDD\" ,\n\"taille\" : 40 , \"age\":7 }]";
    verifier(liste_serveur(&p, &e) && !strcmp(e.code, "CCCC") && !strcmp(e.hash, H2) && e.taille == 30, "sans espace après les deux-points");
    verifier(liste_serveur(&p, &e) && !strcmp(e.code, "DDDD") && e.taille == 40 && e.age == 7, "espaces autour de : et ,");

    p = "[{\"code\": \"EEEE\", \"hash\": \"ABC\", \"version\": \"x\\\"}\"}]";
    verifier(liste_serveur(&p, &e) && !strcmp(e.code, "EEEE") && !e.hash[0] && e.taille == -1, "hash invalide : vide ; \\\" et } dans un texte");
    p = "[]";
    verifier(!liste_serveur(&p, &e), "liste vide");
    p = "[{\"code\": \"FFFF\", \"taille\": 1";
    verifier(!liste_serveur(&p, &e), "liste coupée : rien");
    p = "{\"message\": [1, 2]}";
    verifier(!liste_serveur(&p, &e), "autre réponse : rien");

    printf("%s\n", echecs ? "ECHECS" : "liste : OK");
    return echecs != 0;
}
