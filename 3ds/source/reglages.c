#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include "fichiers.h"
#include "reglages.h"

static const titre_t DEFAUT[] = {
    { 0x0004000000055D00ULL, "Pokemon X" },
    { 0x0004000000055E00ULL, "Pokemon Y" },
    { 0x000400000011C400ULL, "Pokemon Rubis Omega" },
    { 0x000400000011C500ULL, "Pokemon Saphir Alpha" },
    { 0x0004000000164800ULL, "Pokemon Soleil" },
    { 0x0004000000175E00ULL, "Pokemon Lune" },
    { 0x00040000001B5000ULL, "Pokemon Ultra-Soleil" },
    { 0x00040000001B5100ULL, "Pokemon Ultra-Lune" },
};

int titres_defaut(titre_t *t, int max)
{
    int n = (int)(sizeof DEFAUT / sizeof *DEFAUT);
    if (n > max) n = max;
    memcpy(t, DEFAUT, n * sizeof *t);
    return n;
}

int titres_lire(const char *chemin, titre_t *t, int max)
{
    FILE *f = fopen(chemin, "r");
    char ligne[128];
    int n = 0;
    if (!f) return titres_defaut(t, max);
    while (n < max && fgets(ligne, sizeof ligne, f)) {
        char *egal = strchr(ligne, '='), *fin;
        unsigned long long id;
        ligne[strcspn(ligne, "\r\n")] = '\0';
        if (!egal) continue;
        *egal = '\0';
        id = strtoull(ligne, &fin, 16);
        if (fin == ligne || *fin || strlen(ligne) != 16 || titres_index(t, n, id) >= 0) continue;
        t[n].id = id;
        snprintf(t[n].nom, sizeof t[n].nom, "%s", egal + 1);
        n++;
    }
    fclose(f);
    return n;
}

int titres_ecrire(const char *chemin, const titre_t *t, int n)
{
    FILE *f = fopen(chemin, "w");
    int i, ok;
    if (!f) return -1;
    for (i = 0; i < n; i++) fprintf(f, "%016llX=%s\n", t[i].id, t[i].nom);
    ok = !ferror(f);
    return fclose(f) == 0 && ok ? 0 : -1;
}

int titres_index(const titre_t *t, int n, unsigned long long id)
{
    int i;
    for (i = 0; i < n; i++)
        if (t[i].id == id) return i;
    return -1;
}

static int existe(const char *chemin)
{
    FILE *f = fopen(chemin, "rb");
    if (f) fclose(f);
    return f != NULL;
}

int migrer_dossier(const char *ancien, const char *nouveau)
{
    static const char *const FICHIERS[] = { "config.ini", "base.ini", "titres.ini" };
    char src[512], dst[512];
    unsigned char *o;
    size_t taille;
    int i;

    snprintf(dst, sizeof dst, "%s/config.ini", nouveau);
    snprintf(src, sizeof src, "%s/config.ini", ancien);
    if (existe(dst) || !existe(src)) return 0;
    mkdir(nouveau, 0777);
    for (i = 0; i < 3; i++) {
        snprintf(src, sizeof src, "%s/%s", ancien, FICHIERS[i]);
        snprintf(dst, sizeof dst, "%s/%s", nouveau, FICHIERS[i]);
        if (lire_fichier(src, &o, &taille)) continue; /* base.ini ou titres.ini absents : rien à copier */
        if (ecrire_fichier(dst, o, taille)) {
            free(o);
            return -1;
        }
        free(o);
    }
    return 1;
}
