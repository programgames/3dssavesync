/* Clé d'un Jeu (ADR 0007) et SRAM GB/GBA (ADR 0008). Aucune dépendance libctru : testé sur PC. */
#ifndef CLE_H
#define CLE_H

#include <stddef.h>

typedef enum { SYS_DS, SYS_GB, SYS_GBA, SYS_3DS } systeme_t;

#define CLE_MAX 32

/* Clé lue dans les 0x200 premiers octets d'une ROM DS, GB/GBC ou GBA. 0 si valide, -1 sinon. */
int cle_rom(systeme_t s, const unsigned char *entete, char cle[CLE_MAX]);

/* Alias d'une ROM (alias.ini, ADR 0014) : valeur sans espaces autour, en majuscules, de la forme d'une Clé du Système
 * (DS "0RAD", GBA "GBA-0RAD", GB "GB-MONHACK-0001"). 0 si valide (Clé dans cle), -1 sinon. Même règle que client/cas.go. */
int cle_alias(systeme_t s, const char *valeur, char cle[CLE_MAX]);

/* "3DS-00040000001B5000" */
void cle_3ds(unsigned long long title_id, char cle[CLE_MAX]);

/* Taille de la SRAM : GB/GBA sans le Trailer d'horloge (moins de 1 Kio) ; autres Systèmes : taille entière. */
size_t sram_taille(systeme_t s, size_t taille_fichier);

/* 1 si la SRAM est vierge (que des 0x00 ou que des 0xFF) : comptée comme absente. Jamais pour la 3DS. */
int sauvegarde_vide(systeme_t s, const unsigned char *sram, size_t taille);

/* GBA à EEPROM (512 o ou 8 Kio) écrite par open_agb_firm : chaque bloc de 8 octets est à l'envers de l'ordre de mGBA,
 * celui du serveur (ADR 0012). L'opération est son propre inverse. 1 si la taille est celle d'une EEPROM (et inversée). */
int eeprom_inverser(unsigned char *sram, size_t taille);

/* GB à MBC2 (octet 0x147 de la ROM : 0x05 ou 0x06) : 512 valeurs de 4 bits. Forme du serveur : 512 octets, une valeur
 * par octet, quartet haut à 0 (ADR 0012). */
#define MBC2_SERVEUR 512
int rom_mbc2(const unsigned char *entete);
/* Fichier de 256 o (mGBA), 512 o (SameBoy, VBA-M, BGB) ou 8 Kio (GameYob, TWiLight Menu++) -> forme du serveur. 0 si fait. */
int mbc2_normaliser(const unsigned char *fichier, size_t taille, unsigned char serveur[MBC2_SERVEUR]);
/* Forme du serveur -> fichier de taille octets (256, 512 ou 8 Kio). 0 si fait. */
int mbc2_fichier(const unsigned char serveur[MBC2_SERVEUR], unsigned char *fichier, size_t taille);

#endif
