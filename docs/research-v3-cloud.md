# Recherche v3 — hébergement gratuit du Serveur (alternative à Docker)

Date : 2026-11-01. Sources : pages officielles de tarifs et de docs, plus un test TLS réel fait depuis le PC le 2026-11-01.
Légende : **NON VÉRIFIÉ** = pas de source primaire, source communautaire seulement, ou déduction sans test sur le matériel réel.

## Synthèse

- **Recommandation principale : Cloudflare Workers (plan Free) + un Durable Object SQLite.** Le stockage persiste, le plan Free ne facture jamais (il renvoie une erreur au-delà des quotas), et un Durable Object est mono-thread : le `412` se fait sans course. Le bouton « Deploy to Cloudflare » crée le Durable Object tout seul. Il faut réécrire le Serveur en TypeScript (environ 150 lignes).
- **Repli : Deno Deploy + Deno KV** (réécriture TypeScript proche, `atomic().check()` pour le `412`, certificat Let's Encrypt qui remonte à ISRG Root X1). Pour garder le Python **presque inchangé** : PythonAnywhere (gratuit, sans carte, disque persistant), mais l'utilisateur doit se reconnecter **chaque mois**.
- **Risque n° 1 (TLS) : la racine du domaine par défaut change d'un hébergeur à l'autre.** `*.workers.dev` sert soit Google Trust Services, soit Let's Encrypt, selon le compte. `vercel.app` et `run.app` servent GTS. `netlify.app` sert DigiCert. Le homebrew 3DS embarque **une seule** racine (ISRG Root X1). Il doit donc embarquer un **paquet** de racines (`CURLOPT_CAINFO_BLOB` accepte plusieurs PEM).
- **Risque n° 2 (carte bancaire) :** R2, Google Cloud, Oracle, Fly.io et Koyeb (au-delà du strict minimum) demandent une carte. Durable Objects et D1 passent par le plan Workers Free. D'après la communauté, ce plan ne demande pas de carte (NON VÉRIFIÉ par une page officielle).
- **Écartés :** Render (pas de disque persistant en Free), Koyeb (pas de volume sur l'instance Free), Hugging Face Spaces (Docker Spaces payants), Fly.io (pas de free tier), Vercel et Netlify (corps de requête limité, racine non ISRG, réécriture quand même).

## Contrat HTTP à reproduire (lu dans `server/server.py` et `server/test_server.py`)

- Auth `Authorization: Bearer <jeton>` comparée en temps constant ; `401` sinon.
- `GET /saves` : liste de la Version courante de chaque Jeu (`code`, `version`, `hash`, `taille`, `date`, `client`, `age`).
- `GET /saves/<clé>` : corps de la Version courante, `ETag: "<sha256>"`.
- `GET /saves/<clé>/history` et `GET /saves/<clé>/history/<version>`.
- `PUT /saves/<clé>` : `If-Match: "<sha256>"` ou `If-None-Match: *` obligatoire (`428` sans précondition). Précondition fausse : `412` avec l'`ETag` courant. Corps identique à la Version courante : `200` sans nouvelle Version. Taille `1..64 Mio`, corps tronqué : `400`.
- Historique en ajout seul, numéro `<seq>` croissant. Un verrou global sérialise la séquence lecture-comparaison-écriture.

Conséquence pour un hébergeur : il faut (1) un stockage persistant, (2) une opération de comparaison-écriture atomique, ou un point de sérialisation unique, (3) un corps de requête accepté jusqu'à 64 Mio (512 Kio suffisent pour l'usage courant).

## 1. Comparatif des hébergeurs gratuits

### Tableau

| Hébergeur | Stockage persistant gratuit | Carte bancaire | Veille / démarrage à froid | Python actuel tel quel ? | Racine TLS du domaine par défaut (test 2026-11-01) |
|---|---|---|---|---|---|
| Cloudflare Workers + Durable Objects (SQLite) | Oui, 5 Go au total | Non selon la communauté (NON VÉRIFIÉ) | Pas de veille ; isolates | Non : TypeScript (ou Python Workers, autre modèle) | `*.workers.dev` : GTS Root R4 **ou** ISRG Root X1, selon le compte |
| Cloudflare Workers + D1 | Oui, 5 Go (500 Mo par base) | idem | idem | Non | idem |
| Cloudflare Workers + R2 | Oui, 10 Go-mois | **Oui** (communauté) | idem | Non | idem |
| Cloudflare Workers + KV | Oui, 1 Go | idem Workers | idem | Non | idem — **mais pas de CAS : à écarter** |
| Deno Deploy + Deno KV | Oui, 1 Gio KV | NON VÉRIFIÉ | Isolates | Non : TypeScript | `*.deno.net` : ISRG Root X1 (via X2) |
| Vercel Hobby + Blob | Oui, 1 Go | Non (Hobby) — NON VÉRIFIÉ | Fonctions à froid | Runtime Python, mais disque éphémère : réécriture du stockage | `*.vercel.app` : GTS Root R1 |
| Netlify Free + Blobs | Oui (quota non publié) | NON VÉRIFIÉ | Fonctions à froid | Non : TypeScript | `*.netlify.app` : DigiCert Global Root G2 |
| Fly.io | Volumes payants | **Oui** après l'essai | — | Oui (Docker) | `*.fly.dev` : ISRG Root X1 (via X2) |
| Render Free | **Non** | Non | Veille après 15 min, ~1 min de réveil | Oui, mais données perdues | `*.onrender.com` : ISRG Root X1 |
| Railway Free | Volume 0,5 Go | NON VÉRIFIÉ | Option de veille | Oui (Docker) | `*.up.railway.app` : ISRG Root X1 (via X2) |
| Koyeb Free | **Non** (pas de volume sur `free`) | Carte demandée (empreinte 29 $) | — | Oui, mais données perdues | `*.koyeb.app` : ISRG Root X1 (via X2) |
| Google Cloud Run + Cloud Storage | Oui, 5 Go-mois (régions US) | **Oui** (compte de facturation) | Démarrage à froid | Oui avec montage GCS FUSE, mais sans verrou fiable | `*.a.run.app` : GTS Root R1 |
| Oracle Cloud Always Free (VM) | Oui, 200 Go de bloc | **Oui** | VM permanente ; récupération si inactive | Oui, inchangé | Au choix (Let's Encrypt → ISRG Root X1) |
| Hugging Face Spaces | **Non** (disque éphémère ; Buckets payants) | — | Veille | Docker Spaces **payants** | `*.hf.space` : Amazon Root CA 1 |
| PythonAnywhere Beginner | Oui, 512 Mio | Non indiqué | Expiration du site après 1 mois sans clic | Presque : adaptateur WSGI | `*.pythonanywhere.com` : ISRG Root X1 (via X2) |

### Détail par hébergeur

**Cloudflare Workers (plan Free)**
- 100 000 requêtes par jour, 10 ms de CPU par invocation. Source : https://developers.cloudflare.com/workers/platform/pricing/
- Corps de requête jusqu'à 100 Mo sur un plan Cloudflare Free, 128 Mo de mémoire par isolate. Source : https://developers.cloudflare.com/workers/platform/limits/
- Le sous-domaine `*.workers.dev` vise les projets personnels ; Cloudflare conseille un domaine personnalisé pour la production. Source : https://developers.cloudflare.com/workers/configuration/routing/workers-dev/
- Domaine personnalisé : il faut une zone sur Cloudflare (donc un nom de domaine, payant chez un registraire). Les certificats Universal SSL du plan Free sont en ECDSA, et Cloudflare choisit l'autorité (Let's Encrypt, Google Trust Services ou SSL.com). Source : https://developers.cloudflare.com/ssl/reference/certificate-authorities/
- Python Workers : le modèle est une classe `WorkerEntrypoint` avec `async def fetch`. `http.server` ne sert à rien. Le code est donc à réécrire, même en Python. Source : https://developers.cloudflare.com/workers/languages/python/
- Carte bancaire pour le plan Workers Free : la page officielle ne le dit pas. Des réponses de la communauté indiquent que les plans Free ne demandent pas de carte (NON VÉRIFIÉ). Source communautaire : https://community.cloudflare.com/t/free-credit-card-required/399917

**Durable Objects (SQLite) sur Workers Free**
- 100 000 requêtes par jour, 13 000 Go-s par jour, 5 millions de lignes lues et 100 000 lignes écrites par jour, 5 Go stockés au total. Seuls les Durable Objects SQLite existent en Free. Au-delà d'un quota, l'opération échoue avec une erreur : pas de facture. Source : https://developers.cloudflare.com/durable-objects/platform/pricing/
- Ligne, BLOB ou chaîne : 2 Mo maximum. Requête SQL : 100 Ko maximum. CPU par requête : 30 s par défaut. Source : https://developers.cloudflare.com/durable-objects/platform/limits/
- Conséquence : une Sauvegarde de plus de 2 Mo doit être découpée en morceaux de 2 Mo. Le cas courant (8 Kio – 512 Kio) tient dans une seule ligne.

**D1 sur Workers Free**
- 5 millions de lignes lues et 100 000 lignes écrites par jour, 5 Go au total, 10 bases. Source : https://developers.cloudflare.com/workers/platform/pricing/
- 500 Mo par base, ligne ou BLOB de 2 000 000 octets maximum, 50 requêtes par invocation. Source : https://developers.cloudflare.com/d1/platform/limits/

**R2**
- 10 Go-mois, 1 million d'opérations classe A et 10 millions classe B par mois, sortie gratuite. Source : https://developers.cloudflare.com/r2/pricing/
- 5 Gio par PUT simple, 1 écriture par seconde sur une même clé (sinon `429`). Source : https://developers.cloudflare.com/r2/platform/limits/
- Il faut « ajouter un abonnement R2 » par un passage en caisse. Source : https://developers.cloudflare.com/r2/get-started/
- Ce passage en caisse demande un moyen de paiement, même pour le free tier (source communautaire, NON VÉRIFIÉ officiellement) : https://community.cloudflare.com/t/why-using-r2-free-tier-involves-giving-card-info/945179 et https://community.cloudflare.com/t/question-regarding-5-usd-charge-for-r2-storage-activation/900480

**Workers KV**
- 1 Go, 1 000 écritures par jour. Source : https://developers.cloudflare.com/workers/platform/pricing/
- Cohérence à terme : une modification peut mettre 60 s ou plus pour être visible ailleurs. KV « n'est pas idéal » pour les opérations atomiques. Source : https://developers.cloudflare.com/kv/concepts/how-kv-works/
- Verdict : **à écarter**. Sans comparaison-écriture, deux Envois simultanés peuvent écraser une Version sans `412`.

**Deno Deploy (nouvelle plateforme) + Deno KV**
- Free : 1 million de requêtes par mois, 20 Gio de sortie, 1 Gio de KV, 1 million d'unités de lecture et 500 000 d'écriture KV par mois, 5 domaines personnalisés. La page ne dit rien sur la carte bancaire (NON VÉRIFIÉ). Source : https://deno.com/deploy/pricing
- Deploy Classic (dash.deno.com) ferme le 20 juillet 2026 ; il faut viser la nouvelle plateforme (console.deno.com). Source : https://docs.deno.com/deploy/classic/
- KV se crée dans « Databases » puis s'assigne à l'app. Les écritures validées vont dans un quorum de centres de données en Virginie du Nord. Source : https://docs.deno.com/deploy/reference/deno_kv/
- Valeur de 64 Kio maximum, opération atomique de 800 Kio maximum. Source : https://docs.deno.com/deploy/kv/manual/transactions/
- Conséquence : une Sauvegarde de 512 Kio fait 8 morceaux. Une de 64 Mio fait 1 024 morceaux, écrits en plusieurs opérations.

**Vercel (Hobby) + Blob**
- Blob Hobby : 1 Go, 10 000 opérations simples et 2 000 opérations avancées par mois (envoi, copie, **liste**), 10 Go de transfert. Au-delà, Blob est bloqué 30 jours, sans facture. Source : https://vercel.com/docs/vercel-blob/usage-and-pricing
- Corps de requête ou de réponse d'une fonction : **4,5 Mo maximum** (`413`). Source : https://vercel.com/docs/functions/limitations
- Conséquence : les grosses mémoires DS (jusqu'à 64 Mio) ne passent pas par une fonction.

**Netlify (Free) + Blobs**
- 300 crédits par mois, limite stricte, jamais de facture. Requêtes : 2 crédits pour 10 000 ; bande passante : 20 crédits par Go. Source : https://docs.netlify.com/manage/accounts-and-billing/billing/billing-for-credit-based-plans/credit-based-pricing-plans/
- Blobs : cohérence à terme par défaut (60 s), cohérence forte en option, objet de 5 Go maximum. Source : https://docs.netlify.com/build/data-and-storage/netlify-blobs/
- Limite de corps des fonctions : non trouvée dans la doc consultée (NON VÉRIFIÉ ; probablement la limite AWS Lambda de 6 Mo).

**Fly.io**
- « New organizations don't have a free tier » ; essai de 2 h de machine ou 7 jours ; ensuite une carte est obligatoire. Volumes : 0,15 $/Go-mois. Source : https://docs.fly.io/about/pricing

**Render (Free)**
- Veille après 15 min sans trafic, réveil d'environ 1 min, 750 h par mois. Pas de disque persistant en Free ; le système de fichiers se vide à chaque redéploiement, redémarrage ou veille. Postgres Free expire après 30 jours. Source : https://render.com/docs/free
- Verdict : **à écarter** pour un historique qui ne doit jamais rien perdre.

**Railway**
- Essai : 5 $ une fois, 30 jours. Ensuite plan Free : 1 $ de crédit par mois. Source : https://docs.railway.com/reference/pricing/free-trial
- Free : 0,5 Go de RAM, 0,5 Go de volume par service. Après la fin d'un plan Free ou Trial, Railway supprime les données 30 jours plus tard. Prix : RAM 10 $/Go-mois, volume 0,15 $/Go-mois. Source : https://docs.railway.com/reference/pricing/plans
- Sans vérification du compte, Railway place l'utilisateur en « Limited Trial » (réseau sortant restreint). Source : https://docs.railway.com/reference/pricing/free-trial
- Un serveur Python au repos tient peut-être dans 1 $ par mois (NON VÉRIFIÉ : calcul non mesuré). La politique sur la carte bancaire du plan Free n'est pas écrite (NON VÉRIFIÉ).

**Koyeb**
- Une instance `free` : 512 Mo de RAM, 0,1 vCPU, 2 Go de SSD. Koyeb demande une carte « pour prévenir la fraude », avec une empreinte de 29 $. Source : https://www.koyeb.com/docs/faqs/pricing
- Les volumes ne s'attachent pas aux instances `free` ou `eco-*`. Source : https://www.koyeb.com/docs/reference/volumes
- Verdict : **à écarter** (pas de persistance gratuite).

**Google Cloud Run + Cloud Storage**
- « A Google Cloud billing account is required » pour le Free Tier (donc une carte). Cloud Run : 2 millions de requêtes par mois. Cloud Storage : 5 Go-mois dans `us-east1`, `us-west1` ou `us-central1` seulement, 5 000 opérations classe A par mois. Source : https://docs.cloud.google.com/free/docs/free-cloud-features
- Montage Cloud Storage FUSE dans Cloud Run : pas de verrou de fichier, « the last write wins », pas entièrement POSIX. Source : https://docs.cloud.google.com/run/docs/configuring/services/cloud-storage-volume-mounts
- Conséquence : le Python actuel tourne, mais son `threading.Lock` ne protège qu'une instance. Il faut `max-instances=1`, ou réécrire le stockage avec les préconditions de génération (voir §2).

**Oracle Cloud Always Free**
- 2 VM AMD micro (1 Go de RAM) ou 4 OCPU / 24 Go Arm au total, 200 Go de bloc, 10 To de sortie par mois. Oracle peut récupérer une instance inactive (CPU au 95e centile < 20 % sur 7 jours). Source : https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm
- Carte de crédit obligatoire à l'inscription (vérification d'identité, pas de débit). Source : https://www.oracle.com/cloud/free/faq/
- Le Python tourne inchangé (Docker ou systemd), mais l'installation n'est pas pour un non-expert : VM, pare-feu, nom de domaine, certificat.

**Hugging Face Spaces**
- « Gradio and Docker Spaces run on compute and require a paid plan to create » ; seuls les Spaces statiques sont gratuits. Disque non persistant. Source : https://huggingface.co/docs/hub/spaces-overview
- Le disque éphémère se vide au redémarrage ; la persistance passe par des Storage Buckets. Source : https://huggingface.co/docs/hub/spaces-storage
- Buckets facturés au To, sans quota gratuit affiché ; PRO à 9 $ par mois. Source : https://huggingface.co/pricing
- Verdict : **à écarter**.

**PythonAnywhere (Beginner, gratuit)**
- 1 application web sur `<utilisateur>.pythonanywhere.com`, pas de domaine personnalisé, 512 Mio de disque, HTTPS inclus. Source : https://www.pythonanywhere.com/pricing/
- « 1 web app with 1 web worker and 1 month expiry » (comptes créés depuis le 2026-01-15). Source : https://help.pythonanywhere.com/pages/FreeAccountsFeatures/
- À l'expiration, les fichiers restent, mais le site s'arrête jusqu'à un clic dans l'onglet Web. Source : https://blog.pythonanywhere.com/221/
- Le Python doit passer de `http.server` à une fonction WSGI. La logique ne change pas. Avec un seul worker, les requêtes sont déjà en série (NON VÉRIFIÉ : modèle de worker non documenté sur la page lue).
- 512 Mio limitent l'historique en ajout seul : quelques centaines de fichiers de 512 Kio font déjà 150 Mio.

## 2. Écriture conditionnelle sans course (meilleurs candidats)

### Cloudflare Durable Object (SQLite) — recommandé

- Les Durable Objects sont « single-threaded and cooperatively multi-tasked ». Leur stockage est « transactional, and strongly consistent ». Source : https://developers.cloudflare.com/durable-objects/concepts/what-are-durable-objects/
- `sql.exec()` termine de façon synchrone. `transactionSync()` exécute un rappel synchrone dans une transaction et l'annule si le rappel lève une exception. Source : https://developers.cloudflare.com/durable-objects/api/sqlite-storage-api/
- Piège : un `await` sur une E/S hors stockage (`fetch()`, R2) laisse d'autres requêtes s'intercaler. Source : https://developers.cloudflare.com/durable-objects/best-practices/rules-of-durable-objects/
- Conception sûre :
  1. Le Worker vérifie le jeton, lit le corps, calcule le SHA-256 (`crypto.subtle.digest`, donc un `await`), **avant** la section critique. Le Python actuel fait pareil : il hache hors du verrou.
  2. Le Worker transmet à **un seul** Durable Object nommé (par exemple `"global"`). Il reproduit ainsi le verrou global.
  3. Dans le Durable Object, un seul bloc `transactionSync()` sans `await` lit la Version courante, compare `If-Match` / `If-None-Match`, puis insère la nouvelle Version (et ses morceaux de 2 Mo). Aucune autre requête ne peut s'intercaler dans ce bloc.
- Limite : 5 Go au total et 100 000 lignes écrites par jour. Une Sauvegarde de 64 Mio fait 32 lignes. L'usage prévu (quelques Envois par jour) reste très loin du quota.
- Risque : la mémoire de 128 Mo par isolate avec un corps de 64 Mio en mémoire (le corps, sa copie vers le Durable Object, les morceaux). NON VÉRIFIÉ : à mesurer avant de garder la limite de 64 Mio.

### Cloudflare D1 — variante

- « Batched statements are SQL transactions » ; un échec annule tout le lot. Source : https://developers.cloudflare.com/d1/worker-api/d1-database/
- Une seule instruction `INSERT ... SELECT ... WHERE (hash courant) = ?` est atomique, car SQLite n'a qu'un écrivain. Ensuite, `changes = 0` donne le `412`. NON VÉRIFIÉ : déduction du modèle SQLite, pas d'une phrase de la doc D1.

### Cloudflare R2 — si l'utilisateur accepte une carte

- `put(..., { onlyIf })` accepte `etagMatches` / `etagDoesNotMatch` ou des en-têtes conditionnels (« All conditional headers aside from If-Range are supported »). Précondition fausse : `put()` renvoie `null` et n'écrit rien. Les écritures R2 sont fortement cohérentes. Source : https://developers.cloudflare.com/r2/api/workers/workers-api-reference/
- Les en-têtes conditionnels de PutObject sont « evaluated atomically against the committed bucket state ». Avec plusieurs `If-None-Match: *` simultanés, « at most one write succeeds and the others fail with `412 PreconditionFailed` ». Source : https://developers.cloudflare.com/r2/api/s3/api/
- Conception sûre : une Version = l'objet `<clé>/<seq sur 6 chiffres>`, écrit avec `If-None-Match: *` ; métadonnées (`hash`, `date`, `client`) en `customMetadata` (8 192 octets maximum, source : https://developers.cloudflare.com/r2/platform/limits/). Deux Envois concurrents sur la même base visent le même `<seq>` : un seul réussit, l'autre reçoit `412`. La Version courante est le plus grand `<seq>` d'une liste par préfixe.
- Option `sha256` de `put()` : R2 vérifie l'intégrité du corps reçu. Le Client connaît déjà le hash, donc le Worker n'a pas besoin de hacher 64 Mio dans ses 10 ms de CPU. Source : https://developers.cloudflare.com/r2/api/workers/workers-api-reference/

### Deno KV — repli

- `atomic().check({ key, versionstamp })` puis `set()` : le commit ne passe que si les versionstamps sont inchangés ; sinon `commit()` renvoie `ok: false` sans exception. Maximum 64 Kio par valeur, 800 Kio par opération atomique. Source : https://docs.deno.com/deploy/kv/manual/transactions/
- Conception sûre : écrire d'abord les morceaux sous une clé unique de Version (hors transaction), puis un seul `atomic()` qui vérifie le pointeur « courant » et le remplace. Si le check échoue, renvoyer `412` ; les morceaux orphelins ne gênent pas (l'historique ne lit que les Versions référencées).

