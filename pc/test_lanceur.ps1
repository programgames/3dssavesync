# Teste Invoke-Lanceur contre un vrai serveur local (server/server.py) et un dossier temporaire.
# melonDS, les boîtes de dialogue et la détection de processus sont remplacés par des doublures.
# Usage : powershell -NoProfile -File pc\test_lanceur.ps1
. "$PSScriptRoot\sync.ps1"

$script:echecs = 0
function Verifier($condition, $message) {
    if ($condition) { Write-Host "  ok    $message" } else { Write-Host "  ECHEC $message"; $script:echecs++ }
}

# --- Serveur local sur un port libre -------------------------------------------------------
$l = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0); $l.Start(); $port = $l.LocalEndpoint.Port; $l.Stop()
$tmp = Join-Path $env:TEMP "ndssync-test-$PID"
New-Item -ItemType Directory "$tmp\data", "$tmp\saves", "$tmp\roms", "$tmp\cfg" -Force | Out-Null
$env:NDSSYNC_TOKEN = 'jeton-test'; $env:NDSSYNC_DATA = "$tmp\data"; $env:NDSSYNC_PORT = "$port"
$serveur = Start-Process python -ArgumentList "`"$PSScriptRoot\..\server\server.py`"" -PassThru -WindowStyle Hidden
Start-Sleep -Milliseconds 800

$config = "$tmp\cfg\sync.config.json"
@{ url = "http://127.0.0.1:$port"; jeton = 'jeton-test'; melonds = 'inutilise.exe'; sauvegardes = "$tmp\saves"
   mgba = 'inutilise.exe'; sauvegardes_gb = "$tmp\gb"; sauvegardes_gba = "$tmp\gba"
   azahar = 'inutilise.exe'; azahar_sdmc = "$tmp\azahar"; copies_3ds = "$tmp\copies3ds" } |
    ConvertTo-Json | Set-Content $config
New-Item -ItemType Directory "$tmp\gb", "$tmp\gba", "$tmp\azahar" -Force | Out-Null

# Fausses ROM GB / GBA (en-tête seul) dans un .zip au nom No-Intro, comme D:\Roms\gb*.
Add-Type -AssemblyName System.IO.Compression.FileSystem
function Nouvelle-RomZip($nom, $ext, [byte[]]$octets) {
    $brut = "$tmp\roms\$nom.$ext"; [IO.File]::WriteAllBytes($brut, $octets)
    $zip = "$tmp\roms\$nom.zip"
    $a = [IO.Compression.ZipFile]::Open($zip, 'Create')
    [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($a, $brut, "$nom.$ext") | Out-Null
    $a.Dispose(); Remove-Item $brut; $zip
}
function Entete-GB($titre, $somme) {
    $o = New-Object byte[] 0x150; [Text.Encoding]::ASCII.GetBytes($titre).CopyTo($o, 0x134)
    $o[0x14E] = $somme -shr 8; $o[0x14F] = $somme -band 0xFF; $o
}
function Entete-GBA($code) { $o = New-Object byte[] 0xC0; [Text.Encoding]::ASCII.GetBytes($code).CopyTo($o, 0xAC); $o }
# Fausse ROM 3DS : NCSD, partition 0 à 0x200, program_id à NCCH + 0x118.
function Nouvelle-Rom3DS($nom, [UInt64]$titleId) {
    $o = New-Object byte[] 0x400
    [Text.Encoding]::ASCII.GetBytes('NCSD').CopyTo($o, 0x100)
    [BitConverter]::GetBytes([UInt32]1).CopyTo($o, 0x120)
    [BitConverter]::GetBytes($titleId).CopyTo($o, 0x200 + 0x118)
    $chemin = "$tmp\roms\$nom.3ds"; [IO.File]::WriteAllBytes($chemin, $o); $chemin
}
function Dossier-Azahar([UInt64]$titleId) {
    $z = '0' * 32; $h = '{0:x8}' -f [UInt32]($titleId -shr 32); $b = '{0:x8}' -f [UInt32]($titleId % 4294967296)  # 0xFFFFFFFF vaut -1 en PowerShell
    "$tmp\azahar\Nintendo 3DS\$z\$z\title\$h\$b\data\00000001"
}

# Une fausse ROM : 0x200 octets, Code jeu à 0x0C.
function Nouvelle-Rom($nom, $code) {
    $octets = New-Object byte[] 0x200
    [Text.Encoding]::ASCII.GetBytes($code).CopyTo($octets, 0x0C)
    $chemin = "$tmp\roms\$nom.nds"
    [IO.File]::WriteAllBytes($chemin, $octets)
    $chemin
}
function Sav($nom) { "$tmp\saves\$nom.sav" }
function Hash($octets) { Get-Hash $octets }

# Requêtes directes au serveur, pour jouer « l'autre Client ».
function Api($methode, $chemin, $octets, $entetes = @{}) {
    $c = New-Object Net.Http.HttpClient
    $req = New-Object Net.Http.HttpRequestMessage($methode, "http://127.0.0.1:$port$chemin")
    $req.Headers.TryAddWithoutValidation('Authorization', 'Bearer jeton-test') | Out-Null
    foreach ($k in $entetes.Keys) { $req.Headers.TryAddWithoutValidation($k, $entetes[$k]) | Out-Null }
    if ($octets) { $req.Content = New-Object Net.Http.ByteArrayContent(, $octets) }
    $r = $c.SendAsync($req).GetAwaiter().GetResult()
    @{ statut = [int]$r.StatusCode; octets = $r.Content.ReadAsByteArrayAsync().Result }
}
function Courante($code) { (Api Get "/saves/$code").octets }
function Historique($code) { $h = [Text.Encoding]::UTF8.GetString((Api Get "/saves/$code/history").octets) | ConvertFrom-Json; $h }

# Doublures : chaque scénario règle ces variables.
$script:reponses = @(); $script:questions = @(); $script:notifications = @(); $script:partie = $null; $script:joue = $false; $script:melonActif = $false
function Notifier($texte) { $script:notifications += $texte }
function Demander($texte, $boutons) {
    $script:questions += $texte
    $r, $script:reponses = $script:reponses
    if (-not $r) { throw "question inattendue : $texte" }
    $r
}
function Start-Jeu($cfg, $rom) { $script:joue = $true; if ($script:partie) { & $script:partie } }
function Test-EmulateurActif($exe) { $script:melonActif }
function Reinitialiser { $script:reponses = @(); $script:questions = @(); $script:notifications = @(); $script:partie = $null; $script:joue = $false; $script:melonActif = $false }

try {
    Write-Host "Premier lancement : la Sauvegarde locale part sur le serveur"
    Reinitialiser
    $rom = Nouvelle-Rom 'Pokemon Noire' 'IRBF'
    $v1 = [byte[]](1..200)
    [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), $v1)
    $code = Invoke-Lanceur $rom $config
    Verifier ($code -eq 0) "code de sortie 0"
    Verifier $script:joue "melonDS lancé"
    Verifier ((Hash (Courante 'IRBF')) -eq (Hash $v1)) "le serveur a la Sauvegarde du PC"
    Verifier ($script:questions.Count -eq 0) "aucune question"

    Write-Host "La partie modifie la Sauvegarde : Envoi après la partie"
    Reinitialiser
    $v2 = [byte[]](2..201)
    $script:partie = { [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), $v2) }
    Invoke-Lanceur $rom $config | Out-Null
    Verifier ((Hash (Courante 'IRBF')) -eq (Hash $v2)) "le serveur a la Sauvegarde jouée"
    Verifier (@(Historique 'IRBF').Count -eq 2) "l'Historique garde les 2 Versions"

    Write-Host "La 3DS a joué : Récupération avec Copie de sécurité"
    Reinitialiser
    $v3 = [byte[]](3..202)
    Verifier ((Api Put '/saves/IRBF' $v3 @{ 'If-Match' = "`"$(Hash $v2)`""; 'X-Client' = '3ds' }).statut -eq 200) "la 3DS envoie"
    Invoke-Lanceur $rom $config | Out-Null
    Verifier ((Hash ([IO.File]::ReadAllBytes((Sav 'Pokemon Noire')))) -eq (Hash $v3)) "le PC a la Sauvegarde de la 3DS"
    $copies = @(Get-ChildItem "$tmp\saves\ndssync-copies" -Filter '*.sav' -ErrorAction SilentlyContinue)
    Verifier ($copies.Count -eq 1 -and (Hash ([IO.File]::ReadAllBytes($copies[0].FullName))) -eq (Hash $v2)) "Copie de sécurité de l'ancienne Sauvegarde"
    Verifier ($script:questions.Count -eq 0) "aucune question"

    function Local-Hash { Hash ([IO.File]::ReadAllBytes((Sav 'Pokemon Noire'))) }
    function Envoi-3ds($octets, $avant) { (Api Put '/saves/IRBF' $octets @{ 'If-Match' = "`"$(Hash $avant)`""; 'X-Client' = '3ds' }).statut }

    Write-Host "Conflit avant la partie, Annuler : rien ne bouge, pas de partie"
    Reinitialiser
    $v4 = [byte[]](4..203); $v5 = [byte[]](5..204)
    [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), $v4)
    Envoi-3ds $v5 $v3 | Out-Null
    $script:reponses = @('Ne pas jouer')
    Invoke-Lanceur $rom $config | Out-Null
    Verifier ($script:questions.Count -eq 1 -and $script:questions[0] -match 'Conflit') "une question de Conflit"
    Verifier (-not $script:joue) "melonDS non lancé"
    Verifier ((Local-Hash) -eq (Hash $v4)) "le PC garde sa Sauvegarde"
    Verifier ((Hash (Courante 'IRBF')) -eq (Hash $v5)) "le serveur garde la sienne"

    Write-Host "Conflit avant la partie, Oui : le PC prend la Version du serveur"
    Reinitialiser
    $script:reponses = @('Prendre le serveur')
    Invoke-Lanceur $rom $config | Out-Null
    Verifier $script:joue "melonDS lancé"
    Verifier ((Local-Hash) -eq (Hash $v5)) "le PC a la Version du serveur"
    $copie = Get-ChildItem "$tmp\saves\ndssync-copies" -Filter '*.sav' | Where-Object { (Hash ([IO.File]::ReadAllBytes($_.FullName))) -eq (Hash $v4) }
    Verifier ([bool]$copie) "Copie de sécurité de la Sauvegarde du PC"

    Write-Host "Conflit avant la partie, Non : le PC garde la sienne et l'envoie"
    Reinitialiser
    $v6 = [byte[]](6..205); $v7 = [byte[]](7..206)
    [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), $v6)
    Envoi-3ds $v7 $v5 | Out-Null
    $script:reponses = @('Garder le PC')
    Invoke-Lanceur $rom $config | Out-Null
    Verifier $script:joue "melonDS lancé"
    Verifier ((Hash (Courante 'IRBF')) -eq (Hash $v6)) "le serveur a la Sauvegarde du PC"
    Verifier ((Historique 'IRBF').hash -contains (Hash $v7)) "la Version de la 3DS reste dans l'Historique"

    Write-Host "La 3DS envoie pendant la partie (412), Non : rien n'est écrasé"
    Reinitialiser
    $v8 = [byte[]](8..207); $v9 = [byte[]](9..208)
    $script:partie = { Envoi-3ds $v8 $v6 | Out-Null; [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), $v9) }
    $script:reponses = @('Ne rien faire')
    Invoke-Lanceur $rom $config | Out-Null
    Verifier ($script:questions.Count -eq 1 -and $script:questions[0] -match 'Conflit') "une question de Conflit après la partie"
    Verifier ((Hash (Courante 'IRBF')) -eq (Hash $v8)) "le serveur garde la Version de la 3DS"
    Verifier ((Local-Hash) -eq (Hash $v9)) "le PC garde la Sauvegarde jouée"

    Write-Host "Conflit après la partie, Oui : le PC envoie la sienne"
    Reinitialiser
    $v10 = [byte[]](10..209); $v11 = [byte[]](11..210)
    # avant : prendre le serveur (v8) ; pendant : la 3DS envoie v10 et le PC écrit v11 ; après : garder le PC
    $script:partie = { Envoi-3ds $v10 $v8 | Out-Null; [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), $v11) }
    $script:reponses = @('Prendre le serveur', 'Garder le PC')
    Invoke-Lanceur $rom $config | Out-Null
    Verifier ($script:questions.Count -eq 2) "deux questions de Conflit"
    Verifier ((Hash (Courante 'IRBF')) -eq (Hash $v11)) "le serveur a la Sauvegarde du PC"
    Verifier ((Historique 'IRBF').hash -contains (Hash $v10)) "la Version de la 3DS reste dans l'Historique"

    Write-Host "Sauvegarde tronquée pendant la partie : Conflit, rien n'est envoyé"
    Reinitialiser
    $script:partie = { [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), [byte[]](1..100)) }
    $script:reponses = @('Ne rien faire')
    Invoke-Lanceur $rom $config | Out-Null
    Verifier ($script:questions.Count -eq 1 -and $script:questions[0] -match 'Conflit') "une question de Conflit"
    Verifier ((Hash (Courante 'IRBF')) -eq (Hash $v11)) "le serveur garde la Version entière"

    Write-Host "Course : la 3DS envoie entre le GET et le PUT du PC (412)"
    Reinitialiser
    [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), $v11)  # retour à l'état synchronisé
    Invoke-Lanceur $rom $config | Out-Null
    $v12 = [byte[]](12..211); $v13 = [byte[]](13..212)
    [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), $v12)
    ${function:Get-ServeurVrai} = ${function:Get-Serveur}
    $script:course = $true
    function Get-Serveur($ctx) {
        $s = Get-ServeurVrai $ctx
        if ($script:course) { $script:course = $false; Envoi-3ds $v13 $v11 | Out-Null }
        $s
    }
    Reinitialiser
    $script:reponses = @('Ne pas jouer')
    Invoke-Lanceur $rom $config | Out-Null
    ${function:Get-Serveur} = ${function:Get-ServeurVrai}
    Verifier ($script:questions.Count -eq 1 -and $script:questions[0] -match 'Conflit') "le 412 devient une question de Conflit"
    Verifier ((Hash (Courante 'IRBF')) -eq (Hash $v13)) "la Version de la 3DS n'est pas écrasée"
    Verifier ((Local-Hash) -eq (Hash $v12)) "le PC garde sa Sauvegarde"

    Write-Host "melonDS tourne déjà : refus"
    Reinitialiser
    $script:melonActif = $true
    Verifier ((Invoke-Lanceur $rom $config) -eq 1) "code de sortie 1"
    Verifier ($script:questions.Count -eq 0 -and $script:notifications[0] -match 'tourne déjà') "notification, aucune fenêtre"
    Verifier ((Get-Content "$tmp\cfg\ndssync.log" -Raw -ErrorAction SilentlyContinue) -match 'refus') "le refus est écrit dans ndssync.log"
    Verifier (-not $script:joue) "melonDS non lancé"

    Write-Host "Homebrew (code ####) : partie sans Synchronisation"
    Reinitialiser
    $homebrew = Nouvelle-Rom 'Homebrew' '####'
    [IO.File]::WriteAllBytes((Sav 'Homebrew'), [byte[]](1..10))
    Invoke-Lanceur $homebrew $config | Out-Null
    Verifier $script:joue "melonDS lancé"
    Verifier ((Api Get '/saves/____').statut -ne 200 -and $script:questions.Count -eq 0) "rien envoyé, aucune question"

    Write-Host "ROM en .zip : Code jeu et nom lus dans l'archive"
    Reinitialiser
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $nds = Nouvelle-Rom 'Pokemon Blanche' 'IRAF'
    $zip = "$tmp\roms\Pokemon Blanche (archive).zip"
    $archive = [IO.Compression.ZipFile]::Open($zip, 'Create')
    [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, $nds, 'Pokemon Blanche.nds') | Out-Null
    $archive.Dispose()
    $blanche = [byte[]](20..219)
    [IO.File]::WriteAllBytes((Sav 'Pokemon Blanche'), $blanche)
    Invoke-Lanceur $zip $config | Out-Null
    Verifier ((Hash (Courante 'IRAF')) -eq (Hash $blanche)) "le serveur a la Sauvegarde de Pokemon Blanche.sav"

    Write-Host "Serveur injoignable avant la partie, Non : pas de partie"
    Reinitialiser
    $hors = "$tmp\cfg\hors-ligne.json"
    @{ url = 'http://127.0.0.1:1'; jeton = 'x'; melonds = 'inutilise.exe'; sauvegardes = "$tmp\saves" } | ConvertTo-Json | Set-Content $hors
    $script:reponses = @('No')
    Invoke-Lanceur $rom $hors | Out-Null
    Verifier ($script:questions[0] -match 'Synchronisation impossible') "question « Synchronisation impossible »"
    Verifier (-not $script:joue) "melonDS non lancé"

    Write-Host "Serveur injoignable, Oui : partie, puis simple information"
    Reinitialiser
    $script:reponses = @('Yes')
    $script:partie = { [IO.File]::WriteAllBytes((Sav 'Pokemon Noire'), $v13) }
    Invoke-Lanceur $rom $hors | Out-Null
    Verifier $script:joue "melonDS lancé"
    Verifier ($script:questions.Count -eq 1 -and $script:notifications[0] -match 'reste sur le PC') "notification après la partie, pas de fenêtre"
    Verifier ((Local-Hash) -eq (Hash $v13)) "la Sauvegarde jouée reste sur le PC"

    Write-Host "Copie de sécurité impossible : rien n'est écrasé"
    Reinitialiser
    New-Item -ItemType Directory "$tmp\saves2\ndssync-copies" -Force | Out-Null
    $cfg2 = "$tmp\cfg2\sync.config.json"; New-Item -ItemType Directory "$tmp\cfg2" -Force | Out-Null
    @{ url = "http://127.0.0.1:$port"; jeton = 'jeton-test'; melonds = 'inutilise.exe'; sauvegardes = "$tmp\saves2" } | ConvertTo-Json | Set-Content $cfg2
    $platine = Nouvelle-Rom 'Pokemon Platine' 'CPUF'
    $p1 = [byte[]](30..229); $p2 = [byte[]](31..230)
    [IO.File]::WriteAllBytes("$tmp\saves2\Pokemon Platine.sav", $p1)
    Invoke-Lanceur $platine $cfg2 | Out-Null                       # premier Envoi : Base = p1
    Api Put '/saves/CPUF' $p2 @{ 'If-Match' = "`"$(Hash $p1)`""; 'X-Client' = '3ds' } | Out-Null
    # Copy-Item vers une cible en lecture seule échoue SANS arrêter le script (erreur non terminale).
    foreach ($s in 0..20) {
        $f = "$tmp\saves2\ndssync-copies\Pokemon Platine-$((Get-Date).AddSeconds($s).ToString('yyyyMMdd-HHmmss')).sav"
        Set-Content $f 'occupe'; (Get-Item $f).IsReadOnly = $true
    }
    Reinitialiser
    $script:reponses = @('No')
    Invoke-Lanceur $platine $cfg2 | Out-Null
    Get-ChildItem "$tmp\saves2\ndssync-copies" | ForEach-Object { $_.IsReadOnly = $false }
    Verifier ((Hash ([IO.File]::ReadAllBytes("$tmp\saves2\Pokemon Platine.sav"))) -eq (Hash $p1)) "la Sauvegarde du PC est intacte"
    Verifier ($script:questions.Count -eq 1 -and $script:questions[0] -match 'Synchronisation impossible') "question « Synchronisation impossible »"
    Verifier (-not $script:joue) "melonDS non lancé"

    # --- GB / GBA (ADR 0007, 0008) --------------------------------------------------------
    Write-Host "GB : Clé lue dans l'en-tête, seule la SRAM part sur le serveur"
    Reinitialiser
    $rouge = Nouvelle-RomZip 'Pokemon - Version Rouge (France) (SGB Enhanced)' 'gb' (Entete-GB 'POKEMON RED' 0x91E6)
    $sram1 = New-Object byte[] 32768; (New-Object Random 40).NextBytes($sram1)
    $horloge = [byte[]](1..48)
    [IO.File]::WriteAllBytes("$tmp\gb\Pokemon - Version Rouge (France) (SGB Enhanced).sav", [byte[]]($sram1 + $horloge))
    Invoke-Lanceur $rouge $config | Out-Null
    $c = Courante 'GB-POKEMON_RED-91E6'
    Verifier ($c.Length -eq 32768 -and (Hash $c) -eq (Hash $sram1)) "le serveur a la SRAM seule (32 768 o), sous GB-POKEMON_RED-91E6"
    Verifier $script:joue "mGBA lancé"

    Write-Host "GB : la Récupération garde le Trailer d'horloge du PC"
    Reinitialiser
    $sram2 = New-Object byte[] 32768; (New-Object Random 41).NextBytes($sram2)
    Api Put '/saves/GB-POKEMON_RED-91E6' $sram2 @{ 'If-Match' = "`"$(Hash $sram1)`""; 'X-Client' = '3ds' } | Out-Null
    Invoke-Lanceur $rouge $config | Out-Null
    $local = [IO.File]::ReadAllBytes("$tmp\gb\Pokemon - Version Rouge (France) (SGB Enhanced).sav")
    Verifier ($local.Length -eq 32816) "le fichier garde sa taille avec horloge (32 816 o)"
    Verifier ((Hash ([byte[]]$local[0..32767])) -eq (Hash $sram2)) "la SRAM est celle de la 3DS"
    Verifier ((Hash ([byte[]]$local[32768..32815])) -eq (Hash $horloge)) "le Trailer d'horloge du PC est intact"
    Verifier (@(Get-ChildItem "$tmp\gb\ndssync-copies" -Filter '*.sav').Count -eq 1) "Copie de sécurité faite"
    Verifier ($script:questions.Count -eq 0) "aucune question (horloge ignorée dans la comparaison)"

    Write-Host "GBA : Clé GBA-<code>, .sav nommé comme le zip (règle mGBA)"
    Reinitialiser
    $emeraude = Nouvelle-RomZip 'Pokemon - Version Emeraude (France)' 'gba' (Entete-GBA 'BPEF')
    $g1 = New-Object byte[] 131072; (New-Object Random 42).NextBytes($g1)
    [IO.File]::WriteAllBytes("$tmp\gba\Pokemon - Version Emeraude (France).sav", [byte[]]($g1 + [byte[]](1..16)))
    Invoke-Lanceur $emeraude $config | Out-Null
    Verifier ((Hash (Courante 'GBA-BPEF')) -eq (Hash $g1)) "le serveur a la SRAM d'Émeraude sous GBA-BPEF"

    # --- 3DS (ADR 0009) -------------------------------------------------------------------
    Write-Host "3DS : Clé et dossier Azahar lus dans la ROM"
    Reinitialiser
    $x = Nouvelle-Rom3DS 'Pokemon X (Europe)' 0x0004000000055D00
    $dx = Dossier-Azahar 0x0004000000055D00
    New-Item -ItemType Directory $dx -Force | Out-Null
    $m1 = New-Object byte[] 415232; (New-Object Random 43).NextBytes($m1); [IO.File]::WriteAllBytes("$dx\main", $m1)
    Invoke-Lanceur $x $config | Out-Null
    Verifier ((Hash (Courante '3DS-0004000000055D00')) -eq (Hash $m1)) "le serveur a main sous 3DS-0004000000055D00"
    Verifier $script:joue "Azahar lancé"

    Write-Host "3DS : la Récupération n'ajoute rien dans l'archive Azahar"
    Reinitialiser
    $m2 = New-Object byte[] 415232; (New-Object Random 44).NextBytes($m2)
    Api Put '/saves/3DS-0004000000055D00' $m2 @{ 'If-Match' = "`"$(Hash $m1)`""; 'X-Client' = '3ds' } | Out-Null
    Invoke-Lanceur $x $config | Out-Null
    Verifier ((Hash ([IO.File]::ReadAllBytes("$dx\main"))) -eq (Hash $m2)) "main est la Version de la 3DS"
    Verifier ((@(Get-ChildItem $dx -Force).Name -join ',') -eq 'main') "l'archive ne contient que main"
    Verifier (@(Get-ChildItem "$tmp\copies3ds" -File).Count -eq 1) "Copie de sécurité hors de l'archive"

    Write-Host "3DS : jeu jamais lancé dans Azahar, rien n'est créé"
    Reinitialiser
    $y = Nouvelle-Rom3DS 'Pokemon Y (Europe)' 0x0004000000055E00
    Api Put '/saves/3DS-0004000000055E00' $m2 @{ 'If-None-Match' = '*'; 'X-Client' = '3ds' } | Out-Null
    $script:reponses = @('No')
    Invoke-Lanceur $y $config | Out-Null
    Verifier ($script:questions.Count -eq 1 -and $script:questions[0] -match 'Azahar') "question qui explique de lancer le jeu une fois"
    Verifier (-not (Test-Path (Dossier-Azahar 0x0004000000055E00))) "aucun dossier créé dans Azahar"

    Write-Host "3DS hors des 8 Pokémon : partie sans Synchronisation"
    Reinitialiser
    $autre = Nouvelle-Rom3DS 'Zelda (Europe)' 0x0004000000033500
    Invoke-Lanceur $autre $config | Out-Null
    Verifier ($script:joue -and $script:questions.Count -eq 0) "Azahar lancé, aucune question"

    # --- Sauvegarde vierge = absente (amélioration B) ---------------------------------------
    Write-Host "Sauvegarde vierge (0xFF) sur le PC, Version sur le serveur : Récupération sans Conflit"
    Reinitialiser
    $perle = Nouvelle-Rom 'Pokemon Perle' 'APAF'
    $vierge = New-Object byte[] 524288; for ($i = 0; $i -lt $vierge.Length; $i++) { $vierge[$i] = 0xFF }
    [IO.File]::WriteAllBytes((Sav 'Pokemon Perle'), $vierge)
    $pe = New-Object byte[] 524288; (New-Object Random 45).NextBytes($pe)
    Api Put '/saves/APAF' $pe @{ 'If-None-Match' = '*'; 'X-Client' = '3ds' } | Out-Null
    Invoke-Lanceur $perle $config | Out-Null
    Verifier ($script:questions.Count -eq 0) "aucune question de Conflit"
    Verifier ((Hash ([IO.File]::ReadAllBytes((Sav 'Pokemon Perle')))) -eq (Hash $pe)) "le PC a la Version du serveur"
    Verifier ([bool](Get-ChildItem "$tmp\saves\ndssync-copies" -Filter 'Pokemon Perle-*.sav')) "Copie de sécurité de la Sauvegarde vierge"

    Write-Host "Sauvegarde vierge (0x00 + Trailer d'horloge), rien sur le serveur : rien n'est envoyé"
    Reinitialiser
    $or = Nouvelle-RomZip 'Pokemon - Version Or (France)' 'gbc' (Entete-GB 'POKEMON_GLDAAUF' 0x6FC6)
    [IO.File]::WriteAllBytes("$tmp\gb\Pokemon - Version Or (France).sav", [byte[]]((New-Object byte[] 32768) + [byte[]](1..48)))
    Invoke-Lanceur $or $config | Out-Null
    Verifier ((Api Get '/saves/GB-POKEMON_GLDAAUF-6FC6').statut -eq 404) "le serveur n'a rien reçu"
    Verifier $script:joue "mGBA lancé"

    Write-Host "3DS : un main à 0x00 n'est pas une Sauvegarde vierge (il part)"
    Reinitialiser
    $lune = Nouvelle-Rom3DS 'Pokemon Lune (Europe)' 0x0004000000175E00
    $dl = Dossier-Azahar 0x0004000000175E00
    New-Item -ItemType Directory $dl -Force | Out-Null
    [IO.File]::WriteAllBytes("$dl\main", (New-Object byte[] 1024))
    Invoke-Lanceur $lune $config | Out-Null
    Verifier ((Api Get '/saves/3DS-0004000000175E00').statut -eq 200) "le serveur a main"
}
finally {
    Stop-Process $serveur.Id -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
Write-Host ($(if ($script:echecs) { "$($script:echecs) ECHEC(S)" } else { "TOUT OK" }))
exit $script:echecs
