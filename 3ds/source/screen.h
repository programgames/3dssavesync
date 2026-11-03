/* Dessin des deux écrans avec citro2d (issue #21) : texte en police du système (accents, icônes des boutons),
 * rectangles, icône de jeu. */
#ifndef SCREEN_H
#define SCREEN_H

#include <3ds.h>

#define RGB(c) (0xFF000000u | ((c) & 0xFF) << 16 | ((c) & 0xFF00) | ((c) >> 16 & 0xFF)) /* 0xRRGGBB -> citro2d */

void screen_init(void);
void screen_exit(void);

/* Une image : screen_begin, puis screen_top ou screen_bottom avant de dessiner sur cet écran, puis screen_end. */
void screen_begin(void);
void screen_top(void);
void screen_bottom(void);
void screen_end(void);

void screen_rect(float x, float y, float l, float h, u32 color);

/* Texte sur une ligne, coupé avec « … » au-delà de largeur (0 = sans limite). Largeur dessinée. */
float screen_text(float x, float y, float size, u32 color, float width, const char *s);

/* Largeur du texte sur une ligne. */
float screen_text_width(float size, const char *s);

/* Texte sur plusieurs lignes, retour à la ligne avant largeur. Hauteur dessinée. */
float screen_paragraph(float x, float y, float size, u32 color, float width, const char *s);

/* Le même sur max lignes au plus : la dernière finit par « … » s'il reste du texte. Hauteur dessinée. */
float screen_lines(float x, float y, float size, u32 color, float width, int max, const char *s);

/* Hauteur qu'aurait ce paragraphe, sans le dessiner. */
float screen_paragraph_height(float size, float width, const char *s);

/* Icône en RGB565 ligne par ligne (l, h <= 64), agrandie par echelle. */
void screen_icon(const u16 *pixels, int l, int h, float x, float y, float scale);

#endif