### Google Cloud Storage — pour mémoire

- `ifGenerationMatch` (`x-goog-if-generation-match`) : la requête ne passe que si la génération correspond, sinon `412 Precondition Failed`. La valeur `0` signifie « seulement si l'objet n'existe pas ». La page ne dit pas « atomique » (NON VÉRIFIÉ). Source : https://docs.cloud.google.com/storage/docs/request-preconditions

### Vercel Blob et Netlify Blobs — possible, mais écartés pour d'autres raisons

- Vercel : `put()` avec `ifMatch` lève `BlobPreconditionFailedError` si l'ETag ne correspond pas. Sans `allowOverwrite`, réutiliser un chemin lève une erreur, ce qui donne un « créer seulement ». Source : https://vercel.com/docs/vercel-blob/using-blob-sdk
- Netlify : `set()` avec `onlyIfNew` ou `onlyIfMatch` renvoie `modified: false` en cas d'échec. Source : https://docs.netlify.com/build/data-and-storage/netlify-blobs/

## 3. Compatibilité TLS avec la 3DS

### Ce que le Client 3DS sait faire

- Le paquet devkitPro `3ds-mbedtls` est mbedTLS **2.28.8**. Il ne change ni les courbes, ni les versions TLS, ni les suites ; il applique un patch dont le contenu n'a pas été lu (NON VÉRIFIÉ). Source : https://github.com/devkitPro/pacman-packages/blob/master/3ds/mbedtls/PKGBUILD
- La configuration par défaut de mbedTLS 2.28.8 active TLS 1.2, ECDHE-RSA, ECDHE-ECDSA, secp256r1, secp384r1, Curve25519, AES-GCM, ChaCha20-Poly1305 et SHA-512/384. TLS 1.3 reste désactivé. Source : https://github.com/Mbed-TLS/mbedtls/blob/v2.28.8/include/mbedtls/config.h
- `CURLOPT_CAINFO_BLOB` accepte « one or more certificates » en PEM. Le backend mbedTLS le prend en charge depuis curl 7.81.0. Source : https://curl.se/libcurl/c/CURLOPT_CAINFO_BLOB.html

