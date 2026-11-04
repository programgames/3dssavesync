/* Checks text.c: plurals, age of a version.
 * Built on the PC: gcc -std=c99 -Wall -I source source/text.c test/test_text.c -o /tmp/x && /tmp/x */
#include <stdio.h>
#include <string.h>
#include "text.h"

static int failures, total;

static void check(const char *actual, const char *expected)
{
    total++;
    if (strcmp(actual, expected)) {
        printf("ECHEC : attendu « %s », obtenu « %s »\n", expected, actual);
        failures++;
    }
}

static const char *age(long s, int english, int capitalize)
{
    static char t[32];
    format_age(s, english, capitalize, t, sizeof t);
    return t;
}

int main(void)
{
    check(plural(0, 0, "conflit", "conflits"), "conflit"); /* French: 0 and 1 are singular */
    check(plural(1, 0, "conflit", "conflits"), "conflit");
    check(plural(2, 0, "conflit", "conflits"), "conflits");
    check(plural(0, 1, "conflict", "conflicts"), "conflicts"); /* English: 0 is plural */
    check(plural(1, 1, "conflict", "conflicts"), "conflict");
    check(plural(12, 1, "conflict", "conflicts"), "conflicts");

    check(age(-1, 0, 0), "");
    check(age(30, 0, 0), "à l'instant");
    check(age(30, 0, 1), "À l'instant");
    check(age(30, 1, 1), "Just now");
    check(age(125, 0, 0), "il y a 2 min");
    check(age(7200, 0, 1), "Il y a 2 h");
    check(age(86400, 1, 0), "1 d ago"); /* not "1 days ago" */
    check(age(6 * 86400 + 5, 0, 1), "Il y a 6 j");

    printf("%d/%d cas OK\n", total - failures, total);
    return failures != 0;
}
