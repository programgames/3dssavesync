/* Vérifie texte.c : pluriels, âge d'une version, textes qui tiennent dans une place.
 * Compilé sur PC : gcc -std=c99 -Wall -I source source/texte.c test/test_texte.c -o /tmp/x && /tmp/x */
#include <stdio.h>
#include <string.h>
#include "text.h"

static int echecs, total;

static void cas(const char *obtenu, const char *attendu)
{
    total++;
    if (strcmp(obtenu, attendu)) {
        printf("ECHEC : attendu « %s », obtenu « %s »\n", attendu, obtenu);
        echecs++;
    }
}

static void cas_n(int obtenu, int attendu, const char *quoi)
{
    total++;
    if (obtenu != attendu) {
        printf("ECHEC : %s : attendu %d, obtenu %d\n", quoi, attendu, obtenu);
        echecs++;
    }
}

static const char *age(long s, int anglais, int majuscule)
{
    static char t[32];
    format_age(s, anglais, majuscule, t, sizeof t);
    return t;
}

int main(void)
{
    const float l[] = { 50, 60, 70 };

    cas(pluriel(0, 0, "conflit", "conflits"), "conflit"); /* français : 0 et 1 au singulier */
    cas(pluriel(1, 0, "conflit", "conflits"), "conflit");
    cas(pluriel(2, 0, "conflit", "conflits"), "conflits");
    cas(pluriel(0, 1, "conflict", "conflicts"), "conflicts"); /* anglais : 0 au pluriel */
    cas(pluriel(1, 1, "conflict", "conflicts"), "conflict");
    cas(pluriel(12, 1, "conflict", "conflicts"), "conflicts");

    cas(age(-1, 0, 0), "");
    cas(age(30, 0, 0), "à l'instant");
    cas(age(30, 0, 1), "À l'instant");
    cas(age(30, 1, 1), "Just now");
    cas(age(125, 0, 0), "il y a 2 min");
    cas(age(7200, 0, 1), "Il y a 2 h");
    cas(age(86400, 1, 0), "1 d ago"); /* pas "1 days ago" */
    cas(age(6 * 86400 + 5, 0, 1), "Il y a 6 j");

    /* 50 + 60 + 70 avec la marge de 8 % et 12 pixels entre deux : 54 ; 54 + 12 + 64,8 = 130,8 ; + 12 + 75,6 = 218,4. */
    cas_n(textes_tenant(l, 3, 12, 300), 3, "tout tient");
    cas_n(textes_tenant(l, 3, 12, 218), 2, "le dernier part");
    cas_n(textes_tenant(l, 3, 12, 130), 1, "un seul");
    cas_n(textes_tenant(l, 3, 12, 53), 0, "rien");
    cas_n(textes_tenant(l, 0, 12, 300), 0, "liste vide");

    printf("%d/%d cas OK\n", total - echecs, total);
    return echecs != 0;
}
