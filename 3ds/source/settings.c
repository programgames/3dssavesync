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

int read_ini(const char *path, void (*entry)(void *ctx, const char *key, const char *value), void *ctx)
{
    FILE *f = fopen(path, "r");
    char line[512];
    if (!f) { /* power cut between the removal of the old file and the rename (replace_file): the .tmp is complete */
        snprintf(line, sizeof line, "%s.tmp", path);
        f = fopen(line, "r");
    }
    if (!f) return -1;
    while (fgets(line, sizeof line, f)) {
        char *eq;
        line[strcspn(line, "\r\n")] = '\0';
        if ((eq = strchr(line, '='))) {
            *eq = '\0';
            entry(ctx, line, eq + 1);
        }
    }
    fclose(f);
    return 0;
}

static void config_entry(void *ctx, const char *k, const char *v)
{
    static const char *const KEYS[] = { "url", "jeton", "roms", "saves", "roms_gb", "saves_gb", "roms_gba", "saves_gba" };
    config_t *c = ctx;
    char *fields[] = { c->url, c->token, c->roms, c->saves, c->roms_gb, c->saves_gb, c->roms_gba, c->saves_gba };
    size_t i;
    for (i = 0; i < sizeof KEYS / sizeof *KEYS; i++)
        if (!strcmp(k, KEYS[i])) snprintf(fields[i], sizeof c->url, "%s", v); /* all fields have the size of url */
}

void read_config(const char *path, config_t *c)
{
    memset(c, 0, sizeof *c);
    snprintf(c->roms, sizeof c->roms, "sdmc:/roms/nds");
    snprintf(c->saves, sizeof c->saves, "sdmc:/roms/nds/saves");
    snprintf(c->roms_gb, sizeof c->roms_gb, "sdmc:/roms/gb");   /* TWiLight: .sav next to the ROMs */
    snprintf(c->roms_gba, sizeof c->roms_gba, "sdmc:/roms/gba"); /* open_agb_firm, useSavesFolder=false */
    read_ini(path, config_entry, c);
}

int write_config(const char *path, const config_t *c)
{
    char t[sizeof *c + 100];
    int n = snprintf(t, sizeof t, "url=%s\njeton=%s\nroms=%s\nsaves=%s\nroms_gb=%s\nsaves_gb=%s\nroms_gba=%s\nsaves_gba=%s\n", c->url, c->token,
                     c->roms, c->saves, c->roms_gb, c->saves_gb, c->roms_gba, c->saves_gba);
    return replace_file(path, t, (size_t)n) ? -1 : 0;
}

static void base_entry(void *ctx, const char *k, const char *v)
{
    bases_t *b = ctx;
    if (b->n < MAX_BASES && strlen(k) >= 4 && strlen(k) < KEY_MAX && strlen(v) == 64) {
        snprintf(b->e[b->n].key, KEY_MAX, "%s", k);
        snprintf(b->e[b->n].hash, 65, "%s", v);
        b->n++;
    }
}

void read_bases(const char *path, bases_t *b)
{
    b->n = 0;
    read_ini(path, base_entry, b);
}

const char *base_of(const bases_t *b, const char *key)
{
    int i;
    for (i = 0; i < b->n; i++)
        if (!strcmp(b->e[i].key, key)) return b->e[i].hash;
    return NULL;
}

int set_base(const char *path, bases_t *b, const char *key, const char *hash)
{
    static char t[MAX_BASES * (KEY_MAX + 66)];
    size_t n = 0;
    int i;
    for (i = 0; i < b->n && strcmp(b->e[i].key, key); i++);
    if (i == b->n) {
        if (b->n == MAX_BASES) return 0;
        snprintf(b->e[b->n++].key, KEY_MAX, "%s", key);
    }
    snprintf(b->e[i].hash, 65, "%s", hash);
    for (i = 0; i < b->n; i++) n += snprintf(t + n, sizeof t - n, "%s=%s\n", b->e[i].key, b->e[i].hash);
    return replace_file(path, t, n) ? -1 : 0;
}

