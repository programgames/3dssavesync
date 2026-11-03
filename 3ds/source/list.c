#include <stdlib.h>
#include <string.h>
#include "list.h"

static const char *skip_spaces(const char *p)
{
    while (*p == ' ' || *p == '\t' || *p == '\r' || *p == '\n') p++;
    return p;
}

/* Texte JSON (p sur le guillemet ouvrant) dans out (NULL : ignoré), tronqué à n ; "\x" donne x (\u et les autres
 * échappements ne servent à aucun champ lu). Suite après le guillemet fermant, NULL si mal formé. */
static const char *json_string(const char *p, char *out, size_t n)
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
int next_server_entry(const char **p, entry_t *e)
{
    const char *q = strchr(*p, '{');
    memset(e, 0, sizeof *e);
    e->size = e->age = -1;
    if (!q) return 0;
    for (q = skip_spaces(q + 1); *q != '}';) {
        char key[16];
        if (!(q = json_string(q, key, sizeof key))) return 0;
        q = skip_spaces(q);
        if (*q != ':') return 0;
        q = skip_spaces(q + 1);
        if (*q == '"') {
            if (!strcmp(key, "code")) q = json_string(q, e->code, sizeof e->code);
            else if (!strcmp(key, "hash")) q = json_string(q, e->hash, sizeof e->hash);
            else if (!strcmp(key, "date")) q = json_string(q, e->date, sizeof e->date);
            else if (!strcmp(key, "client")) q = json_string(q, e->client, sizeof e->client);
            else {
                e->has_name |= !strcmp(key, "nom");
                q = json_string(q, NULL, 0);
            }
            if (!q) return 0;
        } else {
            if (!strcmp(key, "taille")) e->size = strtol(q, NULL, 10);
            else if (!strcmp(key, "age")) e->age = strtol(q, NULL, 10);
            q += strcspn(q, ",}");
        }
        q = skip_spaces(q);
        if (*q == ',') q = skip_spaces(q + 1);
        else if (*q != '}') return 0;
    }
    if (strlen(e->hash) != 64 || strspn(e->hash, "0123456789abcdef") != 64) e->hash[0] = '\0';
    *p = q + 1;
    return 1;
}
