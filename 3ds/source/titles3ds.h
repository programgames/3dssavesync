/* Native 3DS Save archives (ADR 0009): the /main file of ARCHIVE_USER_SAVEDATA. */
#ifndef TITLES3DS_H
#define TITLES3DS_H

#include <3ds.h>
#include <stddef.h>

/* Reads file ("/main", "/sav.dat" of a Virtual Console title) of the archive (installed on the SD card, else game
 * card). 0 if OK: *bytes allocated (free), *media = medium found.
 * -1 if the game has no archive or no such file (never started, missing): nothing to sync. */
int read_title_save(u64 id, const char *file, FS_MediaType *media, unsigned char **bytes, size_t *size);

/* Rewrites /main (same size required), commit, then removal of the secure value. 0 if OK.
 * Without the commit, the archive keeps its old content: a failure never leaves a half-written save. */
int write_title_save(u64 id, FS_MediaType media, const void *bytes, size_t size);

/* 1 if the archive holds only the /main file (the only case the sync can copy without loss),
 * 0 if it holds something else, -1 if the game has no archive (never started). */
int title_main_only(u64 id);

/* Names at the root of the archive in names, joined by ", ", a folder ending with '/' ("sav.dat, SecureValue"), cut if
 * longer than n. Number of entries, -1 if the game has no archive. */
int title_root(u64 id, char *names, size_t n);

/* First 0x200 bytes (n) of the GB ROM of a Virtual Console title installed on the SD card: the file of rom/ in its
 * RomFS, read through FSPXI (Luma3DS only). On failure, why says which step failed ("OpenFile:D9004676").
 * 0 if read, -1 otherwise. */
int title_rom_header(u64 id, unsigned char *header, size_t n, char *why, size_t wn);

/* 1 if title id (SD) is a GB or GBC Virtual Console game, by its product code (vc_product). */
int title_vc(u64 id);

/* Product code of title id (SD) in code (16 bytes, "" if not read). */
Result title_product_code(u64 id, char *code);

/* Short name of the game (SMDH), in the given language, in UTF-8. 0 if read. */
int title_name(u64 id, FS_MediaType media, int language, char *name, size_t n);

/* Large icon of the game (SMDH): 48×48 RGB565 pixels, row by row. 0 if read. */
int title_icon(u64 id, FS_MediaType media, u16 *pixels);

/* Installed applications (SD card) and inserted game card, all of them, in *ids and *medias (allocated with room for
 * extra more: free them). Returns the number of titles, -1 if out of memory (nothing allocated). */
int installed_titles(u64 **ids, FS_MediaType **medias, int extra);

#endif
