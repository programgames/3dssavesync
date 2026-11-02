# config.ini de la 3DS écrit depuis le PC, adresse de la box en tête

Premier lancement : le PC écrit l'adresse du serveur et le jeton dans `3ds/3dssavesync/config.ini` de la carte SD de la 3DS (page des réglages, « Préparer la carte SD de la 3DS » ; `setup --texte`, étape « Carte SD de la 3DS »). La 3DS lit ce fichier au démarrage et se connecte sans rien taper.

- Seul `config.ini` est écrit. Une racine sans dossier `Nintendo 3DS` est refusée. L'ancien fichier est copié avant (`config.ini.avant-3dssavesync-<date>`), ses autres lignes (`roms`, `saves`…) sont gardées ; écriture par fichier temporaire vérifié, renommage, relecture.
- Ancien dossier `3ds/nds-save-sync` seul présent : `config.ini` y est écrit. Le homebrew le reprend au démarrage avec `base.ini` (`migrer_dossier`) ; un nouveau dossier créé à côté empêcherait cette reprise et perdrait les Bases.
- « Télécharger config.ini » sert le même fichier, sans les autres lignes, pour une copie par FTP.

Adresse du serveur local pour la 3DS : la liste des IPv4 privées du PC, triée. D'abord une adresse qui ne finit pas en `.1` (le côté PC d'un réseau virtuel : WSL, Docker, VirtualBox, VMware), puis une adresse en `192.168.` (plage des box), puis l'adresse de la route par défaut. La route par défaut seule ne suffit pas : un VPN actif (NordVPN, WireGuard) la prend, et son adresse ne mène pas au PC depuis la 3DS. Les autres adresses restent affichées sous « Autres adresses ».

## Conséquences

- Une box en `10.x` avec un VPN actif sur le PC : l'adresse du VPN peut passer en tête. La bonne adresse est dans « Autres adresses ».
- Le PC change d'adresse (DHCP) : il faut préparer la carte à nouveau, ou une réservation DHCP dans la box.
