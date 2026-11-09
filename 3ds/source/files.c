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

int read_cas(const char *path, const char *root, long max, char **out, size_t *n)
{
    FILE *f = fopen(path, "rb");
    size_t r = strlen(root);
    long size = -1;
    int status = -1;
    if (!f) return 1;
    if (!fseek(f, 0, SEEK_END)) size = ftell(f);
    if (size == 0) status = 1;
    else if (size > max) status = -2;
    else if (size > 0 && !fseek(f, 0, SEEK_SET) && (*out = malloc(r + 1 + size + 1))) {
        if (fread(*out + r + 1, 1, size, f) == (size_t)size) {
            memcpy(*out, root, r);
            (*out)[r] = '\n';
            *n = r + 1 + size;
            (*out)[*n] = '\0';
            status = 0;
        } else free(*out);
    }
    fclose(f);
    return status;
}

int write_file(const char *path, const void *bytes, size_t size)
{
    FILE *f = fopen(path, "wb");
    int ok;
    if (!f) return -1;
    ok = fwrite(bytes, 1, size, f) == size;
    return (fclose(f) == 0 && ok) ? 0 : -1;
}

int append_log(const char *path, const char *line, long max)
{
    char old[620];
    FILE *f = fopen(path, "rb");
    long size = 0;
    int ok;
    if (f) {
        if (!fseek(f, 0, SEEK_END)) size = ftell(f);
        fclose(f);
    }
    if (size > max && snprintf(old, sizeof old, "%s.1", path) < (int)sizeof old) { /* one older file kept, the oldest lines go */
        remove(old);
        rename(path, old);
    }
    if (!(f = fopen(path, "ab"))) return -1;
    ok = fputs(line, f) >= 0 && fputc('\n', f) != EOF;
    return (fclose(f) == 0 && ok) ? 0 : -1;
}

/* 1 if path holds exactly these bytes. Read in pieces: a second copy of a 16 MiB archive does not fit an Old 3DS. */
static int same_content(const char *path, const unsigned char *bytes, size_t size)
{
    static unsigned char piece[0x10000]; /* not on the stack (32 KiB on the 3DS) */
    FILE *f = fopen(path, "rb");
    size_t done = 0, got;
    int ok = f != NULL;
    while (ok && (got = fread(piece, 1, sizeof piece, f)) > 0) {
        ok = got <= size - done && !memcmp(piece, bytes + done, got);
        done += got;
    }
    ok = ok && done == size && !ferror(f);
    if (f) fclose(f);
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
    remove(path); /* rename() refuses to overwrite on the SD card */
    return rename(tmp, path) || !same_content(path, bytes, size) ? -2 : 0;
}

int replace_program(const char *path, const void *bytes, size_t size)
{
    char tmp[700], bak[700];
    FILE *f;
    snprintf(tmp, sizeof tmp, "%s.tmp", path);
    snprintf(bak, sizeof bak, "%s.bak", path);
    if (write_verified(tmp, bytes, size) || !(f = fopen(path, "rb"))) { /* path missing: .bak may be the only copy */
        remove(tmp);
        return -1;
    }
    fclose(f);
    remove(bak); /* left over from an earlier update: path is there */
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
    if ((f = fopen(dst, "rb"))) { /* dst exists: Backup copy first */
        fclose(f);
        if (read_file(dst, &old_data, &n)) goto end;
        r = write_verified(backup, old_data, n);
        free(old_data);
        if (r) goto end;
        r = -1;
    }
    if (replace_file(dst, bytes, size)) goto end; /* dst already copied */
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
