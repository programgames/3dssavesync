/* Vérifie deplacer_sauvegarde (bascule forwarder mGBA <-> .sav à côté de la ROM) : jamais de perte.
 * Compilé sur PC : gcc -std=c99 -I source source/fichiers.c test/test_fichiers.c -o /tmp/f && /tmp/f */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "fichiers.h"

static int echecs;

static void verifier(int ok, const char *quoi)
{
    if (!ok) {
        printf("ECHEC  %s\n", quoi);
        echecs++;
    }
}

static void ecrire(const char *chemin, const char *texte)
{
    ecrire_fichier(chemin, texte, strlen(texte));
}

/* 1 si le fichier contient exactement texte. */
static int contient(const char *chemin, const char *texte)
{
    unsigned char *o;
    size_t n;
    int ok;
    if (lire_fichier(chemin, &o, &n)) return 0;
    ok = n == strlen(texte) && !memcmp(o, texte, n);
    free(o);
    return ok;
}

static int existe(const char *chemin)
{
    FILE *f = fopen(chemin, "rb");
    if (f) fclose(f);
    return f != NULL;
}

int main(void)
{
    const char *src = "test-src.sav", *dst = "test-dst.sav", *copie = "test-copie.sav";
    remove(src), remove(dst), remove(copie);

    ecrire(src, "partie forwarder");
    ecrire(dst, "ancienne partie rom");
    verifier(deplacer_sauvegarde(src, dst, copie, 1) == 0, "vers la ROM : fait");
    verifier(contient(dst, "partie forwarder"), "vers la ROM : la ROM a la partie du forwarder");
    verifier(contient(copie, "ancienne partie rom"), "vers la ROM : copie de l'ancien .sav");
    verifier(!existe(src), "vers la ROM : le fichier du forwarder est retiré");

    remove(copie);
    ecrire(src, "partie rom");
    verifier(deplacer_sauvegarde(src, dst, copie, 0) == 0, "vers le forwarder : fait");
    verifier(contient(dst, "partie rom") && contient(src, "partie rom"), "vers le forwarder : les deux fichiers ont la partie");
    verifier(contient(copie, "partie forwarder"), "vers le forwarder : copie de l'ancien fichier");

    remove(copie);
    remove(dst);
    verifier(deplacer_sauvegarde(src, dst, copie, 1) == 0 && contient(dst, "partie rom") && !existe(copie),
             "cible absente : pas de copie nécessaire");

    verifier(deplacer_sauvegarde("absent.sav", dst, copie, 1) != 0 && contient(dst, "partie rom"),
             "source absente : refus, rien ne change");

    ecrire(src, "nouvelle");
    verifier(deplacer_sauvegarde(src, dst, "dossier-absent/copie.sav", 1) != 0, "copie impossible : refus");
    verifier(contient(dst, "partie rom") && contient(src, "nouvelle"), "copie impossible : rien ne change");

    remove(src), remove(dst), remove(copie), remove("test-dst.sav.tmp");
    printf("%s\n", echecs ? "ECHECS" : "fichiers : OK");
    return echecs != 0;
}
