/* Clé d'un Jeu (ADR 0007) et SRAM GB/GBA (ADR 0008). Aucune dépendance libctru : testé sur PC. */
#ifndef CLE_H
#define CLE_H

#include <stddef.h>

typedef enum { SYS_DS, SYS_GB, SYS_GBA, SYS_3DS } systeme_t;

#define CLE_MAX 32

/* Clé lue dans les 0x200 premiers octets d'une ROM DS, GB/GBC ou GBA. 0 si valide, -1 sinon. */
int cle_rom(systeme_t s, const unsigned char *entete, char cle[CLE_MAX]);

/* "3DS-00040000001B5000" */
void cle_3ds(unsigned long long title_id, char cle[CLE_MAX]);

/* Taille de la SRAM : GB/GBA sans le Trailer d'horloge (moins de 1 Kio) ; autres Systèmes : taille entière. */
size_t sram_taille(systeme_t s, size_t taille_fichier);

/* 1 si la SRAM est vierge (que des 0x00 ou que des 0xFF) : comptée comme absente. Jamais pour la 3DS. */
int sauvegarde_vide(systeme_t s, const unsigned char *sram, size_t taille);

#endif
