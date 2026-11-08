/* Checks pack_archive and unpack_archive (ADR 0028) on the shared cases of tests/archive3ds/cases.json: the same SHA-256
 * as client/archive3ds.go, and the same refusals.
 * Built on the PC: gcc -std=c99 -I source source/files.c source/archive.c test/test_archive.c -o /tmp/a && /tmp/a ../tests/archive3ds/cases.json */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "archive.h"
#include "files.h"

static int failures;

static void check(int ok, const char *what)
{
    if (!ok) {
        printf("ECHEC  %s\n", what);
        failures++;
    }
}

/* ---------------------------------------------------------------- SHA-256 (FIPS 180-4; the 3DS uses mbedTLS) */

static unsigned long rotr(unsigned long x, int n) { return ((x >> n) | (x << (32 - n))) & 0xFFFFFFFFUL; }

static void sha256_block(unsigned long h[8], const unsigned char *p)
{
    static const unsigned long K[64] = {
        0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be,
        0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa,
        0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85,
        0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3,
        0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f,
        0x682e6ff3, 0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2 };
    unsigned long w[64], v[8], t1, t2;
    int i;
    for (i = 0; i < 16; i++) w[i] = (unsigned long)p[4 * i] << 24 | (unsigned long)p[4 * i + 1] << 16 | (unsigned long)p[4 * i + 2] << 8 | p[4 * i + 3];
    for (i = 16; i < 64; i++)
        w[i] = (w[i - 16] + (rotr(w[i - 15], 7) ^ rotr(w[i - 15], 18) ^ (w[i - 15] >> 3)) + w[i - 7] +
                (rotr(w[i - 2], 17) ^ rotr(w[i - 2], 19) ^ (w[i - 2] >> 10))) & 0xFFFFFFFFUL;
    memcpy(v, h, sizeof v);
    for (i = 0; i < 64; i++) {
        t1 = (v[7] + (rotr(v[4], 6) ^ rotr(v[4], 11) ^ rotr(v[4], 25)) + ((v[4] & v[5]) ^ (~v[4] & v[6])) + K[i] + w[i]) & 0xFFFFFFFFUL;
        t2 = ((rotr(v[0], 2) ^ rotr(v[0], 13) ^ rotr(v[0], 22)) + ((v[0] & v[1]) ^ (v[0] & v[2]) ^ (v[1] & v[2]))) & 0xFFFFFFFFUL;
        memmove(v + 1, v, 7 * sizeof *v);
        v[4] = (v[4] + t1) & 0xFFFFFFFFUL;
        v[0] = (t1 + t2) & 0xFFFFFFFFUL;
    }
    for (i = 0; i < 8; i++) h[i] = (h[i] + v[i]) & 0xFFFFFFFFUL;
}

static void sha256_hex(const unsigned char *p, size_t n, char out[65])
{
    unsigned long h[8] = { 0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19 };
    unsigned char last[128] = { 0 };
    size_t i, rest = n % 64, tail = rest < 56 ? 64 : 128;
    for (i = 0; i + 64 <= n; i += 64) sha256_block(h, p + i);
    memcpy(last, p + i, rest);
    last[rest] = 0x80;
    for (i = 0; i < 8; i++) last[tail - 1 - i] = (unsigned char)((unsigned long long)n * 8 >> (8 * i));
    sha256_block(h, last);
    if (tail == 128) sha256_block(h, last + 64);
    for (i = 0; i < 8; i++) sprintf(out + 8 * i, "%08lx", h[i]);
}

/* ---------------------------------------------------------------- tests/archive3ds/cases.json */

/* Next JSON string after key from *p ("path": "..."), in out (no escapes in this file). 0 if found before stop. */
static int next_string(const char **p, const char *key, const char *stop, char *out, size_t n)
{
    char k[32];
    const char *at, *end, *s;
    snprintf(k, sizeof k, "\"%s\"", key);
    at = strstr(*p, k);
    s = stop ? strstr(*p, stop) : NULL;
    if (!at || (s && s < at) || !(at = strchr(at + strlen(k), '"')) || !(end = strchr(at + 1, '"')) || (size_t)(end - at - 1) >= n) return -1;
    memcpy(out, at + 1, end - at - 1);
    out[end - at - 1] = '\0';
    *p = end + 1;
    return 0;
}

static size_t from_hex(const char *h, unsigned char *out)
{
    size_t i, n = strlen(h) / 2;
    for (i = 0; i < n; i++) {
        unsigned v;
        sscanf(h + 2 * i, "%2x", &v);
        out[i] = (unsigned char)v;
    }
    return n;
}

