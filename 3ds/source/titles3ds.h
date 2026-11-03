/* Archives de sauvegarde 3DS natives (ADR 0009) : le fichier /main de ARCHIVE_USER_SAVEDATA. */
#ifndef TITLES3DS_H
#define TITLES3DS_H

#include <3ds.h>
#include <stddef.h>

/* Lit /main (installé sur la SD, sinon cartouche). 0 si OK : *octets alloué (free), *media = support trouvé.
 * -1 si le jeu n'a pas d'archive (jamais lancé, absent) : rien à synchroniser. */
int read_title_save(u64 id, FS_MediaType *media, unsigned char **bytes, size_t *size);

/* Réécrit /main (taille identique obligatoire), commit, puis suppression de la secure value. 0 si OK.
 * Sans commit, l'archive garde son ancien contenu : un échec ne laisse pas de sauvegarde à moitié écrite. */
int write_title_save(u64 id, FS_MediaType media, const void *bytes, size_t size);

/* 1 si l'archive ne contient que le fichier /main (seul cas que la synchronisation sait copier sans perte),
 * 0 si elle contient autre chose, -1 si le jeu n'a pas d'archive (jamais lancé). */
int title_main_only(u64 id);

/* Nom court du jeu (SMDH), dans la langue donnée, en UTF-8. 0 si lu. */
int title_name(u64 id, FS_MediaType media, int language, char *name, size_t n);

/* Grande icône du jeu (SMDH) : 48×48 pixels RGB565, ligne par ligne. 0 si lue. */
int title_icon(u64 id, FS_MediaType media, u16 *pixels);

/* Applications installées (SD) et cartouche insérée. Nombre de titres (au plus max). */
int installed_titles(u64 *ids, FS_MediaType *medias, int max);

#endif
