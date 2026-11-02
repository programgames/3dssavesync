# Restauration depuis le PC, sans écriture locale

La Restauration (ADR 0004) se fait sur le PC seulement : partie « Historique » de la page des réglages, et `3dssavesync restore`. Rien sur la 3DS.

Le Client lit l'ancienne Version (`GET /saves/<clé>/history/<version>`), vérifie son Hash, puis l'envoie comme nouvelle Version avec `If-Match` = le Hash de la Version courante que l'utilisateur a vue dans la liste. Il n'écrit aucun fichier local et ne change aucune Base.

Raison : la Base de chaque appareil reste le Hash de sa dernière Synchronisation. La Version restaurée est donc, pour chaque appareil, une Version plus récente (`L == B`, `S != B` : À récupérer). Chaque appareil la reçoit par sa Récupération normale, avec sa Copie de sécurité. Écrire la Sauvegarde locale du PC au moment de la Restauration demanderait de choisir un émulateur, de vérifier qu'il ne tourne pas et de refaire la Copie de sécurité : c'est déjà le travail de la Récupération.

## Conséquences

- Un Envoi arrivé entre l'affichage de la liste et la Restauration donne `412` : la Restauration est refusée, rien n'est écrasé. L'utilisateur recharge la liste.
- Une Version au même contenu que la courante : rien n'est envoyé.
- Un appareil avec une partie pas encore envoyée (`L != B`) passe en Conflit ; l'utilisateur choisit, comme pour tout Conflit.
- Une Version d'une autre taille que la Sauvegarde locale donne aussi un Conflit (ADR 0001).
- `X-Client: restauration` : l'Historique montre qu'une Version vient d'une Restauration.
- Les Versions sont numérotées par leur `<seq>` du serveur : un numéro ne change jamais, même si une Version arrive entre la liste et la commande.
- Liste limitée aux 30 dernières Versions. Pour un Pokémon, chaque Version de la liste est lue en entier (temps de jeu, dresseur) : jusqu'à 30 lectures par liste.
