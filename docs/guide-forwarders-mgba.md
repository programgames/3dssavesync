# Icônes HOME pour les Pokémon GB et GBA (forwarders mGBA)

Un forwarder mGBA est un `.cia` par jeu : mGBA et la ROM dans une seule icône du menu HOME. Source : `docs/research-v2.md`, partie 2 §3c.

**Différence importante** : un forwarder sauvegarde dans `sdmc:/mGBA/forwarders/<nom de la ROM>.sav`, pas à côté de la ROM. ndssync prend ce fichier **dès qu'il existe** ; sinon il garde le `.sav` à côté de la ROM. La ROM reste dans `sd:/roms/gba` (ou `sd:/roms/gb`) : ndssync y lit la Clé.

⚠️ Pour un jeu qui a un forwarder, joue **seulement** avec le forwarder. open_agb_firm et TWiLight lisent toujours l'ancien `.sav` à côté de la ROM : la partie ne serait plus la même.

## 1. Sur le PC : obtenir l'outil

1. Télécharge la version de développement de mGBA pour Windows (64 bits, portable) : https://s3.amazonaws.com/mgba/mGBA-build-latest-win64.7z (page : https://mgba.io/downloads.html, partie « Development downloads »).
2. Décompresse-la dans un **nouveau** dossier, par exemple `<mGBA-dev>`. Ne remplace pas le mGBA de LaunchBox.
3. N'ouvre aucun jeu avec cette version : elle sert seulement à créer les forwarders.

## 2. Sur le PC : la ROM

Le nom du `.sav` vient du nom de la ROM choisie. Il doit être **identique** au nom de la ROM dans `sd:/roms/gba`, extension mise à part.

- Le plus sûr : prends la ROM sur la SD elle-même (ftpd, puis Claude la copie sur le PC).

## 3. Sur le PC : créer le forwarder

Avec l'outil en ligne de commande `pc/forwarder3ds.py` (mêmes étapes que mGBA, chaque échec est signalé) :

```
python pc/forwarder3ds.py --rom "<ROM>" --titre "Pokémon Rubis" --icone "<48x48.png>" --banniere "<256x128.png>" --sortie "<Jeu>.cia"
```

Il utilise `ctrtool`, `3dstool`, `bannertool` et `makerom` de `<mGBA-dev>` (forwarder-kit de mGBA), et la base `mGBA-dev\forwarder\base-mgba-3ds-dev-9167.cia` : le `.cia` du build 3DS « dev » indiqué par https://mgba.io/latest.ini (`[platform.3ds]`, `dev.name`, vérifié par `dev.sha256`). Même ROM = même title ID : FBI remplace le forwarder déjà installé.

Pourquoi pas l'interface de mGBA (Tools → Create forwarder…) : elle ignore les échecs des outils. Au premier forwarder sans icône, elle copie son icône par défaut dans `mGBA-dev\smdh.png` **en lecture seule** ; tous les forwarders suivants gardent alors cette icône, quel que soit le choix. Si tu l'utilises quand même, supprime d'abord ce fichier.

## 4. Sur la console : installer et reprendre la partie

1. Avant tout, ouvre ndssync et appuie sur START : la partie actuelle doit être sur le serveur.
2. Copie le `.cia` sur la SD (ftpd), puis installe-le avec **FBI**.
3. Avant le premier lancement, copie le `.sav` actuel dans `sdmc:/mGBA/forwarders/` sous le même nom. L'original reste à côté de la ROM. Claude peut le faire par FTP.
4. Lance le forwarder. Vérifie que ta partie se charge.
5. Joue, sauvegarde en jeu, puis quitte par HOME.
6. Ouvre ndssync. Dans le détail du jeu, la ligne « Fichier : forwarder mGBA » s'affiche. Appuie sur START.

## Tous les Pokémon d'un coup (préparé le 2026-11-01)

Dans `<mGBA-dev>` :
- `roms-forwarders\` : les ROM, copiées de la SD (noms identiques) ;
- `images\<Jeu> - icone.png` (48×48, depuis la jaquette LaunchBox) et `images\<Jeu> - banniere.png` (256×128 : fond « Fanart » et logo français « Clear Logo » de LaunchBox) ;
- `cia\` : les `.cia` créés.

Les `.sav` actuels sont déjà copiés dans `sdmc:/mGBA/forwarders/` (jamais d'écrasement d'un fichier présent).

Les 11 `.cia` (Rouge, Bleue, Jaune, Or, Argent, Cristal, Rubis, Saphir, Émeraude, Rouge Feu, Vert Feuille) sont créés avec `pc/forwarder3ds.py` (titres « Pokémon Rouge », « Pokémon Émeraude »…). Ensuite, Claude envoie `cia\` dans `sd:/cias/` et FBI installe tout (`<current directory>` → « Install all CIAs »).

⚠️ **Rechargement automatique de mGBA 3DS** : par défaut, mGBA écrit un état (`.ss0`) en quittant et le recharge au lancement suivant (`autoload`, `autosave`). Après une Récupération venue du PC, il reprendrait donc l'ancienne position ; une sauvegarde en jeu écraserait alors la progression du PC. Mettre `autoload=0` et `autosave=0` dans `[ports.3ds]` de `sd:/mGBA/config.ini`.

L'application mGBA (sans forwarder) utilise aussi `savegamePath=/mGBA/forwarders` : le forwarder l'écrit dans la configuration commune. Pour ces jeux, TWiLight et open_agb_firm lisent encore l'ancien `.sav` à côté de la ROM : ne plus les utiliser pour eux.

## Changer de programme pour un jeu (touche X de ndssync)

Dans ndssync, sur un jeu GB ou GBA, **X** fait passer la Sauvegarde d'un côté à l'autre, avec une Copie de sécurité du fichier remplacé :
- forwarder → ROM : le `.sav` du forwarder remplace celui à côté de la ROM, puis il est retiré de `mGBA/forwarders` (open_agb_firm ou TWiLight le lisent ensuite) ;
- ROM → forwarder : le `.sav` à côté de la ROM est copié dans `mGBA/forwarders`.

Le détail du jeu affiche le côté actif (« Fichier : forwarder mGBA », « open_agb_firm » ou « TWiLight »). Appuie sur X **avant** de lancer l'autre programme.

## Limites

- New 3DS obligatoire pour le GBA (tu l'as).
- Pour mettre mGBA à jour, il faut recréer chaque forwarder.
- Or, Argent, Cristal : chaque programme garde sa propre horloge (voir ADR 0008).
