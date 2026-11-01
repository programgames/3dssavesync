# Icônes HOME pour les Pokémon DS (forwarders)

Un forwarder est un petit titre installé sur le menu HOME qui lance une ROM DS précise avec nds-bootstrap. Il sauvegarde **au même endroit que TWiLight** (`sd:/roms/nds/saves/<nom>.sav`), donc ndssync n'a rien à changer. Source : `docs/research-v2.md`, partie 2 §3.

Tout se fait sur la console, sans code.

1. ⚠️ Avant tout, copie le dossier `sd:/_nds/` sur le PC : le « forwarder pack » peut remplacer le nds-bootstrap de TWiLight (non vérifié).
2. Ouvre **Universal Updater**, installe **NDSForwarder**, puis le **nds-bootstrap forwarder pack**.
3. Lance **NDS Forwarder Generator** depuis le Homebrew Launcher.
4. Choisis `sd:/roms/nds/<jeu>.nds`, puis confirme. Recommence pour chaque jeu (Diamant, Perle, Platine, Or HeartGold, Argent SoulSilver, Noire, Blanche, Noire 2, Blanche 2).
5. Ouvre `sd:/_nds/ntr_forwarder.ini` et vérifie `SAVE_LOCATION=0` (même valeur que TWiLight). Le forwarder ne lit **pas** les réglages de TWiLight.
6. Ne déplace et ne renomme plus ces ROM : le chemin est fixé dans chaque forwarder.

Limites : 40 forwarders au maximum ; `.nds` seulement (pas de GB ni de GBA). Les Pokémon 3DS ont déjà leur icône. Pour le GBA, open_agb_firm se lance par le menu de démarrage de Luma (START au démarrage), sans icône HOME possible de façon sûre.

Ensuite : un jeu lancé par son icône se synchronise exactement comme avant, en ouvrant ndssync et en appuyant sur START.
