/* Tabs, search and order of the game list (filter.h). */
#include <ctype.h>
#include "filter.h"

int in_tab(tab_t t, const row_t *r)
{
    switch (t) {
    case TAB_ALL: return 1;
    case TAB_TODO:
        return r->status == STATUS_TO_UPLOAD || r->status == STATUS_TO_DOWNLOAD || r->status == STATUS_CONFLICT ||
               r->status == STATUS_DUPLICATE;
    default: return r->system == (int)(t - TAB_DS); /* TAB_DS..TAB_3DS = SYS_DS..SYS_3DS */
    }
}

tab_t next_tab(tab_t t, int step, const int *counts)
{
    int i;
    for (i = 0; i < TAB_COUNT; i++) {
        t = (tab_t)((t + step + TAB_COUNT) % TAB_COUNT);
        if (t == TAB_ALL || counts[t]) break;
    }
    return t;
}

/* Next character of *s, lower case, accent dropped: U+00C0 to U+00FF (UTF-8 C3 80 to C3 BF) become their ASCII
 * letter; any other byte is returned as is. */
static int fold(const char **s)
{
    static const char LATIN1[] = "aaaaaaaceeeeiiiidnooooo*ouuuuy**aaaaaaaceeeeiiiidnooooo*ouuuuy*y"; /* * : kept as is */
    const unsigned char *p = (const unsigned char *)*s;
    if (p[0] == 0xC3 && p[1] >= 0x80 && p[1] <= 0xBF && LATIN1[p[1] - 0x80] != '*') {
        *s += 2;
        return LATIN1[p[1] - 0x80];
    }
    (*s)++;
    return p[0] < 0x80 ? tolower(p[0]) : p[0];
}

int compare_names(const char *a, const char *b)
{
    while (*a || *b) {
        int x = *a ? fold(&a) : 0, y = *b ? fold(&b) : 0;
        if (x != y) return x - y;
    }
    return 0;
}

int compare_rows(order_t o, const row_t *a, const row_t *b)
{
    int d = 0;
    if (o == ORDER_TODO) d = status_priority(a->status) - status_priority(b->status);
    else if (o == ORDER_RECENT) d = (b->modified > a->modified) - (b->modified < a->modified); /* 0 (no date): last */
    return d ? d : compare_names(a->name, b->name);
}

int name_matches(const char *name, const char *query)
{
    for (;;) {
        const char *n = name, *q = query;
        int same = 1;
        while (*q && same) same = *n && fold(&n) == fold(&q);
        if (same) return 1;
        if (!*name) return 0;
        fold(&name); /* next character */
    }
}
