/* Fichiers de la carte SD. Aucune dépendance libctru : testé sur PC (test/test_fichiers.c). */
#ifndef FICHIERS_H
#define FICHIERS_H

#include <stddef.h>
#include "cle.h"

/* Fichier entier dans *octets (à libérer). 0 si lu, -1 sinon. */
int lire_fichier(const char *chemin, unsigned char **octets, size_t *taille);

/* 0 si écrit en entier, -1 sinon. */
int ecrire_fichier(const char *chemin, const void *octets, size_t taille);

/* 0 si écrit en entier et relu identique, -1 sinon. */
int ecrire_relu(const char *chemin, const void *octets, size_t taille);

/* Remplace chemin sans perte : chemin.tmp écrit et relu, puis l'ancien retiré et chemin.tmp renommé.
 * 0 si fait ; -1 si chemin.tmp n'a pas pu être écrit (ancien intact) ; -2 si l'ancien est retiré mais le renommage ou
 * la relecture a échoué (chemin.tmp, vérifié, reste si le renommage a échoué). */
int remplacer_fichier(const char *chemin, const void *octets, size_t taille);

/* Met le contenu de src dans dst sans perte : copie de dst dans copie s'il existe, dst.tmp relu
 * et vérifié, puis remplacement. supprimer_src : retire src ensuite (dst a le même contenu).
 * 0 si fait ; -1 sinon, sans rien perdre (au pire, dst.tmp reste). */
int deplacer_sauvegarde(const char *src, const char *dst, const char *copie, int supprimer_src);

/* Sauvegarde de la ROM nom (sans extension) rangée dans dossier (racine roms ou un de ses sous-dossiers, ADR 0015).
 * saves vide ou égal à roms : à côté de la ROM. saves = roms + "/saves" (TWiLight Menu++) : dans le dossier saves du
 * dossier de la ROM. Autre dossier (open_agb_firm, _nds/TWiLightMenu/saves) : toutes les ROM dans ce dossier.
 * 0 si le chemin tient dans out, -1 sinon. */
int chemin_sauvegarde(const char *roms, const char *saves, const char *dossier, const char *nom, char *out, size_t n);

/* 1 si deux ROM partagent un fichier de Sauvegarde : même .sav, ou deux ROM GB/GBA de même nom (casse ignorée), quels
 * que soient leur dossier et leur Système : le forwarder mGBA range sa Sauvegarde sous sdmc:/mGBA/forwarders/<nom>.sav. */
int meme_sauvegarde(systeme_t a, const char *nom_a, const char *sav_a, systeme_t b, const char *nom_b, const char *sav_b);

#endif
