/* Temps de jeu et dresseur des Pokémon, pour comparer deux Versions avant un choix (écran de Conflit,
 * Récupération). Aucune dépendance libctru : testé sur PC (test/test_pokemon.c). */
#ifndef POKEMON_H
#define POKEMON_H

#include <stddef.h>

typedef struct {
    int heures, minutes;
    char dresseur[40]; /* UTF-8 ; vide : partie jamais commencée */
} partie_t;

/* 0 si la sauvegarde est un Pokémon reconnu : Rouge/Bleu/Jaune, Or/Argent/Cristal, Rubis/Saphir/Émeraude/Rouge Feu/
 * Vert Feuille, Diamant/Perle/Platine/HeartGold/SoulSilver, Noir/Blanc 1 et 2 (versions européennes et américaines),
 * puis les jeux 3DS (fichier main). La sauvegarde est la SRAM seule, sans Trailer d'horloge. -1 sinon. */
int pokemon_partie(const unsigned char *d, size_t taille, partie_t *p);

#endif
