# Plan v3 « 3dssavesync »

Recherches : `research-v3-cloud.md`, `research-v3-emulateurs.md`, `research-v3-frontends.md`.
Règles inchangées : aucune perte de sauvegarde (copie avant tout écrasement), un Conflit n'est jamais résolu seul, synchro à trois points par hash (ADR 0001), le serveur n'efface rien.

Chaque phase se termine par une démonstration et une validation avant la suivante.

## Phase 0 — Nouveau dépôt et intégration continue

- Dépôt `3dssavesync`, un commit de départ : code et docs du projet d'origine, nettoyés.
- Arborescence : `server/` (Python, Docker), `cloud/` (Cloudflare), `client/` (Go), `3ds/` (homebrew), `docs/`.
- GitHub Actions : tests serveur, tests Go sur Windows, Linux et macOS, compilation du `.cia`/`.3dsx` (image Docker devkitPro), binaires Go, image Docker, release automatique sur tag.
- Une suite de tests du **contrat HTTP**, lancée contre n'importe quelle URL : elle sert pour le serveur Python et pour la version Cloudflare.

## Phase 1 — Programme PC en Go (cœur)

- Reprise de `sync.ps1` : Clés (DS, GB, GBA, 3DS), SRAM, État (mêmes `cas_etat.csv` que le C), Envoi conditionnel, Récupération avec copie, `.tmp` vérifié.
- Commandes :
  - `3dssavesync run [--emulator <profil>] <rom>` : mode « un jeu » des frontends ;
  - `3dssavesync run --rom <rom> -- <commande>` : enveloppe une commande quelconque (RetroBat, EmuDeck) ;
  - `3dssavesync launch <profil>` : synchronise tous les jeux du profil, lance l'émulateur, envoie à la fermeture.
- Pas de console sous Windows ; l'envoi se termine même si le frontend arrête le programme.
- Reprise des 68 vérifications de `test_lanceur.ps1` en tests Go contre le vrai serveur.
- **Migration d'une installation v2** (`sync.config.json`, `base.json`) testée sur des copies, puis bascule de LaunchBox. L'ancien `sync.ps1` reste en secours.

## Phase 2 — Profils d'émulateurs

- Format de profil : exécutable, arguments, processus, dossier de sauvegardes par système d'exploitation, règle de nom, format (SRAM brute, `.srm` + `.rtc`), Systèmes gérés.
- v3 : melonDS, mGBA, Azahar, RetroArch (cœurs melonDS DS, mGBA, Gambatte, SameBoy, VBA-M), VBA-M et SameBoy autonomes. Chemins par défaut : Windows, Linux (dont Flatpak), macOS, EmuDeck, RetroDECK.
- Profil personnalisé dans la configuration.
- Avertissement quand un émulateur recharge un état au lancement (mGBA `autoload`, RetroArch `savestate_auto_load`, VBA-M), avec le réglage à changer.
- Plus tard : DeSmuME, NO$GBA, gpSP, Beetle GBA (conversion de format nécessaire), BGB, Emulicious, SkyEmu, NanoBoyAdvance, Panda3DS.

## Phase 3 — Fenêtres, notifications, langues

- Fenêtres natives (Windows, macOS ; zenity ou kdialog sous Linux) ; sans fenêtre possible, règle Q16 : rien n'est remplacé, notification.
- Français et anglais selon la langue du système.

## Phase 4 — Assistant d'installation PC et frontends

- `3dssavesync setup` : URL et jeton du serveur, test de connexion, détection des émulateurs et de leurs dossiers, création d'un raccourci par émulateur (`.lnk`, `.desktop`, `.app`).
- Configuration automatique (avec copie de sauvegarde du fichier modifié) : LaunchBox/BigBox, ES-DE, Steam ROM Manager.
- Guides pas à pas : Playnite, EmuDeck, RetroBat, Pegasus, Lutris, Attract-Mode, RetroDECK.
- `3dssavesync repair` : refait la configuration après une mise à jour d'EmuDeck ; avertit si la synchro cloud d'EmuDeck est active.
- **Interface graphique de l'assistant** (demande du 2026-11-01) : **faite**, page web locale servie par l'exe (choix ci-dessous) ; la console reste disponible (`setup --texte`). La version texte dans une console fonctionne, mais elle n'est pas assez simple. Choix à faire :
  - page web locale servie par l'exe (`net/http`, ouverture du navigateur par défaut) : aucune dépendance, même binaire, compilation croisée inchangée, marche sur Windows, Linux, macOS et le Steam Deck en mode Bureau (recommandé) ;
  - Fyne ou Gio (fenêtre native en Go) : demande cgo et OpenGL, compilation croisée plus difficile, binaire plus gros ;
  - Wails (fenêtre avec moteur web) : demande WebView2 (Windows) ou webkit2gtk (Linux).

## Phase 5 — Homebrew 3DS

- Écran de réglages : une ligne par réglage, A pour modifier, explication en bas, test de connexion.
- Emplacements GB/GBA : TWiLight (dossier `saves` ou à côté de la ROM), open_agb_firm (`useSavesFolder`), forwarders mGBA ; la touche X reste.
- Liste des jeux 3DS natifs configurable (8 Pokémon par défaut).
- Français et anglais selon la langue de la console.
- Certificats racine : ISRG Root X1 suffit (Let's Encrypt). Paquet de racines reporté avec Cloudflare. Serveur local : HTTP, sans certificat.
- `pc/forwarder3ds.py` rendu générique (outils et base téléchargés et vérifiés), ou intégré au programme Go.

## Phase 6 — Serveur

- Docker : image publiée, `docker-compose.yml` simple ; variante avec Caddy pour le HTTPS automatique (sans reverse proxy existant). Même format de données qu'aujourd'hui : un serveur v2 change seulement d'image.
- **Serveur local dans l'exe PC** (`3dssavesync serve`, décision du 2026-11-01) : le même contrat HTTP, réécrit en Go dans le Lanceur, sur le réseau local, en HTTP avec jeton. L'assistant le configure (démarrage avec le système, adresse et jeton à entrer sur la 3DS). Limite : synchronisation à la maison, PC allumé. Ne jamais ouvrir ce port sur Internet. Même suite de tests du contrat HTTP.
- Deux façons de servir, au choix de l'utilisateur : serveur personnel (Docker, HTTPS, synchronisation partout) ou serveur local sur le PC.
- Cloudflare : **reporté** (2026-11-01). Trop lourd pour l'utilisateur (compte, déploiement) et pour la maintenance (deuxième serveur en TypeScript, certificats variables). À reprendre seulement si des utilisateurs le demandent.

## Phase 7 — Documentation et version publique

- README anglais et français : installation en 3 étapes (serveur, 3DS, PC), tableau des émulateurs et frontends, plateformes « non testées sur appareil ».
- Release `v3.0.0` publique.

## Points connus à trancher plus tard (ADR)

- SRAM de 512 octets avec trailer d'horloge (GB MBC2, GBA EEPROM 4 Kbit) : la règle `taille - taille % 1024` garde le trailer. Rare ; les Pokémon ne sont pas concernés.
- GB MBC2 : tailles différentes selon l'émulateur ; GBA EEPROM : ordre des octets propre à open_agb_firm.
- Gamescope (Steam Deck, mode Jeu) : affichage des fenêtres non vérifié.
