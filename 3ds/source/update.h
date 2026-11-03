/* App update from the GitHub releases (ADR 0021). No libctru dependency: tested on the PC
 * (test/test_update.c). */
#ifndef UPDATE_H
#define UPDATE_H

typedef struct {
    char version[32];                     /* tag_name without the "v": "3.7.0" */
    char cia[512], dsx[512], checksums[512]; /* addresses of 3dssavesync.cia, 3dssavesync.3dsx, SHA256SUMS; "" if missing */
} release_t;

/* Response of GET /repos/<repo>/releases/latest. 0 if tag_name is read (an address may be missing), -1 otherwise. */
int parse_release(const char *json, release_t *r);

/* 1 if remote is newer than current: "3.10.0" > "3.9.1", "3.7.0" > "3.7.0-rc.1", leading "v" allowed.
 * 0 otherwise, and if either is not an X.Y.Z version ("dev" build: never an update). */
int update_is_newer(const char *current, const char *remote);

/* Hash of name in a SHA256SUMS file (sha256sum format: "<hash>  <name>" or "<hash> *<name>").
 * 0 if found (hash in lowercase), -1 otherwise. */
int find_checksum(const char *checksums, const char *name, char hash[65]);

/* 1 if the automatic check is due: none today (day = now / 86400, in seconds; a clock set back
 * counts as another day). */
int update_check_due(long last_day, long long now);

#endif
