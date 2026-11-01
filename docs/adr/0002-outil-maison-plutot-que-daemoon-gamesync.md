# Outil maison (serveur + homebrew 3DS + Lanceur) plutôt qu'un projet existant

Trois pièces écrites pour ce besoin : un Serveur Python, un homebrew 3DS en C, un Lanceur PowerShell. Aucun projet existant ne couvre 3DS + melonDS + HTTPS public + Historique + Lanceur automatique (recherche du 2026-10-31, `docs/research.md` partie 2 §3).

## Options écartées

- **DaeMoon** (mirusu400) : bon client 3DS HTTPS et serveur Docker, mais aucun client melonDS. Le PC aurait dû reproduire son protocole (paquets zip, manifeste, digest, appairage) ; son CLI semble ne parler qu'en HTTP clair. Plus cher que notre petit client 3DS, et dépendance à une v0.1.2 d'un seul auteur.
- **GameSync** (pepasjc) : 3DS + PC, mais HTTP local seulement (`httpc`), client PC graphique et non automatique, projet couvrant 15 plateformes.
- **ftpd + script PC** : aucun code 3DS, mais PC allumé et même réseau obligatoires, ftpd à lancer à la main, pas d'Historique.

On reprend de DaeMoon les idées vérifiées sur matériel (curl + mbedTLS, racine ISRG embarquée, Code jeu lu dans la ROM), pas son code.
