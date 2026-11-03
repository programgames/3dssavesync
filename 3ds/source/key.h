/* Key of a Game (ADR 0007) and GB/GBA SRAM (ADR 0008). No libctru dependency: tested on the PC. */
#ifndef KEY_H
#define KEY_H

#include <stddef.h>

typedef enum { SYS_DS, SYS_GB, SYS_GBA, SYS_3DS } system_t;

#define KEY_MAX 32

/* Key read from the first 0x200 bytes of a DS, GB/GBC or GBA ROM. 0 if valid, -1 otherwise. */
int rom_key(system_t s, const unsigned char *header, char key[KEY_MAX]);

/* Alias of a ROM (alias.ini, ADR 0014): value trimmed, uppercased, in the form of a Key of the System
 * (DS "0RAD", GBA "GBA-0RAD", GB "GB-MONHACK-0001"). 0 if valid (Key in key), -1 otherwise. Same rule as client/cases.go. */
int alias_key(system_t s, const char *value, char key[KEY_MAX]);

/* "3DS-00040000001B5000" */
void key_3ds(unsigned long long title_id, char key[KEY_MAX]);

/* SRAM size: GB/GBA without the Clock trailer (under 1 KiB); other Systems: whole size. */
size_t sram_size(system_t s, size_t file_size);

/* 1 if the SRAM is blank (only 0x00 or only 0xFF): counted as absent. Never for the 3DS. */
int is_blank_save(system_t s, const unsigned char *sram, size_t size);

/* GBA with EEPROM (512 B or 8 KiB) written by open_agb_firm: each 8-byte block is in reverse order from mGBA,
 * the server's order (ADR 0012). The operation is its own inverse. 1 if the size is an EEPROM size (and swapped). */
int eeprom_swap(unsigned char *sram, size_t size);

/* GB with MBC2 (ROM byte 0x147: 0x05 or 0x06): 512 values of 4 bits. Server form: 512 bytes, one value
 * per byte, high nibble 0 (ADR 0012). */
#define MBC2_SERVER 512
int rom_mbc2(const unsigned char *header);
/* File of 256 B (mGBA), 512 B (SameBoy, VBA-M, BGB) or 8 KiB (GameYob, TWiLight Menu++) -> server form. 0 if done. */
int mbc2_normalize(const unsigned char *file, size_t size, unsigned char server[MBC2_SERVER]);
/* Server form -> file of size bytes (256, 512 or 8 KiB). 0 if done. */
int mbc2_to_file(const unsigned char server[MBC2_SERVER], unsigned char *file, size_t size);

#endif
