/* Vérifie etat() contre la table partagée tests/cas_etat.csv ("-" = absent).
 * Compilé sur PC : gcc -I source source/etat.c test/test_etat.c -o test_etat && ./test_etat ../tests/cas_etat.csv */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "etat.h"

static const char *NOMS[] = { "A_JOUR", "A_ENVOYER", "A_RECUPERER", "CONFLIT" };

int main(int argc, char **argv)
{
    FILE *f = fopen(argc > 1 ? argv[1] : "../tests/cas_etat.csv", "r");
    char ligne[512];
    int total = 0, echecs = 0;

    if (!f) { perror("cas_etat.csv"); return 1; }
    fgets(ligne, sizeof ligne, f); /* en-tête */
    while (fgets(ligne, sizeof ligne, f)) {
        char *c[7];
        int n = 0;
        ligne[strcspn(ligne, "\r\n")] = '\0';
        if (!*ligne) continue;
        for (char *p = strtok(ligne, ";"); p && n < 7; p = strtok(NULL, ";")) c[n++] = p;
        if (n != 7) { printf("ligne invalide\n"); return 1; }

        obs_t local = { c[1], strtoul(c[2], NULL, 10) };
        obs_t serveur = { c[3], strtoul(c[4], NULL, 10) };
        etat_t e = etat(strcmp(c[1], "-") ? &local : NULL,
                        strcmp(c[3], "-") ? &serveur : NULL,
                        strcmp(c[5], "-") ? c[5] : NULL);
        total++;
        if (strcmp(NOMS[e], c[6])) {
            printf("ECHEC  %s : attendu %s, obtenu %s\n", c[0], c[6], NOMS[e]);
            echecs++;
        }
    }
    fclose(f);
    printf("%d/%d cas OK\n", total - echecs, total);
    return echecs != 0;
}