static void alias_entry(void *ctx, const char *k, const char *v)
{
    aliases_t *a = ctx;
    size_t n = strlen(k);
    while (n && k[n - 1] == ' ') n--; /* "Pokemon Radical Red = GBA-0RAD" */
    if (a->n == MAX_ALIAS || !n || n >= sizeof a->e[0].name) return;
    snprintf(a->e[a->n].name, sizeof a->e[0].name, "%.*s", (int)n, k);
    snprintf(a->e[a->n].value, sizeof a->e[0].value, "%s", v); /* cut: alias_key will reject it */
    a->n++;
}

void read_aliases(const char *path, aliases_t *a)
{
    a->n = 0;
    read_ini(path, alias_entry, a);
}

const char *alias_of(const aliases_t *a, const char *name)
{
    int i;
    for (i = 0; i < a->n; i++)
        if (!strcmp(a->e[i].name, name)) return a->e[i].value;
    return NULL;
}

static void update_entry(void *ctx, const char *k, const char *v)
{
    update_info_t *u = ctx;
    if (!strcmp(k, "jour")) u->day = strtol(v, NULL, 10);
    else if (!strcmp(k, "version")) snprintf(u->latest, sizeof u->latest, "%s", v);
    else if (!strcmp(k, "installee")) snprintf(u->installed, sizeof u->installed, "%s", v);
}

void read_update_info(const char *path, update_info_t *u)
{
    memset(u, 0, sizeof *u);
    u->day = -1;
    read_ini(path, update_entry, u);
}

int write_update_info(const char *path, const update_info_t *u, const char *running)
{
    char t[120];
    int n = snprintf(t, sizeof t, "jour=%ld\nversion=%s\ninstallee=%s\n", u->day, u->latest, running);
    return replace_file(path, t, (size_t)n) ? -1 : 0;
}

int default_titles(title_t *t, int max)
{
    int n = (int)(sizeof DEFAULT_TITLES / sizeof *DEFAULT_TITLES);
    if (n > max) n = max;
    memcpy(t, DEFAULT_TITLES, n * sizeof *t);
    return n;
}

typedef struct {
    title_t *t;
    int n, max;
} titles_read_t;

static void title_entry(void *ctx, const char *k, const char *v)
{
    titles_read_t *r = ctx;
    unsigned long long id;
    char *end;
    if (r->n == r->max) return;
    id = strtoull(k, &end, 16);
    if (end == k || *end || strlen(k) != 16 || title_index(r->t, r->n, id) >= 0) return;
    r->t[r->n].id = id;
    snprintf(r->t[r->n].name, sizeof r->t[r->n].name, "%s", v);
    r->n++;
}

int read_titles(const char *path, title_t *t, int max)
{
    titles_read_t r = { t, 0, max };
    return read_ini(path, title_entry, &r) ? default_titles(t, max) : r.n;
}

int write_titles(const char *path, const title_t *t, int n)
{
    char text[MAX_TITLES * 66]; /* "0004000000055D00=" + name (47 at most) + "\n" per game */
    size_t l = 0;
    int i;
    if (n > MAX_TITLES) return -1;
    for (i = 0; i < n; i++) l += snprintf(text + l, sizeof text - l, "%016llX=%s\n", t[i].id, t[i].name);
    /* .tmp written and read back, then renamed: a power cut never empties titres.ini (read_titles reads the .tmp if needed) */
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
    /* A single file of the new folder, or its .tmp (power cut during replace_file): install already done,
     * never replaced by the old files. */
    for (i = 0; i < 6; i++) {
        snprintf(dst, sizeof dst, "%s/%s%s", new_dir, FILES[i / 2], i % 2 ? ".tmp" : "");
        if (exists(dst)) return 0;
    }
    mkdir(new_dir, 0777);
    for (i = 0; i < 3; i++) {
        snprintf(src, sizeof src, "%s/%s", old_dir, FILES[i]);
        snprintf(dst, sizeof dst, "%s/%s", new_dir, FILES[i]);
        if (read_file(src, &o, &size)) continue; /* base.ini or titres.ini missing: nothing to copy */
        if (write_file(dst, o, size)) {
            free(o);
            return -1;
        }
        free(o);
    }
    return 1;
}
