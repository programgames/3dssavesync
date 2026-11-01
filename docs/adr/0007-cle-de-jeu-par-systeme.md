# La Clé d'un Jeu dépend de son Système et se lit dans la ROM

Le Serveur range chaque Jeu sous une Clé. Avec le GB, le GBA et la 3DS, le Code jeu DS de 4 caractères ne suffit plus. Chaque Client calcule la Clé à partir de l'en-tête de la ROM, jamais du nom de fichier (les noms diffèrent entre la 3DS et le PC, ADR 0005).

| Système | Clé | Source |
|---|---|---|
| DS | `ADAF` | Code jeu, offset `0x0C` (inchangé : l'Historique existant reste valable) |
| GBA | `GBA-BPEF` | code jeu, offset `0xAC` |
| GB / GBC | `GB-POKEMON_RED-91E6` | titre `0x134`–`0x142` (majuscules, autre caractère → `_`, arrêt au premier `0x00`) + somme globale `0x14E` (big-endian) |
| 3DS | `3DS-00040000001B5000` | title ID (16 hex) |

Le Serveur accepte exactement ces quatre formes, rien d'autre (pas de `/`, pas de `.`).

## Conséquences

- Deux révisions d'un même jeu GB (somme globale différente) sont deux Jeux. Deux révisions GBA (même code jeu) sont un seul Jeu : leurs sauvegardes sont compatibles.
- Un jeu GB sans ROM visible (forwarder mGBA qui cache la ROM dans le CIA) n'a pas de Clé lisible : il faudra une ROM du même nom sur la SD pour la lire. Hors v2.
