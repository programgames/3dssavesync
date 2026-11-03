/* Textes calculés de l'écran (texte.h). */
#include <stdio.h>
#include <string.h>
#include "text.h"

const char *pluriel(long n, int anglais, const char *un, const char *plusieurs)
{
    return (anglais ? n == 1 : n < 2) ? un : plusieurs; /* français : « 0 conflit », anglais : "0 conflicts" */
}

void format_age(long s, int anglais, int majuscule, char *out, size_t n)
{
    if (s < 0) snprintf(out, n, "%s", "");
    else if (s < 60) snprintf(out, n, "%s", anglais ? "just now" : "à l'instant");
    else if (s < 3600) snprintf(out, n, anglais ? "%ld min ago" : "il y a %ld min", s / 60);
    else if (s < 86400) snprintf(out, n, anglais ? "%ld h ago" : "il y a %ld h", s / 3600);
    else snprintf(out, n, anglais ? "%ld d ago" : "il y a %ld j", s / 86400);
    if (!majuscule) return;
    if (out[0] >= 'a' && out[0] <= 'z') out[0] -= 'a' - 'A';
    else if (!strncmp(out, "à", 2)) out[1] = (char)0x80; /* à -> À en UTF-8 */
}

int textes_tenant(const float *l, int n, float ecart, float place)
{
    float total = 0;
    int i;
    for (i = 0; i < n; i++) {
        total += l[i] * MARGE_LARGEUR + (i ? ecart : 0);
        if (total > place) break;
    }
    return i;
}
