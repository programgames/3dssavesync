/* Same rules, same bytes as client/archive3ds.go (ADR 0028). */
#include <stdlib.h>
#include <string.h>
#include "archive.h"

#define BLOCK 512

int archive_path_ok(const char *p)
{
    size_t n = strlen(p), start = 0, i;
    if (!n || n > 100 || p[0] == '/' || strchr(p, '\\')) return 0;
    for (i = 0; i <= n; i++) {
        if (i < n && p[i] != '/') continue;
        if (i == start || (i - start == 1 && p[start] == '.') || (i - start == 2 && p[start] == '.' && p[start + 1] == '.')) return 0;
        start = i + 1;
    }
    return 1;
}

/* Data of n bytes padded with zeros to a whole number of blocks. */
static unsigned long long padded(unsigned long long n) { return (n + BLOCK - 1) / BLOCK * BLOCK; }

static int zero(const unsigned char *p, size_t n)
{
    size_t i;
    for (i = 0; i < n; i++)
        if (p[i]) return 0;
    return 1;
}

/* v as len - 1 octal digits (zeros in front), then NUL. */
static void octal(unsigned char *field, size_t len, unsigned long long v)
{
    size_t i = len - 1;
    field[i] = 0;
    while (i--) {
        field[i] = (unsigned char)('0' + (v & 7));
        v >>= 3;
    }
}

/* Value of an octal field: its digits up to the trailing NULs (Go: ParseInt of TrimRight). -1 if none, or another
 * character. */
static long long octal_value(const unsigned char *p, size_t len)
{
    long long v = 0;
    size_t i;
    while (len && !p[len - 1]) len--;
    if (!len) return -1;
    for (i = 0; i < len; i++) {
        if (p[i] < '0' || p[i] > '7') return -1;
        v = v * 8 + (p[i] - '0');
    }
    return v;
}

/* ustar checksum: every byte of the header, the checksum field counted as 8 spaces. */
static unsigned long header_sum(const unsigned char *h)
{
    unsigned long sum = 0;
    size_t i;
    for (i = 0; i < BLOCK; i++) sum += i >= 148 && i < 156 ? ' ' : h[i];
    return sum;
}

/* Header of f in h (zeros): mode 0644, uid, gid and mtime 0, typeflag '0', "ustar\0" "00", devmajor and devminor 0. */
static void write_header(unsigned char *h, const archive_file_t *f)
{
    memcpy(h, f->path, strlen(f->path));
    octal(h + 100, 8, 0644);
    octal(h + 108, 8, 0);
    octal(h + 116, 8, 0);
    octal(h + 124, 12, f->size);
    octal(h + 136, 12, 0);
    h[156] = '0';
    memcpy(h + 257, "ustar", 6); /* with its NUL */
    memcpy(h + 263, "00", 2);
    octal(h + 329, 8, 0);
    octal(h + 337, 8, 0);
    octal(h + 148, 7, header_sum(h)); /* 6 digits and NUL, then a space */
    h[155] = ' ';
}

static int compare_paths(const void *a, const void *b)
{
    return strcmp(((const archive_file_t *)a)->path, ((const archive_file_t *)b)->path);
}

int pack_archive(archive_file_t *files, int n, unsigned char **out, size_t *size, int *bad)
{
    unsigned long long total = 2 * BLOCK;
    unsigned char *a;
    size_t off = 0;
    int i;
    if (n > 1) qsort(files, n, sizeof *files, compare_paths);
    for (i = 0; i < n; i++) {
        if (!archive_path_ok(files[i].path) || (i && !strcmp(files[i - 1].path, files[i].path))) {
            if (bad) *bad = i;
            return -1;
        }
        total += BLOCK + padded(files[i].size);
    }
    if (total > (size_t)-1 || !(a = calloc(1, (size_t)total))) return -2;
    for (i = 0; i < n; i++) {
        write_header(a + off, &files[i]);
        files[i].offset = off + BLOCK;
        if (files[i].data && files[i].size) memcpy(a + files[i].offset, files[i].data, files[i].size);
        off += BLOCK + (size_t)padded(files[i].size);
    }
    *out = a;
    *size = (size_t)total;
    return 0;
}

/* Checks the header h (not a zero block) and puts its file in e (path, size). 0 if it is a header of pack_archive. */
static int read_header(const unsigned char *h, archive_file_t *e)
{
    const unsigned char *end = memchr(h, 0, 100);
    size_t len = end ? (size_t)(end - h) : 100;
    long long size = octal_value(h + 124, 11);
    if (!zero(h + len, 100 - len) || size < 0 || h[156] != '0' || memcmp(h + 257, "ustar", 6) || h[345]) return -1;
    memcpy(e->path, h, len);
    e->path[len] = '\0';
    if (!archive_path_ok(e->path) || octal_value(h + 148, 7) != (long long)header_sum(h)) return -1;
    e->size = (size_t)size;
    return (unsigned long long)size == e->size ? 0 : -1; /* larger than the memory of a 32-bit system */
}

int unpack_archive(unsigned char *data, size_t size, archive_file_t **files, int *n)
{
    archive_file_t *list = NULL, *e;
    int count = 0, cap = 0;
    size_t off = 0;
    for (;;) {
        unsigned char *h;
        if (off > size || size - off < BLOCK) break;
        h = data + off;
        if (zero(h, BLOCK)) {
            if (size - off != 2 * BLOCK || !zero(h + BLOCK, BLOCK)) break;
            *files = list;
            *n = count;
            return 0;
        }
        if (count == cap) {
            archive_file_t *p = realloc(list, (cap = cap ? cap * 2 : 8) * sizeof *list);
            if (!p) {
                free(list);
                return -2;
            }
            list = p;
        }
        e = &list[count];
        if (read_header(h, e) || e->size > size - off - BLOCK || (count && strcmp(list[count - 1].path, e->path) >= 0)) break;
        e->data = h + BLOCK;
        e->offset = off + BLOCK;
        count++;
        off += BLOCK + (size_t)padded(e->size);
    }
    free(list);
    return -1;
}
