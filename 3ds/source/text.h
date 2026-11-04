/* Computed texts of the 3DS screen: plurals, age of a version. No libctru: tested on
 * the host (test/test_text.c). */
#ifndef TEXT_H
#define TEXT_H

#include <stddef.h>

/* Margin on a measured width: the Azahar font (screenshots) is not quite the 3DS one. */
#define WIDTH_MARGIN 1.08f

/* one for 1 (and 0 in French), many otherwise. */
const char *plural(long n, int english, const char *one, const char *many);

/* Age of s seconds: "il y a 5 min", "à l'instant"…; empty if s < 0. capitalize: at the start of a value. */
void format_age(long s, int english, int capitalize, char *out, size_t n);

#endif
