#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include "files.h"
#include "settings.h"

static const title_t DEFAULT_TITLES[] = {
    { 0x0004000000055D00ULL, "Pokemon X" },
    { 0x0004000000055E00ULL, "Pokemon Y" },
    { 0x000400000011C400ULL, "Pokemon Rubis Omega" },
    { 0x000400000011C500ULL, "Pokemon Saphir Alpha" },
    { 0x0004000000164800ULL, "Pokemon Soleil" },
    { 0x0004000000175E00ULL, "Pokemon Lune" },
    { 0x00040000001B5000ULL, "Pokemon Ultra-Soleil" },
    { 0x00040000001B5100ULL, "Pokemon Ultra-Lune" },
};

int default_titles(title_t *t, int max)
{
    int n = (int)(sizeof DEFAULT_TITLES / sizeof *DEFAULT_TITLES);
    if (n > max) n = max;
    memcpy(t, DEFAULT_TITLES, n * sizeof *t);
    return n;
}

int read_titles(const char *path, title_t *t, int max)
{
    FILE *f = fopen(path, "r");
    char line[128];
    int n = 0;
    if (!f) { /* coupure entre le retrait de l'ancien et le renommage (replace_file) : le .tmp est complet */
        snprintf(line, sizeof line, "%s.tmp", path);
        f = fopen(line, "r");
    }
    if (!f) return default_titles(t, max);
    while (n < max && fgets(line, sizeof line, f)) {
        char *eq = strchr(line, '='), *end;
        unsigned long long id;
        line[strcspn(line, "\r\n")] = '\0';
        if (!eq) continue;
        *eq = '\0';
        id = strtoull(line, &end, 16);
        if (end == line || *end || strlen(line) != 16 || title_index(t, n, id) >= 0) continue;
        t[n].id = id;
        snprintf(t[n].name, sizeof t[n].name, "%s", eq + 1);
        n++;
    }
    fclose(f);
    return n;
}

int write_titles(const char *path, const title_t *t, int n)
{
    char text[MAX_TITLES * 66]; /* "0004000000055D00=" + nom (47 au plus) + "\n" par jeu */
    size_t l = 0;
    int i;
    if (n > MAX_TITLES) return -1;
    for (i = 0; i < n; i++) l += snprintf(text + l, sizeof text - l, "%016llX=%s\n", t[i].id, t[i].name);
    /* .tmp écrit et relu, puis renommé : une coupure ne vide jamais titres.ini (read_titles lit le .tmp au besoin) */
    return replace_file(path, text, l) ? -1 : 0;
}

int title_index(const title_t *t, int n, unsigned long long id)
{
    int i;
    for (i = 0; i < n; i++)
        if (t[i].id == id) return i;
    return -1;
}

static int exists(const char *path)
{
    FILE *f = fopen(path, "rb");
    if (f) fclose(f);
    return f != NULL;
}

int migrate_dir(const char *old_dir, const char *new_dir)
{
    static const char *const FILES[] = { "config.ini", "base.ini", "titres.ini" };
    char src[512], dst[512];
    unsigned char *o;
    size_t size;
    int i;

    snprintf(src, sizeof src, "%s/config.ini", old_dir);
    if (!exists(src)) return 0;
    /* Un seul fichier du nouveau dossier, ou son .tmp (coupure pendant replace_file) : installation déjà faite,
     * jamais remplacée par les anciens fichiers. */
    for (i = 0; i < 6; i++) {
        snprintf(dst, sizeof dst, "%s/%s%s", new_dir, FILES[i / 2], i % 2 ? ".tmp" : "");
        if (exists(dst)) return 0;
    }
    mkdir(new_dir, 0777);
    for (i = 0; i < 3; i++) {
        snprintf(src, sizeof src, "%s/%s", old_dir, FILES[i]);
        snprintf(dst, sizeof dst, "%s/%s", new_dir, FILES[i]);
        if (read_file(src, &o, &size)) continue; /* base.ini ou titres.ini absents : rien à copier */
        if (write_file(dst, o, size)) {
            free(o);
            return -1;
        }
        free(o);
    }
    return 1;
}