### Test réel (2026-11-01, OpenSSL 3.2.1 sur le PC)

Commande : `openssl s_client -connect <hôte>:443 -servername <hôte> -tls1_2 -cipher 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:…:ECDHE-RSA-CHACHA20-POLY1305' -showcerts`.
Tous les hôtes ci-dessous ont accepté **TLS 1.2 avec une suite ECDHE** (donc pas de TLS 1.3 seul).

| Hôte testé | Suite négociée | Chaîne servie (feuille → haut) | Valide avec ISRG Root X1 seule ? |
|---|---|---|---|
| `dhen.maes84671.workers.dev`, `www.sitefiles.workers.dev`, `workers.dev` | ECDHE-ECDSA | WE1 → GTS Root R4 → (croisé) GlobalSign Root CA | **Non** |
| `x.bdragon.workers.dev` | ECDHE-ECDSA | YE1 → Root YE → ISRG Root X2 → (croisé) ISRG Root X1 | Oui |
| `fresh.deno.dev`, `deno.net` | ECDHE-ECDSA | YE1/YE2 → Root YE → X2 → X1 | Oui |
| `*.vercel.app` | ECDHE-RSA | WR1 → GTS Root R1 → (croisé) GlobalSign Root CA | **Non** |
| `*.netlify.app` | ECDHE-ECDSA | DigiCert Global G2 TLS RSA SHA256 2020 CA1 → DigiCert Global Root G2 | **Non** |
| `debug.fly.dev` | ECDHE-ECDSA | YE1 → Root YE → X2 → X1 | Oui |
| `*.onrender.com` | ECDHE-RSA | YR1 → Root YR → X1 | Oui |
| `*.up.railway.app` | ECDHE-ECDSA | YE2 → Root YE → X2 → X1 | Oui |
| `*.koyeb.app` (app réelle) | ECDHE-ECDSA | YE2 → Root YE → X2 → X1 | Oui |
| `*.a.run.app` | ECDHE-ECDSA | WR2 → GTS Root R1 → (croisé) GlobalSign Root CA | **Non** |
| `gradio-hello-world.hf.space` | ECDHE-RSA | Amazon RSA 2048 M04 → Amazon Root CA 1 → Starfield | **Non** |
| `*.pythonanywhere.com` | ECDHE-ECDSA | YE2 → Root YE → X2 → X1 | Oui |
| serveur auto-hébergé (Traefik + Let's Encrypt) | ECDHE-RSA | YR2 → Root YR → X1 | Oui |

- Hiérarchie Let's Encrypt : chaîne par défaut ECDSA `EE ← YE ← Root YE ← X2 ← X1` ; RSA `EE ← YR ← Root YR ← X1`. Root YE et Root YR ne sont pas encore dans les magasins de racines. Source : https://letsencrypt.org/certificates/
- Cloudflare Universal SSL : Cloudflare choisit l'autorité (Let's Encrypt, Google Trust Services, SSL.com) ; le plan Free reçoit des certificats ECDSA. Source : https://developers.cloudflare.com/ssl/reference/certificate-authorities/ — le test confirme que deux comptes `workers.dev` différents ont deux autorités différentes.
- Les certificats croisés GTS Root R1/R4 par GlobalSign Root CA expirent le **2028-01-28** (lu dans les certificats servis, test du 2026-11-01).

