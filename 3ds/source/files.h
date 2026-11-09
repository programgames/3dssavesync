/* Files on the SD card. No libctru dependency: tested on the PC (test/test_files.c). */
#ifndef FILES_H
#define FILES_H

#include <stddef.h>
#include "key.h"

/* Whole file in *bytes (to free). 0 if read, -1 otherwise. */
int read_file(const char *path, unsigned char **bytes, size_t *size);

/* 0 if written in full, -1 otherwise. */
int write_file(const char *path, const void *bytes, size_t size);

/* Adds line and a newline at the end of path. When path is already larger than max bytes, it becomes path.1 first
 * (the previous path.1 goes): the two files hold the last events, at most about 2 × max. 0 if written, -1 otherwise. */
int append_log(const char *path, const char *line, long max);

/* 0 if written in full and read back identical, -1 otherwise. */
int write_verified(const char *path, const void *bytes, size_t size);

/* Replaces path without loss: path.tmp written and read back, then the old file removed and path.tmp renamed.
 * 0 if done; -1 if path.tmp could not be written (old file intact); -2 if the old file is removed but the rename or
 * the read-back failed (path.tmp, checked, stays if the rename failed). */
int replace_file(const char *path, const void *bytes, size_t size);

/* Replaces the program at path (.3dsx): path.tmp written and read back, path renamed path.bak, path.tmp renamed
 * path and read back, then path.bak removed. A power cut always leaves a whole program (path or path.bak).
 * 0 if done; -1 if the old program is in place (path missing: nothing is touched); -2 if the old program could not
 * come back: it stays in path.bak. */
int replace_program(const char *path, const void *bytes, size_t size);

/* 1 if argv[0] is a .3dsx on the SD card (Homebrew Launcher); 0 for 3dslink (3dslink:/…), a .cia or nothing. */
int launched_from_sd(const char *argv0);

/* Puts the content of src in dst without loss: copy of dst in backup if it exists, dst.tmp read back
 * and checked, then replace. remove_src: removes src afterwards (dst has the same content).
 * 0 if done; -1 otherwise, with nothing lost (at worst, dst.tmp stays). */
int move_save(const char *src, const char *dst, const char *backup, int remove_src);

/* Free name for a Backup copy: dir/name-date.ext, else dir/name-date-1.ext, -2… (two copies in the same
 * second, or two ROMs with the same name). An existing copy is never overwritten. 0 if found and fits in out, -1 otherwise. */
int backup_path(const char *dir, const char *name, const char *date, const char *ext, char *out, size_t n);

/* Save of the ROM name (no extension) stored in dir (the roms root or one of its subfolders, ADR 0015).
 * saves empty or equal to roms: next to the ROM. saves = roms + "/saves" (TWiLight Menu++): in the saves folder of
 * the ROM's folder. Other folder (open_agb_firm, _nds/TWiLightMenu/saves): all ROMs in that folder.
 * 0 if the path fits in out, -1 otherwise. */
int save_path(const char *roms, const char *saves, const char *dir, const char *name, char *out, size_t n);

/* 1 if two ROMs share a Save file: same .sav, or two GB/GBA ROMs with the same name (case ignored), whatever
 * their folder and System: the mGBA forwarder stores its Save as sdmc:/mGBA/forwarders/<name>.sav. */
int same_save(system_t a, const char *name_a, const char *sav_a, system_t b, const char *name_b, const char *sav_b);

#endif
