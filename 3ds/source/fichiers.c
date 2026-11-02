#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "fichiers.h"

int lire_fichier(const char *chemin, unsigned char **octets, size_t *taille)
{
    FILE *f = fopen(chemin, "rb");
    long n;
    if (!f) return -1;
    fseek(f, 0, SEEK_END);
    n = ftell(f);
    fseek(f, 0, SEEK_SET);
    *octets = malloc(n > 0 ? n : 1);
    if (!*octets || fread(*octets, 1, n, f) != (size_t)n) {
        fclose(f);
        free(*octets);
        return -1;
    }
    fclose(f);
    *taille = n;
    return 0;
}

int ecrire_fichier(const char *chemin, const void *octets, size_t taille)
{
    FILE *f = fopen(chemin, "wb");
    int ok;
    if (!f) return -1;
    ok = fwrite(octets, 1, taille, f) == taille;
    return (fclose(f) == 0 && ok) ? 0 : -1;
}

/* 1 si chemin contient exactement octets. */
static int identique(const char *chemin, const unsigned char *octets, size_t taille)
{
    unsigned char *lu;
    size_t n;
    int ok;
    if (lire_fichier(chemin, &lu, &n)) return 0;
    ok = n == taille && !memcmp(lu, octets, n);
    free(lu);
    return ok;
}

int remplacer_fichier(const char *chemin, const void *octets, size_t taille)
{
    char tmp[700];
    snprintf(tmp, sizeof tmp, "%s.tmp", chemin);
    if (ecrire_fichier(tmp, octets, taille) || !identique(tmp, octets, taille)) {
        remove(tmp);
        return -1;
    }
    remove(chemin); /* rename() refuse d'écraser sur la SD */
    return rename(tmp, chemin) || !identique(chemin, octets, taille) ? -1 : 0;
}

int deplacer_sauvegarde(const char *src, const char *dst, const char *copie, int supprimer_src)
{
    unsigned char *octets, *ancien;
    size_t taille, n;
    FILE *f;
    int r = -1;

    if (lire_fichier(src, &octets, &taille)) return -1;
    if ((f = fopen(dst, "rb"))) { /* dst existe : copie de sécurité d'abord */
        fclose(f);
        if (lire_fichier(dst, &ancien, &n)) goto fin;
        r = ecrire_fichier(copie, ancien, n) || !identique(copie, ancien, n) ? -1 : 0;
        free(ancien);
        if (r) goto fin;
        r = -1;
    }
    if (remplacer_fichier(dst, octets, taille)) goto fin; /* dst déjà copié */
    if (supprimer_src) remove(src);
    r = 0;
fin:
    free(octets);
    return r;
}
