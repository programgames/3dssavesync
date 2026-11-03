/* Vérifie etat() contre la table partagée tests/cas_etat.csv ("-" = absent).
 * Compilé sur PC : gcc -I source source/state.c test/test_state.c -o test_state && ./test_state ../tests/cas_etat.csv */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "state.h"

static const char *NAMES[] = { "A_JOUR", "A_ENVOYER", "A_RECUPERER", "CONFLIT" };

int main(int argc, char **argv)
{
    FILE *f = fopen(argc > 1 ? argv[1] : "../tests/cas_etat.csv", "r");
    char line[512];
    int total = 0, failures = 0;

    if (!f) { perror("cas_etat.csv"); return 1; }
    fgets(line, sizeof line, f); /* en-tête */
    while (fgets(line, sizeof line, f)) {
        char *c[7];
        int n = 0;
        line[strcspn(line, "\r\n")] = '\0';
        if (!*line) continue;
        for (char *p = strtok(line, ";"); p && n < 7; p = strtok(NULL, ";")) c[n++] = p;
        if (n != 7) { printf("ligne invalide\n"); return 1; }

        obs_t local = { c[1], strtoul(c[2], NULL, 10) };
        obs_t server = { c[3], strtoul(c[4], NULL, 10) };
        state_t e = sync_state(strcmp(c[1], "-") ? &local : NULL,
                        strcmp(c[3], "-") ? &server : NULL,
                        strcmp(c[5], "-") ? c[5] : NULL);
        total++;
        if (strcmp(NAMES[e], c[6])) {
            printf("ECHEC  %s : attendu %s, obtenu %s\n", c[0], c[6], NAMES[e]);
            failures++;
        }
    }
    fclose(f);
    printf("%d/%d cas OK\n", total - failures, total);
    return failures != 0;
}
