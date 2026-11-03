#include <stdlib.h>
#include <string.h>
#include "list.h"

static const char *skip_spaces(const char *p)
{
    while (*p == ' ' || *p == '\t' || *p == '\r' || *p == '\n') p++;
    return p;
}

/* JSON string (p on the opening quote) into out (NULL: skipped), truncated to n; "\x" gives x (\u and the other
 * escapes are used by no field we read). Returns what follows the closing quote, NULL if malformed. */
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

/* ponytail: string or number values only (the server.py format); an object or array value stops the
 * parse, a real parser (jsmn) if one is ever needed. */
#define VERSION_CHARS "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"

int next_server_entry(const char **p, entry_t *e)
{
    const char *q = strchr(*p, '{');
    size_t n;
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
            else if (!strcmp(key, "version")) q = json_string(q, e->version, sizeof e->version);
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
    n = strlen(e->version); /* goes into a URL: "<seq>-<date>-<client>-<hash>.sav", with the hash of the entry */
    if (!e->hash[0] || n < 69 || strspn(e->version, VERSION_CHARS) != n - 4 || strcmp(e->version + n - 4, ".sav") ||
        e->version[n - 69] != '-' || strncmp(e->version + n - 68, e->hash, 64))
        e->version[0] = '\0';
    *p = q + 1;
    return 1;
}
