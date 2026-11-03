/* Dessin des deux écrans avec citro2d (issue #21) : texte en police du système (accents, icônes des boutons),
 * rectangles, icône de jeu. */
#ifndef ECRAN_H
#define ECRAN_H

#include <3ds.h>

#define RGB(c) (0xFF000000u | ((c) & 0xFF) << 16 | ((c) & 0xFF00) | ((c) >> 16 & 0xFF)) /* 0xRRGGBB -> citro2d */

void ecran_init(void);
void ecran_fin(void);

/* Une image : ecran_debut, puis ecran_haut ou ecran_bas avant de dessiner sur cet écran, puis ecran_finir. */
void ecran_debut(void);
void ecran_haut(void);
void ecran_bas(void);
void ecran_finir(void);

void ecran_rect(float x, float y, float l, float h, u32 couleur);

/* Texte sur une ligne, coupé avec « … » au-delà de largeur (0 = sans limite). Largeur dessinée. */
float ecran_texte(float x, float y, float taille, u32 couleur, float largeur, const char *s);

/* Largeur du texte sur une ligne. */
float ecran_largeur(float taille, const char *s);

/* Texte sur plusieurs lignes, retour à la ligne avant largeur. Hauteur dessinée. */
float ecran_paragraphe(float x, float y, float taille, u32 couleur, float largeur, const char *s);

/* Le même sur max lignes au plus : la dernière finit par « … » s'il reste du texte. Hauteur dessinée. */
float ecran_lignes(float x, float y, float taille, u32 couleur, float largeur, int max, const char *s);

/* Hauteur qu'aurait ce paragraphe, sans le dessiner. */
float ecran_hauteur(float taille, float largeur, const char *s);

/* Icône en RGB565 ligne par ligne (l, h <= 64), agrandie par echelle. */
void ecran_icone(const u16 *pixels, int l, int h, float x, float y, float echelle);

#endif
