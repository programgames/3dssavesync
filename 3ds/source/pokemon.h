/* Temps de jeu et dresseur des Pokémon 3DS, pour comparer deux Versions avant un choix (écran de Conflit,
 * Récupération). Aucune dépendance libctru : testé sur PC (test/test_pokemon.c). */
#ifndef POKEMON_H
#define POKEMON_H

#include <stddef.h>

typedef struct {
    int heures, minutes;
    char dresseur[40]; /* UTF-8 ; vide : partie jamais commencée */
} partie_t;

/* 0 si la sauvegarde (fichier main) est un Pokémon 3DS reconnu par sa taille : X/Y, Rubis Oméga/Saphir Alpha,
 * Soleil/Lune, Ultra-Soleil/Ultra-Lune. -1 sinon. */
int pokemon_partie(const unsigned char *d, size_t taille, partie_t *p);

#endif
