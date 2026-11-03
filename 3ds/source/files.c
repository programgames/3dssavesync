#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include "files.h"

int read_file(const char *path, unsigned char **bytes, size_t *size)
{
    FILE *f = fopen(path, "rb");
    long n;
    if (!f) return -1;
    fseek(f, 0, SEEK_END);
    n = ftell(f);
    fseek(f, 0, SEEK_SET);
    *bytes = malloc(n > 0 ? n : 1);
    if (!*bytes || fread(*bytes, 1, n, f) != (size_t)n) {
        fclose(f);
        free(*bytes);
        return -1;
    }
    fclose(f);
    *size = n;
    return 0;
}

int write_file(const char *path, const void *bytes, size_t size)
{
    FILE *f = fopen(path, "wb");
    int ok;
    if (!f) return -1;
    ok = fwrite(bytes, 1, size, f) == size;
    return (fclose(f) == 0 && ok) ? 0 : -1;
}

/* 1 si chemin contient exactement octets. */
static int same_content(const char *path, const unsigned char *bytes, size_t size)
{
    unsigned char *data;
    size_t n;
    int ok;
    if (read_file(path, &data, &n)) return 0;
    ok = n == size && !memcmp(data, bytes, n);
    free(data);
    return ok;
}

int write_verified(const char *path, const void *bytes, size_t size)
{
    return write_file(path, bytes, size) || !same_content(path, bytes, size) ? -1 : 0;
}

int replace_file(const char *path, const void *bytes, size_t size)
{
    char tmp[700];
    snprintf(tmp, sizeof tmp, "%s.tmp", path);
    if (write_verified(tmp, bytes, size)) {
        remove(tmp);
        return -1;
    }
    remove(path); /* rename() refuse d'écraser sur la SD */
    return rename(tmp, path) || !same_content(path, bytes, size) ? -2 : 0;
}

int replace_program(const char *path, const void *bytes, size_t size)
{
    char tmp[700], bak[700];
    FILE *f;
    snprintf(tmp, sizeof tmp, "%s.tmp", path);
    snprintf(bak, sizeof bak, "%s.bak", path);
    if (write_verified(tmp, bytes, size) || !(f = fopen(path, "rb"))) { /* chemin absent : .bak est peut-être la seule copie */
        remove(tmp);
        return -1;
    }
    fclose(f);
    remove(bak); /* reste d'une mise à jour d'avant : chemin est là */
    if (rename(path, bak)) {
        remove(tmp);
        return -1;
    }
    if (!rename(tmp, path) && same_content(path, bytes, size)) {
        remove(bak);
        return 0;
    }
    remove(path);
    if (rename(bak, path)) return -2;
    remove(tmp);
    return -1;
}

int launched_from_sd(const char *argv0)
{
    size_t n = strlen(argv0);
    return n > 11 && !strncmp(argv0, "sdmc:/", 6) && !strcasecmp(argv0 + n - 5, ".3dsx");
}

int move_save(const char *src, const char *dst, const char *backup, int remove_src)
{
    unsigned char *bytes, *old_data;
    size_t size, n;
    FILE *f;
    int r = -1;

    if (read_file(src, &bytes, &size)) return -1;
    if ((f = fopen(dst, "rb"))) { /* dst existe : copie de sécurité d'abord */
        fclose(f);
        if (read_file(dst, &old_data, &n)) goto end;
        r = write_verified(backup, old_data, n);
        free(old_data);
        if (r) goto end;
        r = -1;
    }
    if (replace_file(dst, bytes, size)) goto end; /* dst déjà copié */
    if (remove_src) remove(src);
    r = 0;
end:
    free(bytes);
    return r;
}

int backup_path(const char *dir, const char *name, const char *date, const char *ext, char *out, size_t n)
{
    FILE *f;
    int i, r;
    for (i = 0; i < 1000; i++) {
        r = i ? snprintf(out, n, "%s/%s-%s-%d.%s", dir, name, date, i, ext) : snprintf(out, n, "%s/%s-%s.%s", dir, name, date, ext);
        if (r < 0 || (size_t)r >= n) return -1;
        if (!(f = fopen(out, "rb"))) return 0;
        fclose(f);
    }
    return -1;
}

int save_path(const char *roms, const char *saves, const char *dir, const char *name, char *out, size_t n)
{
    size_t l = strlen(roms);
    int r;
    if (!saves[0] || !strcasecmp(saves, roms)) r = snprintf(out, n, "%s/%s.sav", dir, name);
    else if (!strncasecmp(saves, roms, l) && !strcasecmp(saves + l, "/saves")) r = snprintf(out, n, "%s/saves/%s.sav", dir, name);
    else r = snprintf(out, n, "%s/%s.sav", saves, name);
    return r < 0 || (size_t)r >= n ? -1 : 0;
}

int same_save(system_t a, const char *name_a, const char *sav_a, system_t b, const char *name_b, const char *sav_b)
{
    int forwarder = (a == SYS_GB || a == SYS_GBA) && (b == SYS_GB || b == SYS_GBA);
    return !strcasecmp(sav_a, sav_b) || (forwarder && !strcasecmp(name_a, name_b));
}
