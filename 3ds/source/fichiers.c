#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
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

int ecrire_relu(const char *chemin, const void *octets, size_t taille)
{
    return ecrire_fichier(chemin, octets, taille) || !identique(chemin, octets, taille) ? -1 : 0;
}

int remplacer_fichier(const char *chemin, const void *octets, size_t taille)
{
    char tmp[700];
    snprintf(tmp, sizeof tmp, "%s.tmp", chemin);
    if (ecrire_relu(tmp, octets, taille)) {
        remove(tmp);
        return -1;
    }
    remove(chemin); /* rename() refuse d'écraser sur la SD */
    return rename(tmp, chemin) || !identique(chemin, octets, taille) ? -2 : 0;
}

int remplacer_programme(const char *chemin, const void *octets, size_t taille)
{
    char tmp[700], bak[700];
    FILE *f;
    snprintf(tmp, sizeof tmp, "%s.tmp", chemin);
    snprintf(bak, sizeof bak, "%s.bak", chemin);
    if (ecrire_relu(tmp, octets, taille) || !(f = fopen(chemin, "rb"))) { /* chemin absent : .bak est peut-être la seule copie */
        remove(tmp);
        return -1;
    }
    fclose(f);
    remove(bak); /* reste d'une mise à jour d'avant : chemin est là */
    if (rename(chemin, bak)) {
        remove(tmp);
        return -1;
    }
    if (!rename(tmp, chemin) && identique(chemin, octets, taille)) {
        remove(bak);
        return 0;
    }
    remove(chemin);
    if (rename(bak, chemin)) return -2;
    remove(tmp);
    return -1;
}

int lance_depuis_sd(const char *argv0)
{
    size_t n = strlen(argv0);
    return n > 11 && !strncmp(argv0, "sdmc:/", 6) && !strcasecmp(argv0 + n - 5, ".3dsx");
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
        r = ecrire_relu(copie, ancien, n);
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

int chemin_sauvegarde(const char *roms, const char *saves, const char *dossier, const char *nom, char *out, size_t n)
{
    size_t l = strlen(roms);
    int r;
    if (!saves[0] || !strcasecmp(saves, roms)) r = snprintf(out, n, "%s/%s.sav", dossier, nom);
    else if (!strncasecmp(saves, roms, l) && !strcasecmp(saves + l, "/saves")) r = snprintf(out, n, "%s/saves/%s.sav", dossier, nom);
    else r = snprintf(out, n, "%s/%s.sav", saves, nom);
    return r < 0 || (size_t)r >= n ? -1 : 0;
}

int meme_sauvegarde(systeme_t a, const char *nom_a, const char *sav_a, systeme_t b, const char *nom_b, const char *sav_b)
{
    int forwarder = (a == SYS_GB || a == SYS_GBA) && (b == SYS_GB || b == SYS_GBA);
    return !strcasecmp(sav_a, sav_b) || (forwarder && !strcasecmp(nom_a, nom_b));
}
