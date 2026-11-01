# Synchronisation à trois points par Hash, jamais par date

Chaque Client garde une Base par Jeu et compare trois Hash : Sauvegarde locale (L), Version courante (S), Base (B). Les dates de modification sont inutilisables : nds-bootstrap écrit les secteurs du `.sav` sans mettre à jour l'entrée FAT, donc le `mtime` ne bouge pas quand on joue sur la 3DS (voir `docs/research.md`, partie 1 §1), et l'horloge de la 3DS est réglable à la main.

| Cas | État |
|---|---|
| L == S | À jour (et B := L) |
| S absent | À envoyer |
| L absent | À récupérer |
| taille(L) != taille(S) | Conflit |
| L == B et S != B | À récupérer |
| S == B et L != B | À envoyer |
| sinon | Conflit |

## Conséquences

- Une taille différente donne toujours un Conflit : melonDS tronque ou complète un `.sav` à la taille de sa base, et nds-bootstrap écrit modulo la taille du fichier. Synchroniser une mauvaise taille peut corrompre la partie.
- L'Envoi porte `If-Match: <S vu par le Client>`. Si un autre Client a envoyé entre-temps, le Serveur répond `412` et le Client repasse en Conflit. Aucun Conflit n'est jamais résolu automatiquement.
