# Synchronisation des sauvegardes DS

Garder la même partie d'un jeu DS entre une 3DS (TWiLight Menu++) et un PC (melonDS), via un serveur personnel qui ne perd jamais rien.

## Jeux

**Jeu** :
Un titre DS, GB, GBC, GBA ou 3DS, identifié par sa Clé. Deux ROM avec la même Clé sont le même Jeu.
_Éviter_ : titre, ROM, cartouche

**Système** :
La console d'origine d'un Jeu : DS, GB (GB et GBC confondus), GBA ou 3DS. Il fixe la forme de la Clé et l'emplacement des Sauvegardes.
_Éviter_ : plateforme, console, support

**Clé** :
L'identifiant d'un Jeu sur le Serveur, lu dans l'en-tête de la ROM : le Code jeu pour la DS, `GBA-` + code jeu, `GB-` + titre + somme de contrôle, `3DS-` + title ID (ADR 0007).
_Éviter_ : id, identifiant, game ID

**ROM** :
Le fichier qui contient un Jeu sur un Client (`.nds` sur la 3DS, `.zip` contenant un `.nds` sur le PC). Son nom de fichier n'a aucun rôle dans l'identité du Jeu.
_Éviter_ : image, dump, cartouche

**Code jeu** :
Les 4 caractères ASCII de l'en-tête d'une ROM DS (offset `0x0C`) ou GBA (offset `0xAC`), région incluse (ex. `IRBF`). Deux régions d'un même titre ont deux Codes jeu, donc sont deux Jeux.
_Éviter_ : game ID, title ID, serial, gamecode

**Doublon** :
Situation où un Client a plusieurs ROM avec la même Clé. Un Jeu en Doublon n'est pas synchronisé.
_Éviter_ : copie, conflit

## Sauvegardes

**Sauvegarde** :
Le contenu brut de la mémoire de sauvegarde d'un Jeu, octet pour octet, sans en-tête ni pied de page.
_Éviter_ : save, partie, fichier `.sav`, backup

**Sauvegarde locale** :
La Sauvegarde d'un Jeu telle qu'elle existe sur un Client à un instant donné.
_Éviter_ : fichier local, save locale

**Hash** :
L'empreinte SHA-256 du contenu d'une Sauvegarde. Deux Sauvegardes sont identiques si et seulement si leurs Hash sont égaux.
_Éviter_ : checksum, digest, empreinte, ETag

**Copie de sécurité** :
Une copie horodatée d'une Sauvegarde locale, faite par le Client juste avant de l'écraser.
_Éviter_ : backup, `.bak`, sauvegarde (ambigu en français)

**SRAM** :
La partie « jeu » d'une Sauvegarde GB ou GBA : le fichier sans son Trailer d'horloge. C'est elle qui est hachée, comparée et synchronisée.
_Éviter_ : données, contenu

**Trailer d'horloge** :
Les octets qu'un émulateur ajoute après la SRAM pour l'horloge temps réel (mGBA : 48 o en GB, 16 o en GBA). Il reste sur la machine qui l'a écrit.
_Éviter_ : RTC, footer, footer RTC

**Sauvegarde vierge** :
Une Sauvegarde DS, GB ou GBA dont la SRAM ne contient que des 0x00 ou que des 0xFF : aucune partie n'y est encore écrite. Elle compte comme absente.
_Éviter_ : sauvegarde vide, sauvegarde blanche

**Archive de sauvegarde** :
Le conteneur système d'une Sauvegarde 3DS native ; pour les Pokémon 3DS il contient un seul fichier, `main`. Azahar le range en dossier ordinaire.
_Éviter_ : savedata, conteneur

**Secure value** :
Le compteur anti-restauration d'un jeu 3DS. Il est supprimé après chaque écriture d'une Archive de sauvegarde sur la 3DS, sinon le jeu refuse la Sauvegarde.
_Éviter_ : valeur sécurisée, anti-rollback

**Slot TWiLight** :
Une Sauvegarde alternative de TWiLight (`.sav1` à `.sav9`). Hors du périmètre : seul le slot 0 est synchronisé.
_Éviter_ : emplacement, numéro de sauvegarde

## Serveur

**Version** :
Une Sauvegarde reçue par le Serveur pour un Jeu, avec sa date de réception et le Client qui l'a envoyée. Une Version ne change et ne disparaît jamais.
_Éviter_ : révision, snapshot, copie

**Version courante** :
La Version la plus récemment acceptée par le Serveur pour un Jeu.
_Éviter_ : dernière version, head, latest, version serveur

**Historique** :
L'ensemble des Versions d'un Jeu sur le Serveur, de la plus ancienne à la Version courante.
_Éviter_ : archive, backups, journal

**Restauration** :
Faire d'une ancienne Version le contenu de la Version courante, en l'envoyant comme une nouvelle Version. L'Historique ne recule jamais.
_Éviter_ : rollback, retour arrière

## Synchronisation

**Client** :
Une machine où l'on joue et qui synchronise ses Sauvegardes locales : la 3DS ou le PC.
_Éviter_ : appareil, device, console, poste

**Serveur** :
Le service qui garde l'Historique de chaque Jeu et arbitre les Envois concurrents.
_Éviter_ : cloud, backend, NAS

**Base** :
Le Hash de la dernière Version qu'un Client a synchronisée pour un Jeu (envoyée ou récupérée). Chaque Client a sa propre Base par Jeu.
_Éviter_ : dernier sync, ancêtre, référence, état précédent

**État** :
Le résultat de la comparaison entre Sauvegarde locale, Version courante et Base pour un Jeu : À jour, À envoyer, À récupérer ou Conflit.
_Éviter_ : action, statut, décision

**Conflit** :
L'État où la Sauvegarde locale et la Version courante ont toutes deux changé depuis la Base, ou n'ont pas la même taille. Seul l'utilisateur résout un Conflit.
_Éviter_ : collision, divergence, désynchronisation

**Envoi** :
Le Client transmet sa Sauvegarde locale ; si le Serveur l'accepte, elle devient une nouvelle Version courante.
_Éviter_ : push, upload, téléversement

**Récupération** :
Le Client remplace sa Sauvegarde locale par la Version courante, après une Copie de sécurité.
_Éviter_ : pull, download, téléchargement

**Lanceur** :
Le programme PC que LaunchBox appelle à la place de melonDS : Récupération, partie, puis Envoi.
_Éviter_ : wrapper, plugin, script de sync
