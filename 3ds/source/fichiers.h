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

/* Remplace le programme chemin (.3dsx) : chemin.tmp écrit et relu, chemin renommé chemin.bak, chemin.tmp renommé
 * chemin et relu, puis chemin.bak retiré. Une coupure laisse toujours un programme entier (chemin ou chemin.bak).
 * 0 si fait ; -1 si l'ancien est en place (chemin absent : rien n'est touché) ; -2 si l'ancien n'a pas pu revenir :
 * il reste dans chemin.bak. */
int remplacer_programme(const char *chemin, const void *octets, size_t taille);

/* 1 si argv[0] est un .3dsx de la carte SD (Homebrew Launcher) ; 0 pour 3dslink (3dslink:/…), un .cia ou rien. */
int lance_depuis_sd(const char *argv0);

/* Met le contenu de src dans dst sans perte : copie de dst dans copie s'il existe, dst.tmp relu
 * et vérifié, puis remplacement. supprimer_src : retire src ensuite (dst a le même contenu).
 * 0 si fait ; -1 sinon, sans rien perdre (au pire, dst.tmp reste). */
int deplacer_sauvegarde(const char *src, const char *dst, const char *copie, int supprimer_src);

/* Nom libre pour une copie de sécurité : dossier/nom-date.ext, sinon dossier/nom-date-1.ext, -2… (deux copies la même
 * seconde, ou deux ROM de même nom). Une copie existante n'est jamais écrasée. 0 si trouvé et tient dans out, -1 sinon. */
int chemin_copie(const char *dossier, const char *nom, const char *date, const char *ext, char *out, size_t n);

/* Sauvegarde de la ROM nom (sans extension) rangée dans dossier (racine roms ou un de ses sous-dossiers, ADR 0015).
 * saves vide ou égal à roms : à côté de la ROM. saves = roms + "/saves" (TWiLight Menu++) : dans le dossier saves du
 * dossier de la ROM. Autre dossier (open_agb_firm, _nds/TWiLightMenu/saves) : toutes les ROM dans ce dossier.
 * 0 si le chemin tient dans out, -1 sinon. */
int chemin_sauvegarde(const char *roms, const char *saves, const char *dossier, const char *nom, char *out, size_t n);

/* 1 si deux ROM partagent un fichier de Sauvegarde : même .sav, ou deux ROM GB/GBA de même nom (casse ignorée), quels
 * que soient leur dossier et leur Système : le forwarder mGBA range sa Sauvegarde sous sdmc:/mGBA/forwarders/<nom>.sav. */
int meme_sauvegarde(systeme_t a, const char *nom_a, const char *sav_a, systeme_t b, const char *nom_b, const char *sav_b);

#endif
