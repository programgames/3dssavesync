/* Native 3DS Save archives: the /main file of ARCHIVE_USER_SAVEDATA (ADR 0009), or all of its files (ADR 0028). */
#ifndef TITLES3DS_H
#define TITLES3DS_H

#include <3ds.h>
#include <stddef.h>

/* Reads file ("/main", "/sav.dat" of a Virtual Console title) of the archive (installed on the SD card, else game
 * card). 0 if OK: *bytes allocated (free), *media = medium found.
 * -1 if the game has no archive or no such file (never started, missing): nothing to sync. */
int read_title_save(u64 id, const char *file, FS_MediaType *media, unsigned char **bytes, size_t *size);

/* Rewrites file ("/main", or "/sav.dat" of a Virtual Console game; same size required), commit, then removal of the
 * secure value. 0 if OK. Without the commit, the archive keeps its old content: a failure never leaves a half-written
 * save. */
int write_title_save(u64 id, FS_MediaType media, const char *file, const void *bytes, size_t size);

/* Whole save archive of the other 3DS games (ADR 0028): every file, as one 3dssavesync archive (archive.h). */
#define ARCHIVE_MAX (16 * 1024 * 1024) /* bytes of files at most: beyond, the game is Not synced (memory of an Old 3DS) */

/* Every file of the archive (SD card, else game card), packed in *bytes (to free), *media = medium found.
 * 0 if read; -1 if the game has no archive (never started); -2 if it can't be synced or read: why says why (English,
 * the technical line: a path over 100 bytes, a name that is not ASCII, over ARCHIVE_MAX, a read error). */
int read_title_archive(u64 id, FS_MediaType *media, unsigned char **bytes, size_t *size, char *why, size_t wn);

/* Writes the version tar (unpack_archive: a damaged one is refused) into the existing archive: files absent from it
 * deleted, the others created or resized and written, commit, secure value removed, then read back and compared. An
 * archive is never created or formatted. tar is freed (always): the read back takes its memory and is compared by
 * SHA-256. 0 if done; -1 if nothing was committed (old save intact); -2 if committed but the read back differs. why: the
 * technical line. */
int write_title_archive(u64 id, FS_MediaType media, unsigned char *tar, size_t size, char *why, size_t wn);

/* Extra data (extdata) of 3DS games (ADR 0031): its user/ files, as one 3dssavesync archive, same rules and limit. */

/* Extdata ID of the application id: AM (its exheader), else title ID low >> 8. */
u32 title_extdata_id(u64 id);

/* Every file of the extdata, packed in *bytes (to free). A sparse file is read at its full size, with zeros where the
 * system has no data. 0 if read; -1 if it does not exist (never created by the game: not synced, never created here);
 * -2 if it can't be synced or read (why, as read_title_archive). */
int read_extdata(u32 extdata, unsigned char **bytes, size_t *size, char *why, size_t wn);

/* As write_title_archive, in the existing extdata, without commit or secure value. 0 if done; -1 if nothing was
 * changed (refused: a damaged version, an empty file the extdata does not have empty, a listing error); -2 if files
 * were changed and a write failed or the read back differs: old and new files may be mixed, the backup copy is needed. */
int write_extdata(u32 extdata, unsigned char *tar, size_t size, char *why, size_t wn);

/* Names at the root of the archive in names, joined by ", ", a folder ending with '/' ("sav.dat, SecureValue"), cut if
 * longer than n. Number of entries, -1 if the game has no archive. */
int title_root(u64 id, char *names, size_t n);

/* First 0x200 bytes (n) of the GB ROM of a Virtual Console title installed on the SD card: the file of rom/ in its
 * RomFS, read through FSPXI (Luma3DS only). On failure, why says which step failed ("OpenFile:D9004676").
 * 0 if read, -1 otherwise. */
int title_rom_header(u64 id, unsigned char *header, size_t n, char *why, size_t wn);

/* 1 if title id (SD) is a GB or GBC Virtual Console game, by its product code (vc_product). */
int title_vc(u64 id);

/* Creates the Save archive of a Virtual Console title (SD) that has none (never started) and writes
 * /sav.dat (size bytes), commit, secure value removed. Refused (0xFFFFFFFE) if the archive exists: never formatted
 * over a save. 0 if done, otherwise the failed result. */
Result create_vc_save(u64 id, const void *bytes, size_t size);

/* Short name of the game (SMDH), in the given language, in UTF-8. 0 if read. */
int title_name(u64 id, FS_MediaType media, int language, char *name, size_t n);

/* Large icon of the game (SMDH): 48×48 RGB565 pixels, row by row. 0 if read. */
int title_icon(u64 id, FS_MediaType media, u16 *pixels);

/* Installed applications (SD card) and inserted game card, all of them, in *ids and *medias (allocated with room for
 * extra more: free them). Returns the number of titles, -1 if out of memory (nothing allocated). */
int installed_titles(u64 **ids, FS_MediaType **medias, int extra);

/* Date of the newest .tmd of an SD title (sdmc:/Nintendo 3DS/<id0>/<id1>/title/<high>/<low>/content), as the SD card
 * stores it: it changes when the game is installed again (ADR 0027). 0 if unknown (game card, no access). */
unsigned long long title_install_time(u64 id);

#endif
