/* Liste du Serveur (GET /saves, docs/design.md §2). Aucune dépendance libctru : testé sur PC (test/test_liste.c). */
#ifndef LISTE_H
#define LISTE_H

#include "key.h"

/* Un jeu de la liste : {"code": "IRBF", "version": "...", "hash": "...", "taille": 524288, "date": "...",
 * "client": "pc", "age": 120, "nom": "..."}. Champ absent : "" (texte), -1 (taille, age), 0 (a_nom).
 * hash : 64 chiffres hexadécimaux minuscules, sinon "". */
typedef struct {
    char code[CLE_MAX], hash[65], date[17], client[17];
    long taille, age;
    int a_nom;
} entree_t;

/* Jeu suivant de la liste JSON à partir de *p : chaque champ est lu dans son propre objet, espaces permis autour de
 * ":" et ",". 1 si lu (*p passe après l'objet), 0 à la fin de la liste ou sur un texte mal formé. */
int liste_serveur(const char **p, entree_t *e);

#endif