/* One case: { "name", "files": [ {"path", "hex"}, ... ], "sha256", "error" }. 0 if read. */
static int run_case(const char **p)
{
    static archive_file_t files[8];
    static unsigned char data[8][2048];
    static char hex[8192];
    char name[128], sha[65], got[65], *next_case;
    const char *end;
    unsigned char *a = NULL;
    archive_file_t *back = NULL;
    size_t size = 0;
    int n = 0, nb = 0, i, r, error;

    if (next_string(p, "name", NULL, name, sizeof name)) return -1;
    end = strstr(*p, "\"sha256\"");
    while (n < 8 && !next_string(p, "path", "\"sha256\"", files[n].path, sizeof files[n].path)) {
        next_string(p, "hex", NULL, hex, sizeof hex);
        files[n].size = from_hex(hex, data[n]);
        files[n].data = data[n];
        n++;
    }
    if (!end || next_string(p, "sha256", NULL, sha, sizeof sha)) return -1;
    next_case = strstr(*p, "\"name\"");
    end = strstr(*p, "\"error\": true");
    error = end && (!next_case || end < next_case);

    r = pack_archive(files, n, &a, &size, NULL);
    if (error) {
        check(r == -1, name);
        return 0;
    }
    check(r == 0, name);
    if (r) return 0;
    sha256_hex(a, size, got);
    if (strcmp(got, sha)) printf("       %s : SHA-256 %s, attendu %s\n", name, got, sha);
    check(!strcmp(got, sha), name);
    check(unpack_archive(a, size, &back, &nb) == 0 && nb == n, "relecture");
    for (i = 0; i < nb && i < n; i++)
        check(!strcmp(back[i].path, files[i].path) && back[i].size == files[i].size && !memcmp(back[i].data, files[i].data, files[i].size), "fichier relu");
    free(back);
    free(a);
    return 0;
}

/* Archive of one file path holding text, without its two end blocks (entries to put together by hand). */
static size_t entry(const char *path, const char *text, unsigned char *out)
{
    archive_file_t f = { "", (unsigned char *)text, strlen(text), 0 };
    unsigned char *a;
    size_t size;
    snprintf(f.path, sizeof f.path, "%s", path);
    if (pack_archive(&f, 1, &a, &size, NULL)) return 0;
    memcpy(out, a, size - 1024);
    free(a);
    return size - 1024;
}

static int refused(unsigned char *a, size_t size)
{
    archive_file_t *f = NULL;
    int n;
    if (unpack_archive(a, size, &f, &n) == 0) {
        free(f);
        return 0;
    }
    return 1;
}

/* Same damages as TestArchiveRefusesDamage in client/archive3ds_test.go, and the hand-made orders. */
static void test_refusals(void)
{
    static unsigned char d[4096], c[4096];
    archive_file_t files[2] = { { "sub/x", NULL, 600, 0 }, { "main", (unsigned char *)"abc", 3, 0 } };
    unsigned char *a, *b;
    size_t size, bsize, n;
    int bad = -1;

    check(pack_archive(files, 2, &a, &size, NULL) == 0 && size == 512 + 512 + 512 + 1024 + 1024, "archive de deux fichiers");
    check(!strcmp(files[0].path, "main") && files[0].offset == 512 && files[1].offset == 1536, "triés, offsets des données");
    check(!refused(a, size), "archive intacte lue");
    check(refused(a, size - 1), "coupée");
    memcpy(d, a, size);
    d[size] = 0;
    check(refused(d, size + 1), "un octet de trop");
    d[0] = 'M';
    check(refused(d, size), "somme de contrôle fausse");
    check(refused(a, size - 1024), "sans les blocs de fin");
    check(refused(NULL, 0), "vide");
    free(a);

    check(pack_archive(NULL, 0, &b, &bsize, NULL) == 0 && bsize == 1024 && !refused(b, bsize), "archive sans fichier lue");
    free(b);

    n = entry("d", "", d);
    memcpy(c, d, n + 1024);
    c[156] = '5'; /* a directory entry, checksum fixed: still refused */
    {
        unsigned long sum = 0;
        size_t i;
        for (i = 0; i < 512; i++) sum += i >= 148 && i < 156 ? ' ' : c[i];
        sprintf((char *)c + 148, "%06lo", sum);
        c[155] = ' ';
    }
    memset(c + n, 0, 1024);
    check(refused(c, n + 1024), "entrée qui n'est pas un fichier");

    n = entry("b", "2", d);
    n += entry("a", "1", d + n);
    memset(d + n, 0, 1024);
    check(refused(d, n + 1024), "chemins dans le désordre");
    n = entry("a", "1", d);
    n += entry("a", "2", d + n);
    memset(d + n, 0, 1024);
    check(refused(d, n + 1024), "chemin en double");

    {
        archive_file_t twice[2] = { { "x", NULL, 0, 0 }, { "x", NULL, 0, 0 } };
        check(pack_archive(twice, 2, &a, &size, &bad) == -1 && bad == 1, "pack : chemin en double refusé");
    }
    check(archive_path_ok("a/b.bin") && !archive_path_ok("a//b") && !archive_path_ok("./a") && !archive_path_ok("a/..") &&
              !archive_path_ok("a\\b") && !archive_path_ok("") && !archive_path_ok("a/"),
          "chemins acceptés et refusés");
}

int main(int argc, char **argv)
{
    unsigned char *json;
    size_t size;
    const char *p;
    int cases = 0;
    if (argc < 2 || read_file(argv[1], &json, &size)) {
        printf("usage : test_archive ../tests/archive3ds/cases.json\n");
        return 1;
    }
    json = realloc(json, size + 1);
    json[size] = '\0';
    for (p = (const char *)json; !run_case(&p);) cases++;
    check(cases == 8, "8 cas lus");
    free(json);
    test_refusals();
    printf("%s\n", failures ? "ECHECS" : "archive : OK");
    return failures != 0;
}
