/* Archives de sauvegarde 3DS natives (ADR 0009) : le fichier /main de ARCHIVE_USER_SAVEDATA. */
#ifndef TITRES3DS_H
#define TITRES3DS_H

#include <3ds.h>
#include <stddef.h>

typedef struct {
    u64 id;
    const char *nom;
} titre3ds_t;

#define NB_TITRES_3DS 8
extern const titre3ds_t TITRES_3DS[NB_TITRES_3DS];

/* Lit /main (installé sur la SD, sinon cartouche). 0 si OK : *octets alloué (free), *media = support trouvé.
 * -1 si le jeu n'a pas d'archive (jamais lancé, absent) : rien à synchroniser. */
int titre_lire(u64 id, FS_MediaType *media, unsigned char **octets, size_t *taille);

/* Réécrit /main (taille identique obligatoire), commit, puis suppression de la secure value. 0 si OK.
 * Sans commit, l'archive garde son ancien contenu : un échec ne laisse pas de sauvegarde à moitié écrite. */
int titre_ecrire(u64 id, FS_MediaType media, const void *octets, size_t taille);

#endif