### Conséquences pour le Client 3DS

- Les chaînes ECDSA Let's Encrypt passent par ISRG Root X2 (P-384, signée en RSA par X1). mbedTLS 2.28.8 a secp384r1 et SHA-384 par défaut. Elles devraient donc passer (NON VÉRIFIÉ sur une vraie 3DS ; le serveur actuel sert une chaîne RSA).
- Pour Cloudflare, le homebrew doit embarquer un paquet PEM : ISRG Root X1 + GTS Root R1 + GTS Root R4. Il faut des racines auto-signées, pas GlobalSign Root CA, qui expire en 2028. NON VÉRIFIÉ : que mbedTLS accepte GTS Root R4 auto-signée comme ancre, alors que le serveur envoie la version croisée par GlobalSign. Test à faire sur la 3DS avant de choisir Cloudflare.
- Autre solution : un domaine personnalisé derrière Cloudflare garde le même problème, car Cloudflare choisit l'autorité sur Universal SSL. Choisir l'autorité demande Advanced Certificate Manager (payant) (NON VÉRIFIÉ : prix non lus).
- Client PowerShell 5.1 : il utilise le magasin de racines Windows, qui contient GTS, DigiCert, ISRG et Amazon. Aucun hébergeur ne pose de problème de racine côté PC (NON VÉRIFIÉ : pas de source lue ; le client actuel marche déjà en TLS 1.2 avec le serveur personnel de l'auteur).

## 4. Déploiement facile pour un non-expert

- **Cloudflare** : bouton `[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=<dépôt git public>)`. Cloudflare lit le fichier Wrangler du dépôt, crée les ressources déclarées (KV, D1, R2, **Durable Objects**), puis clone le dépôt dans le compte GitHub/GitLab de l'utilisateur. Les secrets se déclarent dans `.dev.vars.example`. La page ne dit pas clairement que Cloudflare demande leur valeur (NON VÉRIFIÉ). Source : https://developers.cloudflare.com/workers/platform/deploy-buttons/
  - Étapes utilisateur : (1) créer un compte Cloudflare, (2) cliquer le bouton, (3) saisir le jeton `NDSSYNC_TOKEN` comme secret, (4) copier l'URL `https://ndssync.<sous-domaine>.workers.dev` dans les deux Clients.
- **Deno Deploy** : créer une organisation sur console.deno.com, créer une app depuis GitHub, provisionner une base Deno KV dans « Databases » et l'assigner. Une page « Deploy Button » existe dans la référence (contenu non lu, NON VÉRIFIÉ). Sources : https://docs.deno.com/deploy/ et https://docs.deno.com/deploy/reference/deno_kv/
- **PythonAnywhere** : envoyer `server.py` et un fichier WSGI depuis l'onglet Files, configurer l'app web dans l'onglet Web, puis cliquer une fois par mois sur le bouton de prolongation. Pas de bouton en un clic (NON VÉRIFIÉ : aucune doc de modèle lue).
- **Railway** : modèles « Deploy on Railway » à partir d'un Dockerfile (NON VÉRIFIÉ : page des modèles non lue).
- **Oracle** : aucune voie en un clic adaptée à un non-expert.

## Recommandation

1. **Principal : Cloudflare Workers Free + un Durable Object SQLite**, réécrit en TypeScript.
   - Gratuit et plafonné : erreur au-delà des quotas, jamais de facture. Pas de carte selon la communauté.
   - Persistance forte, pas de veille, bouton de déploiement qui crée le Durable Object.
   - `412` sans course grâce à `transactionSync()` dans un Durable Object unique.
   - Condition : le homebrew embarque ISRG Root X1 + GTS Root R1 + GTS Root R4, testé sur la 3DS.
2. **Repli : Deno Deploy + Deno KV**, avec la même réécriture TypeScript (API `Request`/`Response` identique) et `atomic().check()`. Le certificat `*.deno.net` remonte déjà à ISRG Root X1 : la racine actuelle suffit.
3. **Sans réécriture**, pour qui accepte un clic par mois : PythonAnywhere avec un adaptateur WSGI. Pour un expert : Oracle Always Free avec le Docker actuel.

## Principaux risques

- **Racine TLS** : `workers.dev` peut servir GTS. Sans paquet de racines, la 3DS refuse la connexion. Let's Encrypt peut aussi changer sa chaîne (Root YE/YR pas encore dans les magasins).
- **Conditions gratuites changeantes** : Fly.io, Koyeb, Hugging Face et PythonAnywhere ont réduit leur offre gratuite récemment ; Deno a fermé Deploy Classic. Une offre gratuite actuelle peut disparaître.
- **Grosses Sauvegardes** : 64 Mio dépassent Vercel (4,5 Mo), forcent un découpage en 2 Mo (Durable Objects, D1) ou 64 Kio (Deno KV), et chargent la mémoire de 128 Mo d'un Worker.
- **Carte bancaire** : R2, Google, Oracle, Fly.io et Koyeb en demandent une. L'absence de carte pour Workers Free et Deno Deploy Free n'est confirmée par aucune page officielle (NON VÉRIFIÉ).
- **Réécriture** : le contrat (`428`, `412` + ETag, corps identique, clés ADR 0007, `age`) doit être retesté. Les tests `server/test_server.py` peuvent viser une URL distante si l'on rend `self.url` configurable.
- **Quota de stockage et ajout seul** : 5 Go (Cloudflare), 1 Gio (Deno KV), 512 Mio (PythonAnywhere). L'historique ne supprime jamais rien : il faudra surveiller sa taille.
