/* Checks list.c: GET /saves list (tests/saves.json, written by server/server.py, same structure as the Go server),
 * a missing field never read from the next game, spaces, end and malformed text.
 * Built on the PC: gcc -std=c99 -Wall -I source source/files.c source/list.c test/test_list.c -o /tmp/l && /tmp/l ../tests/saves.json */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "files.h"
#include "list.h"

static int failures;

static void check(int ok, const char *what)
{
    if (!ok) {
        printf("ECHEC  %s\n", what);
        failures++;
    }
}

#define H1 "49abd65bbf7f7e40c7055093ed2e3fd75f2f602f2c5fcf955c213e3135eb03f7"
#define H2 "07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541"

int main(int argc, char **argv)
{
    unsigned char *json;
    size_t size;
    const char *p;
    entry_t e;

    if (read_file(argc > 1 ? argv[1] : "../tests/saves.json", &json, &size)) {
        printf("saves.json illisible\n");
        return 1;
    }
    json = realloc(json, size + 1);
    json[size] = '\0';
    p = (const char *)json;
    check(next_server_entry(&p, &e) && !strcmp(e.code, "3DS-0004000000055D00") && !strcmp(e.hash, H1) && e.size == 1024 &&
                 !strcmp(e.date, "20260101T120000Z") && !strcmp(e.client, "3ds") && e.age > 0 && e.has_name,
             "saves.json : jeu 3DS, nommé");
    check(next_server_entry(&p, &e) && !strcmp(e.code, "ADAF") && !strcmp(e.hash, H2) && e.size == 524288 &&
                 !strcmp(e.date, "20260102T081011Z") && !strcmp(e.client, "pc") && e.age > 0 && !e.has_name &&
                 !strcmp(e.version, "000002-20260102T081011Z-pc-" H2 ".sav"),
             "saves.json : ADAF, sans nom (pas celui du jeu suivant)");
    check(next_server_entry(&p, &e) && !strcmp(e.code, "GBA-BPEF") && e.size == 131072 && !strcmp(e.client, "3ds") && e.has_name,
             "saves.json : GBA, nom avec } et \"");
    check(!next_server_entry(&p, &e), "saves.json : fin de la liste");
    free(json);

    p = "[{\"code\": \"AAAA\", \"taille\": 10}, {\"code\": \"BBBB\", \"hash\": \"" H1 "\", \"taille\": 20, \"date\": \"d\", "
        "\"client\": \"pc\", \"age\": 5}]";
    check(next_server_entry(&p, &e) && !strcmp(e.code, "AAAA") && !e.hash[0] && e.size == 10 && !e.date[0] && !e.client[0] && e.age == -1,
             "champs absents : vides, jamais lus dans le jeu suivant");
    check(next_server_entry(&p, &e) && !strcmp(e.code, "BBBB") && !strcmp(e.hash, H1) && e.age == 5, "jeu suivant lu en entier");

    p = "[{\"code\":\"CCCC\",\"hash\":\"" H2 "\",\"taille\":30}, {  \"code\"  :  \"DDDD\" ,\n\"taille\" : 40 , \"age\":7 }]";
    check(next_server_entry(&p, &e) && !strcmp(e.code, "CCCC") && !strcmp(e.hash, H2) && e.size == 30, "sans espace après les deux-points");
    check(next_server_entry(&p, &e) && !strcmp(e.code, "DDDD") && e.size == 40 && e.age == 7, "espaces autour de : et ,");

    p = "[{\"code\": \"EEEE\", \"hash\": \"ABC\", \"version\": \"x\\\"}\"}]";
    check(next_server_entry(&p, &e) && !strcmp(e.code, "EEEE") && !e.hash[0] && e.size == -1, "hash invalide : vide ; \\\" et } dans un texte");
    /* GET /saves/<key>/history: the same objects; version goes into a URL */
    p = "[{\"code\": \"ADAF\", \"version\": \"000001-20261103T101500Z-restauration-" H2 ".sav\", \"hash\": \"" H2 "\", \"taille\": 512, "
        "\"date\": \"20261103T101500Z\", \"client\": \"restauration\", \"age\": 120}, "
        "{\"code\": \"ADAF\", \"version\": \"000002-x/../" H1 ".sav\", \"hash\": \"" H1 "\"}, "
        "{\"code\": \"ADAF\", \"version\": \"000003-20261103T101500Z-pc-" H1 ".sav\", \"hash\": \"" H2 "\"}, "
        "{\"code\": \"ADAF\", \"version\": \"000004-20261103T101500Z-pc-" H1 ".sav?\", \"hash\": \"" H1 "\"}]";
    check(next_server_entry(&p, &e) && !strcmp(e.version, "000001-20261103T101500Z-restauration-" H2 ".sav") && e.size == 512 &&
                 !strcmp(e.client, "restauration") && e.age == 120,
             "historique : version lue");
    check(next_server_entry(&p, &e) && !e.version[0] && !strcmp(e.hash, H1), "historique : / dans la version : vide");
    check(next_server_entry(&p, &e) && !e.version[0], "historique : version d'un autre hash : vide");
    check(next_server_entry(&p, &e) && !e.version[0], "historique : version sans .sav à la fin : vide");
    p = "[]";
    check(!next_server_entry(&p, &e), "liste vide");
    p = "[{\"code\": \"FFFF\", \"taille\": 1";
    check(!next_server_entry(&p, &e), "liste coupée : rien");
    p = "{\"message\": [1, 2]}";
    check(!next_server_entry(&p, &e), "autre réponse : rien");

    printf("%s\n", failures ? "ECHECS" : "liste : OK");
    return failures != 0;
}
