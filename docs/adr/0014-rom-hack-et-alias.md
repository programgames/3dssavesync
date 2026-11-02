# ROM hack sous la Clé d'un autre jeu : refus, puis alias

Un ROM hack garde souvent le code jeu de son original (Pokémon Radical Red garde `BPRE`, celui de Rouge Feu). Pour l'ADR 0005, même Clé = même Jeu. Sur le PC, le hack et l'original ont deux fichiers de Sauvegarde (nommés d'après la ROM), mais une seule Base et une seule Version courante : jouer à l'un puis à l'autre envoie la partie du hack par-dessus celle de l'original, ou l'inverse.

Règle :

- **PC, `run`** : à chaque Base écrite, le Lanceur retient la ROM de cette Clé dans `bases/roms/<émulateur>.json`, à côté de `bases/<émulateur>.json` (inchangé, toujours lisible par une version plus ancienne). Une autre ROM de même Clé, alors que la ROM retenue existe encore (`os.SameFile`) : partie sans Synchronisation, notification qui nomme les deux fichiers et propose un alias. ROM retenue disparue (renommée, déplacée, extraction temporaire de LaunchBox) : la nouvelle est acceptée et retenue. Aucune ROM retenue (Base d'avant cette règle) : la ROM lancée est acceptée et retenue.
- **PC, `launch`** : deux ROM de même Clé dans les dossiers `roms` : aucune n'est synchronisée, une seule notification (comme le Doublon de la 3DS). Une ROM seule pour sa Clé passe par la même règle que `run`.
- **Alias** : `"alias": {"<nom de la ROM sans extension>": "<Clé>"}` dans `config.json`, et `alias.ini` (`<nom de la ROM sans extension>=<Clé>`, une ligne par ROM) dans `sd:/3ds/3dssavesync/` sur la 3DS. Le nom est celui du fichier de chaque côté, sans changement de casse. La Clé est mise en majuscules et sans espaces autour ; elle doit avoir la forme de la Clé du Système de la ROM (ADR 0007) : `0RAD` (DS), `GBA-0RAD` (GBA), `GB-MONHACK-0001` (GB). Le même alias donne la même Clé sur les deux machines (`cle_alias` dans `3ds/source/cle.c`, `cleAlias` dans `client/cas.go`, mêmes cas de test). Avec un alias, la règle du Doublon ne compte plus cette ROM avec l'original.
- **Alias invalide** : la ROM n'est pas synchronisée, jamais sous la Clé de son en-tête. Le message donne un exemple valide.
- **3DS** : pas d'alias pour les jeux 3DS (Clé = title ID, et Azahar range la sauvegarde par title ID : deux ROM d'un jeu 3DS ont la même).

Conseil de la documentation : commencer l'alias par un chiffre (`GBA-0RAD`). Les vrais codes jeu DS et GBA commencent par une lettre, donc l'alias ne rencontre aucun jeu du commerce.

## Options écartées

- **Clé allongée (`GBA-BPRE-RADICALRED`)** : il faut changer l'expression des Clés du serveur Python et du serveur local, puis redéployer le serveur. Un alias de la forme actuelle suffit. L'ajout reste possible plus tard sans casser l'existant.
- **Clé = en-tête + Hash de la ROM** : une ROM corrigée (patch de traduction, autre dump) changerait de Jeu et perdrait son Historique.
- **Deviner le hack** (titre interne, nom de fichier) : l'outil ne devine jamais (ADR 0005).

## Conséquences

- Un hack synchronisé avant l'original devient la ROM retenue : c'est alors l'original qui est refusé. Le message dit quoi faire (alias pour l'une des deux).
- Une ROM retenue sur un disque débranché compte comme disparue : la ROM lancée est acceptée.
- Pour partager la partie d'un hack entre la 3DS et le PC, il faut le même alias des deux côtés.
