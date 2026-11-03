/* Vérifie maj.c : versions, réponse de GitHub (test/release_latest.json, vraie réponse de /releases/latest réduite),
 * SHA256SUMS, une vérification par jour.
 * Compilé sur PC : gcc -std=c99 -Wall -I source source/fichiers.c source/maj.c test/test_maj.c -o /tmp/m && /tmp/m test/release_latest.json */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "files.h"
#include "update.h"

static int echecs, total;

static void verifier(int ok, const char *quoi)
{
    total++;
    if (!ok) {
        printf("ECHEC  %s\n", quoi);
        echecs++;
    }
}

int main(int argc, char **argv)
{
    unsigned char *json;
    size_t taille;
    release_t r;
    char h[65];
    const char *sommes = "5f2b1c0e9a8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b3a2f1e0d9c8b7a6f5e4d3c2b  3dssavesync.cia.sig\n"
                         "AB12CD34EF56AB12CD34EF56AB12CD34EF56AB12CD34EF56AB12CD34EF56AB12  3dssavesync.cia\r\n"
                         "0000000000000000000000000000000000000000000000000000000000000001 *3dssavesync.3dsx\n";

    verifier(maj_plus_recente("3.9.1", "3.10.0"), "3.10.0 > 3.9.1");
    verifier(!maj_plus_recente("3.10.0", "3.9.1"), "3.9.1 < 3.10.0");
    verifier(maj_plus_recente("3.6.1", "v3.7.0"), "v3.7.0 > 3.6.1");
    verifier(!maj_plus_recente("3.7.0", "3.7.0"), "3.7.0 = 3.7.0");
    verifier(maj_plus_recente("3.7.0-rc.1", "3.7.0"), "3.7.0 > 3.7.0-rc.1");
    verifier(!maj_plus_recente("3.7.0", "3.7.0-rc.1"), "3.7.0-rc.1 < 3.7.0");
    verifier(maj_plus_recente("2.99.99", "3.0.0"), "3.0.0 > 2.99.99");
    verifier(!maj_plus_recente("dev", "3.7.0"), "dev : jamais de mise à jour");
    verifier(!maj_plus_recente("3.6.1", ""), "version distante vide");
    verifier(!maj_plus_recente("3.6.1", "3.7"), "version distante incomplète");
    verifier(!maj_plus_recente("3.6.1", "3.7.0x"), "version distante mal formée");

    if (lire_fichier(argc > 1 ? argv[1] : "test/release_latest.json", &json, &taille)) {
        printf("release_latest.json illisible\n");
        return 1;
    }
    json = realloc(json, taille + 1);
    json[taille] = '\0';
    verifier(maj_release((char *)json, &r) == 0, "release_latest.json lu");
    verifier(!strcmp(r.version, "3.6.1"), "tag_name v3.6.1 -> 3.6.1");
    verifier(!strcmp(r.cia, "https://github.com/programgames/3dssavesync/releases/download/v3.6.1/3dssavesync.cia"), "adresse du .cia");
    verifier(!strcmp(r.dsx, "https://github.com/programgames/3dssavesync/releases/download/v3.6.1/3dssavesync.3dsx"), "adresse du .3dsx");
    verifier(r.sommes[0] == '\0', "v3.6.1 : pas de SHA256SUMS");
    free(json);

    verifier(maj_release("{\"body\":\"\\\"tag_name\\\":\\\"v9.9.9\\\"\",\"tag_name\": \"v3.7.0\",\"assets\":[{\"browser_download_url\": "
                         "\"https://github.com/o/r/releases/download/v3.7.0/SHA256SUMS\"}]}", &r) == 0 && !strcmp(r.version, "3.7.0"),
             "tag_name échappé dans body ignoré, espace après les deux-points");
    verifier(!strcmp(r.sommes, "https://github.com/o/r/releases/download/v3.7.0/SHA256SUMS") && !r.cia[0], "adresse de SHA256SUMS");
    verifier(maj_release("{\"message\":\"API rate limit exceeded\"}", &r) == -1, "réponse sans tag_name");
    verifier(maj_release("{\"tag_name\":\"\"}", &r) == -1, "tag_name vide");

    verifier(maj_somme(sommes, "3dssavesync.cia", h) == 0 && !strcmp(h, "ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12"),
             "somme du .cia (pas celle du .cia.sig), en minuscules, fin de ligne CRLF");
    verifier(maj_somme(sommes, "3dssavesync.3dsx", h) == 0 && !strcmp(h, "0000000000000000000000000000000000000000000000000000000000000001"),
             "somme du .3dsx (mode binaire « * »)");
    verifier(maj_somme(sommes, "3dssavesync", h) == -1, "nom absent");
    verifier(maj_somme("abc  3dssavesync.cia\n", "3dssavesync.cia", h) == -1, "hash trop court");
    verifier(maj_somme("", "3dssavesync.cia", h) == -1, "fichier vide");

    verifier(maj_a_verifier(-1, 1793710800LL), "jamais vérifié");
    verifier(!maj_a_verifier(20760, 20760LL * 86400), "même jour, minuit");
    verifier(!maj_a_verifier(20760, 20760LL * 86400 + 86399), "même jour, 23:59:59");
    verifier(maj_a_verifier(20760, 20761LL * 86400), "jour suivant");
    verifier(maj_a_verifier(20760, 20700LL * 86400), "horloge reculée");

    printf("%d/%d cas OK\n", total - echecs, total);
    return echecs != 0;
}
