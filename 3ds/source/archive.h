/* 3dssavesync archive (ADR 0028): the files of a 3DS save archive as one deterministic ustar, byte for byte the same as
 * client/archive3ds.go and Android, so the same files give the same Hash everywhere. Shared cases: tests/archive3ds/.
 * No libctru dependency: tested on the PC (test/test_archive.c). */
#ifndef ARCHIVE_H
#define ARCHIVE_H

#include <stddef.h>

#define ARCHIVE_PATH_MAX 256 /* room for a path read from the 3DS; the archive itself takes 100 bytes at most */

/* One file of a save archive: its path from the archive root ("main", "dir/file"), its bytes. offset: where its data
 * starts in the packed archive (pack_archive). */
typedef struct {
    char path[ARCHIVE_PATH_MAX];
    unsigned char *data;
    size_t size, offset;
} archive_file_t;

/* 1 if p is a path the archive accepts: relative, '/' separators, no empty, "." or ".." part, no '\', 100 bytes at most. */
int archive_path_ok(const char *p);

/* Sorts files by path (byte order) and packs them in *out (allocated, to free), *size bytes. A file whose data is NULL
 * keeps zeros in its place: the caller fills it at out + offset (the 3DS reads each file straight into the archive).
 * 0 if packed; -1 if a path is refused or twice (*bad: its index after the sort), -2 if out of memory. */
int pack_archive(archive_file_t *files, int n, unsigned char **out, size_t *size, int *bad);

/* Reads an archive written by pack_archive: *files allocated (free it; *n entries, possibly 0), their data point into
 * data. Anything else (other entry types, unsorted or duplicate paths, a bad checksum, data past the end, missing end
 * blocks) is refused: 0 if read, -1 if damaged, -2 if out of memory. */
int unpack_archive(unsigned char *data, size_t size, archive_file_t **files, int *n);

#endif
