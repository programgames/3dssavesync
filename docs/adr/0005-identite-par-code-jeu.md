# Un Jeu = un Code jeu ; noms de fichiers ignorés, slot 0 seulement, Doublons ignorés

TWiLight et melonDS nomment le `.sav` d'après le nom de fichier de la ROM, et ces noms diffèrent entre la 3DS et le PC (noms No-Intro sur le PC). L'identité d'un Jeu est donc le Code jeu à l'offset `0x0C` de l'en-tête ROM, région incluse.

- Codes `####`, vides ou non ASCII (homebrews) : exclus.
- Seul le slot 0 (`<nom ROM>.sav`) est synchronisé ; les Slots TWiLight `.sav1`..`.sav9` sont ignorés.
- Si un Client a plusieurs ROM avec le même Code jeu, le Jeu est marqué Doublon et n'est pas synchronisé : l'outil ne devine jamais quel fichier est le bon.

## Conséquences

- Deux régions du même titre (`IRBF`, `IRBO`) sont deux Jeux distincts. La compatibilité des Sauvegardes entre langues n'est pas vérifiée, donc on ne les apparie pas.
- Un ROM hack qui garde le Code jeu du jeu d'origine partage son identité : à éviter sur un même Client (il apparaîtra en Doublon). Complété par l'ADR 0014 (refus sur le PC, alias).
