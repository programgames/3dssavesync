# Le Serveur ne supprime ni ne modifie jamais une Version

Chaque Envoi accepté crée un nouveau fichier `data/<Code jeu>/<seq>-<date>-<client>-<Hash>.sav`. La Version courante est celle de plus grand `<seq>` : aucun pointeur à maintenir, aucune dépendance à l'horloge. Aucune route ne supprime, n'écrase ni ne fait reculer la Version courante. Une Restauration se fait en renvoyant une ancienne Version comme nouvel Envoi depuis un Client.

Raison : une partie Pokémon représente des centaines d'heures. Le pire scénario (jeton volé, bug d'un Client, mauvais choix dans un Conflit) doit rester réparable en copiant un fichier.

## Conséquences

- `<seq>` vaut le `<seq>` de la dernière Version + 1 : retirer un fichier à la main ne casse pas l'ordre.
- Le Serveur refuse un corps plus court que son `Content-Length` (connexion coupée) : jamais de Version tronquée.
- Le disque grossit sans limite : environ 512 Kio par Envoi, soit quelques centaines de Mio par an au pire. Pas de purge en v1.
- L'Historique vit sur le même disque que le Serveur ; le dossier `data/` doit faire partie d'une sauvegarde externe (restic, Backrest, etc.).
- Les données de test sont déplacées hors de `data/` (`quarantaine-tests/`), jamais supprimées par un script.
