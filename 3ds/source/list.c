#include <stdlib.h>
#include <string.h>
#include "list.h"

static const char *espaces(const char *p)
{
    while (*p == ' ' || *p == '\t' || *p == '\r' || *p == '\n') p++;
    return p;
}

/* Texte JSON (p sur le guillemet ouvrant) dans out (NULL : ignoré), tronqué à n ; "\x" donne x (\u et les autres
 * échappements ne servent à aucun champ lu). Suite après le guillemet fermant, NULL si mal formé. */
static const char *chaine(const char *p, char *out, size_t n)
{
    size_t i = 0;
    if (*p++ != '"') return NULL;
    for (; *p && *p != '"'; p++) {
        if (*p == '\\' && !*++p) return NULL;
        if (out && i + 1 < n) out[i++] = *p;
    }
    if (out) out[i] = '\0';
    return *p ? p + 1 : NULL;
}

/* ponytail: valeurs texte ou nombre seulement (le format de server.py) ; un objet ou un tableau en valeur arrête la
 * lecture, un vrai parseur (jsmn) s'il en faut un jour. */
int liste_serveur(const char **p, entree_t *e)
{
    const char *q = strchr(*p, '{');
    memset(e, 0, sizeof *e);
    e->taille = e->age = -1;
    if (!q) return 0;
    for (q = espaces(q + 1); *q != '}';) {
        char cle[16];
        if (!(q = chaine(q, cle, sizeof cle))) return 0;
        q = espaces(q);
        if (*q != ':') return 0;
        q = espaces(q + 1);
        if (*q == '"') {
            if (!strcmp(cle, "code")) q = chaine(q, e->code, sizeof e->code);
            else if (!strcmp(cle, "hash")) q = chaine(q, e->hash, sizeof e->hash);
            else if (!strcmp(cle, "date")) q = chaine(q, e->date, sizeof e->date);
            else if (!strcmp(cle, "client")) q = chaine(q, e->client, sizeof e->client);
            else {
                e->a_nom |= !strcmp(cle, "nom");
                q = chaine(q, NULL, 0);
            }
            if (!q) return 0;
        } else {
            if (!strcmp(cle, "taille")) e->taille = strtol(q, NULL, 10);
            else if (!strcmp(cle, "age")) e->age = strtol(q, NULL, 10);
            q += strcspn(q, ",}");
        }
        q = espaces(q);
        if (*q == ',') q = espaces(q + 1);
        else if (*q != '}') return 0;
    }
    if (strlen(e->hash) != 64 || strspn(e->hash, "0123456789abcdef") != 64) e->hash[0] = '\0';
    *p = q + 1;
    return 1;
}
