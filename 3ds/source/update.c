#include <ctype.h>
#include <stdlib.h>
#include <string.h>
#include "update.h"

/* JSON string after "key": (spaces allowed), no escapes. 0 if read in full into out.
 * ponytail: hand-written parse like /saves (list.c); tag_name and addresses never have escapes. */
static int json_value(const char *p, char *out, size_t n)
{
    size_t i = 0;
    while (*p == ' ') p++;
    if (*p++ != '"') return -1;
    while (*p && *p != '"' && *p != '\\' && i + 1 < n) out[i++] = *p++;
    out[i] = '\0';
    return *p == '"' ? 0 : -1;
}

int parse_release(const char *json, release_t *r)
{
    static const char TAG[] = "\"tag_name\":", URL[] = "\"browser_download_url\":";
    const char *p = strstr(json, TAG);

    memset(r, 0, sizeof *r);
    if (!p || json_value(p + sizeof TAG - 1, r->version, sizeof r->version) || !r->version[0]) return -1;
    if (r->version[0] == 'v') memmove(r->version, r->version + 1, strlen(r->version));
    for (p = strstr(json, URL); p; p = strstr(p + 1, URL)) {
        char url[512], *name;
        if (json_value(p + sizeof URL - 1, url, sizeof url)) continue;
        name = strrchr(url, '/') ? strrchr(url, '/') + 1 : url;
        if (!strcmp(name, "3dssavesync.cia")) strcpy(r->cia, url);
        else if (!strcmp(name, "3dssavesync.3dsx")) strcpy(r->dsx, url);
        else if (!strcmp(name, "SHA256SUMS")) strcpy(r->checksums, url);
    }
    return 0;
}

/* "v3.10.2-rc.1" -> { 3, 10, 2 }, *pre = 1. 0 if read. */
static int parse_version(const char *s, long n[3], int *pre)
{
    int i;
    char *end;
    if (*s == 'v') s++;
    for (i = 0; i < 3; i++) {
        if (!isdigit((unsigned char)*s)) return -1;
        n[i] = strtol(s, &end, 10);
        s = end;
        if (i < 2 && *s++ != '.') return -1;
    }
    *pre = *s == '-';
    return *s && !*pre ? -1 : 0;
}

int update_is_newer(const char *current, const char *remote)
{
    long a[3], b[3];
    int pa, pb, i;
    if (parse_version(current, a, &pa) || parse_version(remote, b, &pb)) return 0;
    for (i = 0; i < 3; i++)
        if (a[i] != b[i]) return b[i] > a[i];
    return pa && !pb;
}

int find_checksum(const char *checksums, const char *name, char hash[65])
{
    const char *l;
    size_t n = strlen(name);
    for (l = checksums; l && *l; l = strchr(l, '\n') ? strchr(l, '\n') + 1 : NULL) {
        const char *f = l + 64;
        int i;
        for (i = 0; i < 64 && isxdigit((unsigned char)l[i]); i++);
        if (i < 64 || f[0] != ' ' || (f[1] != ' ' && f[1] != '*')) continue;
        if (strncmp(f + 2, name, n) || (f[2 + n] && f[2 + n] != '\n' && f[2 + n] != '\r')) continue;
        for (i = 0; i < 64; i++) hash[i] = (char)tolower((unsigned char)l[i]);
        hash[64] = '\0';
        return 0;
    }
    return -1;
}

int update_check_due(long last_day, long long now)
{
    return last_day != (long)(now / 86400);
}
