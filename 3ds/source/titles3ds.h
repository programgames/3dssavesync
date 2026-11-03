/* Native 3DS Save archives (ADR 0009): the /main file of ARCHIVE_USER_SAVEDATA. */
#ifndef TITLES3DS_H
#define TITLES3DS_H

#include <3ds.h>
#include <stddef.h>

/* Reads /main (installed on the SD card, else game card). 0 if OK: *bytes allocated (free), *media = medium found.
 * -1 if the game has no archive (never started, missing): nothing to sync. */
int read_title_save(u64 id, FS_MediaType *media, unsigned char **bytes, size_t *size);

/* Rewrites /main (same size required), commit, then removal of the secure value. 0 if OK.
 * Without the commit, the archive keeps its old content: a failure never leaves a half-written save. */
int write_title_save(u64 id, FS_MediaType media, const void *bytes, size_t size);

/* 1 if the archive holds only the /main file (the only case the sync can copy without loss),
 * 0 if it holds something else, -1 if the game has no archive (never started). */
int title_main_only(u64 id);

/* Short name of the game (SMDH), in the given language, in UTF-8. 0 if read. */
int title_name(u64 id, FS_MediaType media, int language, char *name, size_t n);

/* Large icon of the game (SMDH): 48×48 RGB565 pixels, row by row. 0 if read. */
int title_icon(u64 id, FS_MediaType media, u16 *pixels);

/* Installed applications (SD card) and inserted game card. Returns the number of titles (max at most). */
int installed_titles(u64 *ids, FS_MediaType *medias, int max);

#endif
