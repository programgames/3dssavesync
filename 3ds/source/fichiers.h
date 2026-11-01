/* Fichiers de la carte SD. Aucune dépendance libctru : testé sur PC (test/test_fichiers.c). */
#ifndef FICHIERS_H
#define FICHIERS_H

#include <stddef.h>

/* Fichier entier dans *octets (à libérer). 0 si lu, -1 sinon. */
int lire_fichier(const char *chemin, unsigned char **octets, size_t *taille);

/* 0 si écrit en entier, -1 sinon. */
int ecrire_fichier(const char *chemin, const void *octets, size_t taille);

/* Met le contenu de src dans dst sans perte : copie de dst dans copie s'il existe, dst.tmp relu
 * et vérifié, puis remplacement. supprimer_src : retire src ensuite (dst a le même contenu).
 * 0 si fait ; -1 sinon, sans rien perdre (au pire, dst.tmp reste). */
int deplacer_sauvegarde(const char *src, const char *dst, const char *copie, int supprimer_src);

#endif
