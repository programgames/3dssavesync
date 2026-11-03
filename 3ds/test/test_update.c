/* Checks update.c: versions, GitHub response (test/release_latest.json, a real /releases/latest response, trimmed),
 * SHA256SUMS, one check per day.
 * Built on the PC: gcc -std=c99 -Wall -I source source/files.c source/update.c test/test_update.c -o /tmp/m && /tmp/m test/release_latest.json */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "files.h"
#include "update.h"

static int failures, total;

static void check(int ok, const char *what)
{
    total++;
    if (!ok) {
        printf("ECHEC  %s\n", what);
        failures++;
    }
}

int main(int argc, char **argv)
{
    unsigned char *json;
    size_t size;
    release_t r;
    char h[65];
    const char *checksums = "5f2b1c0e9a8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b3a2f1e0d9c8b7a6f5e4d3c2b  3dssavesync.cia.sig\n"
                         "AB12CD34EF56AB12CD34EF56AB12CD34EF56AB12CD34EF56AB12CD34EF56AB12  3dssavesync.cia\r\n"
                         "0000000000000000000000000000000000000000000000000000000000000001 *3dssavesync.3dsx\n";

    check(update_is_newer("3.9.1", "3.10.0"), "3.10.0 > 3.9.1");
    check(!update_is_newer("3.10.0", "3.9.1"), "3.9.1 < 3.10.0");
    check(update_is_newer("3.6.1", "v3.7.0"), "v3.7.0 > 3.6.1");
    check(!update_is_newer("3.7.0", "3.7.0"), "3.7.0 = 3.7.0");
    check(update_is_newer("3.7.0-rc.1", "3.7.0"), "3.7.0 > 3.7.0-rc.1");
    check(!update_is_newer("3.7.0", "3.7.0-rc.1"), "3.7.0-rc.1 < 3.7.0");
    check(update_is_newer("2.99.99", "3.0.0"), "3.0.0 > 2.99.99");
    check(!update_is_newer("dev", "3.7.0"), "dev : jamais de mise à jour");
    check(!update_is_newer("3.6.1", ""), "version distante vide");
    check(!update_is_newer("3.6.1", "3.7"), "version distante incomplète");
    check(!update_is_newer("3.6.1", "3.7.0x"), "version distante mal formée");

    if (read_file(argc > 1 ? argv[1] : "test/release_latest.json", &json, &size)) {
        printf("release_latest.json illisible\n");
        return 1;
    }
    json = realloc(json, size + 1);
    json[size] = '\0';
    check(parse_release((char *)json, &r) == 0, "release_latest.json lu");
    check(!strcmp(r.version, "3.6.1"), "tag_name v3.6.1 -> 3.6.1");
    check(!strcmp(r.cia, "https://github.com/programgames/3dssavesync/releases/download/v3.6.1/3dssavesync.cia"), "adresse du .cia");
    check(!strcmp(r.dsx, "https://github.com/programgames/3dssavesync/releases/download/v3.6.1/3dssavesync.3dsx"), "adresse du .3dsx");
    check(r.checksums[0] == '\0', "v3.6.1 : pas de SHA256SUMS");
    free(json);

    check(parse_release("{\"body\":\"\\\"tag_name\\\":\\\"v9.9.9\\\"\",\"tag_name\": \"v3.7.0\",\"assets\":[{\"browser_download_url\": "
                         "\"https://github.com/o/r/releases/download/v3.7.0/SHA256SUMS\"}]}", &r) == 0 && !strcmp(r.version, "3.7.0"),
             "tag_name échappé dans body ignoré, espace après les deux-points");
    check(!strcmp(r.checksums, "https://github.com/o/r/releases/download/v3.7.0/SHA256SUMS") && !r.cia[0], "adresse de SHA256SUMS");
    check(parse_release("{\"message\":\"API rate limit exceeded\"}", &r) == -1, "réponse sans tag_name");
    check(parse_release("{\"tag_name\":\"\"}", &r) == -1, "tag_name vide");

    check(find_checksum(checksums, "3dssavesync.cia", h) == 0 && !strcmp(h, "ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12"),
             "somme du .cia (pas celle du .cia.sig), en minuscules, fin de ligne CRLF");
    check(find_checksum(checksums, "3dssavesync.3dsx", h) == 0 && !strcmp(h, "0000000000000000000000000000000000000000000000000000000000000001"),
             "somme du .3dsx (mode binaire « * »)");
    check(find_checksum(checksums, "3dssavesync", h) == -1, "nom absent");
    check(find_checksum("abc  3dssavesync.cia\n", "3dssavesync.cia", h) == -1, "hash trop court");
    check(find_checksum("", "3dssavesync.cia", h) == -1, "fichier vide");

    check(update_check_due(-1, 1793710800LL), "jamais vérifié");
    check(!update_check_due(20760, 20760LL * 86400), "même jour, minuit");
    check(!update_check_due(20760, 20760LL * 86400 + 86399), "même jour, 23:59:59");
    check(update_check_due(20760, 20761LL * 86400), "jour suivant");
    check(update_check_due(20760, 20700LL * 86400), "horloge reculée");

    printf("%d/%d cas OK\n", total - failures, total);
    return failures != 0;
}
