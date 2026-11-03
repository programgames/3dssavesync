#include <ctype.h>
#include <stdlib.h>
#include <string.h>
#include "update.h"

/* Texte JSON après "clé": (espaces permis), sans échappement. 0 si lu en entier dans out.
 * ponytail: lecture à la main comme /saves (liste.c) ; tag_name et adresses n'ont jamais d'échappement. */
static int valeur(const char *p, char *out, size_t n)
{
    size_t i = 0;
    while (*p == ' ') p++;
    if (*p++ != '"') return -1;
    while (*p && *p != '"' && *p != '\\' && i + 1 < n) out[i++] = *p++;
    out[i] = '\0';
    return *p == '"' ? 0 : -1;
}

int maj_release(const char *json, release_t *r)
{
    static const char TAG[] = "\"tag_name\":", URL[] = "\"browser_download_url\":";
    const char *p = strstr(json, TAG);

    memset(r, 0, sizeof *r);
    if (!p || valeur(p + sizeof TAG - 1, r->version, sizeof r->version) || !r->version[0]) return -1;
    if (r->version[0] == 'v') memmove(r->version, r->version + 1, strlen(r->version));
    for (p = strstr(json, URL); p; p = strstr(p + 1, URL)) {
        char url[512], *nom;
        if (valeur(p + sizeof URL - 1, url, sizeof url)) continue;
        nom = strrchr(url, '/') ? strrchr(url, '/') + 1 : url;
        if (!strcmp(nom, "3dssavesync.cia")) strcpy(r->cia, url);
        else if (!strcmp(nom, "3dssavesync.3dsx")) strcpy(r->dsx, url);
        else if (!strcmp(nom, "SHA256SUMS")) strcpy(r->sommes, url);
    }
    return 0;
}

/* "v3.10.2-rc.1" -> { 3, 10, 2 }, *pre = 1. 0 si lu. */
static int lire_version(const char *s, long n[3], int *pre)
{
    int i;
    char *fin;
    if (*s == 'v') s++;
    for (i = 0; i < 3; i++) {
        if (!isdigit((unsigned char)*s)) return -1;
        n[i] = strtol(s, &fin, 10);
        s = fin;
        if (i < 2 && *s++ != '.') return -1;
    }
    *pre = *s == '-';
    return *s && !*pre ? -1 : 0;
}

int maj_plus_recente(const char *courante, const char *distante)
{
    long a[3], b[3];
    int pa, pb, i;
    if (lire_version(courante, a, &pa) || lire_version(distante, b, &pb)) return 0;
    for (i = 0; i < 3; i++)
        if (a[i] != b[i]) return b[i] > a[i];
    return pa && !pb;
}

int maj_somme(const char *sommes, const char *nom, char hash[65])
{
    const char *l;
    size_t n = strlen(nom);
    for (l = sommes; l && *l; l = strchr(l, '\n') ? strchr(l, '\n') + 1 : NULL) {
        const char *f = l + 64;
        int i;
        for (i = 0; i < 64 && isxdigit((unsigned char)l[i]); i++);
        if (i < 64 || f[0] != ' ' || (f[1] != ' ' && f[1] != '*')) continue;
        if (strncmp(f + 2, nom, n) || (f[2 + n] && f[2 + n] != '\n' && f[2 + n] != '\r')) continue;
        for (i = 0; i < 64; i++) hash[i] = (char)tolower((unsigned char)l[i]);
        hash[64] = '\0';
        return 0;
    }
    return -1;
}

int maj_a_verifier(long dernier_jour, long long maintenant)
{
    return dernier_jour != (long)(maintenant / 86400);
}
