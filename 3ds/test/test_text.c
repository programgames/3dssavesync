/* Vérifie text.c : pluriels, âge d'une version, textes qui tiennent dans une place.
 * Compilé sur PC : gcc -std=c99 -Wall -I source source/text.c test/test_text.c -o /tmp/x && /tmp/x */
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

static void check_int(int actual, int expected, const char *what)
{
    total++;
    if (actual != expected) {
        printf("ECHEC : %s : attendu %d, obtenu %d\n", what, expected, actual);
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
    const float l[] = { 50, 60, 70 };

    check(plural(0, 0, "conflit", "conflits"), "conflit"); /* français : 0 et 1 au singulier */
    check(plural(1, 0, "conflit", "conflits"), "conflit");
    check(plural(2, 0, "conflit", "conflits"), "conflits");
    check(plural(0, 1, "conflict", "conflicts"), "conflicts"); /* anglais : 0 au pluriel */
    check(plural(1, 1, "conflict", "conflicts"), "conflict");
    check(plural(12, 1, "conflict", "conflicts"), "conflicts");

    check(age(-1, 0, 0), "");
    check(age(30, 0, 0), "à l'instant");
    check(age(30, 0, 1), "À l'instant");
    check(age(30, 1, 1), "Just now");
    check(age(125, 0, 0), "il y a 2 min");
    check(age(7200, 0, 1), "Il y a 2 h");
    check(age(86400, 1, 0), "1 d ago"); /* pas "1 days ago" */
    check(age(6 * 86400 + 5, 0, 1), "Il y a 6 j");

    /* 50 + 60 + 70 avec la marge de 8 % et 12 pixels entre deux : 54 ; 54 + 12 + 64,8 = 130,8 ; + 12 + 75,6 = 218,4. */
    check_int(texts_fitting(l, 3, 12, 300), 3, "tout tient");
    check_int(texts_fitting(l, 3, 12, 218), 2, "le dernier part");
    check_int(texts_fitting(l, 3, 12, 130), 1, "un seul");
    check_int(texts_fitting(l, 3, 12, 53), 0, "rien");
    check_int(texts_fitting(l, 0, 12, 300), 0, "liste vide");

    printf("%d/%d cas OK\n", total - failures, total);
    return failures != 0;
}
