/* Textes calculés de l'écran de la 3DS : pluriels, âge d'une version, place des textes. Sans libctru : testé sur
 * l'hôte (test/test_text.c). */
#ifndef TEXT_H
#define TEXT_H

#include <stddef.h>

/* Marge sur une largeur mesurée : la police d'Azahar (captures) n'est pas tout à fait celle de la 3DS. */
#define WIDTH_MARGIN 1.08f

/* un pour 1 (et 0 en français), plusieurs sinon. */
const char *plural(long n, int english, const char *one, const char *many);

/* Âge de s secondes : « il y a 5 min », « à l'instant »… ; vide si s < 0. majuscule : en début de valeur. */
void format_age(long s, int english, int capitalize, char *out, size_t n);

/* Nombre des premiers textes (largeurs l, dans l'ordre) qui tiennent dans place, ecart entre deux, marge comprise. */
int texts_fitting(const float *l, int n, float gap, float room);

#endif
