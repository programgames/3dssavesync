/* Computed texts of the screen (text.h). */
#include <stdio.h>
#include <string.h>
#include "text.h"

const char *plural(long n, int english, const char *one, const char *many)
{
    return (english ? n == 1 : n < 2) ? one : many; /* French: "0 conflit", English: "0 conflicts" */
}

void format_age(long s, int english, int capitalize, char *out, size_t n)
{
    if (s < 0) snprintf(out, n, "%s", "");
    else if (s < 60) snprintf(out, n, "%s", english ? "just now" : "à l'instant");
    else if (s < 3600) snprintf(out, n, english ? "%ld min ago" : "il y a %ld min", s / 60);
    else if (s < 86400) snprintf(out, n, english ? "%ld h ago" : "il y a %ld h", s / 3600);
    else snprintf(out, n, english ? "%ld d ago" : "il y a %ld j", s / 86400);
    if (!capitalize) return;
    if (out[0] >= 'a' && out[0] <= 'z') out[0] -= 'a' - 'A';
    else if (!strncmp(out, "à", 2)) out[1] = (char)0x80; /* à -> À in UTF-8 */
}
