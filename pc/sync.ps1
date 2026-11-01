# Lanceur ndssync : appelé par LaunchBox à la place de l'émulateur (docs/design.md §3, ADR 0006).
# Systèmes : DS (melonDS), GB/GBC et GBA (mGBA), 3DS (Azahar) — ADR 0007 à 0009.
# LaunchBox : powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "<ce fichier>" "<rom>"
param([string]$Rom, [string]$Config = "$PSScriptRoot\sync.config.json")

$ErrorActionPreference = 'Stop'  # toute erreur arrête la Synchronisation : jamais d'écrasement après un échec
Add-Type -AssemblyName System.Net.Http, System.Windows.Forms, System.IO.Compression.FileSystem

# État d'un Jeu (docs/design.md §1, ADR 0001). $local et $serveur : @{hash; taille} ou $null. $base : hash ou $null.
function Get-Etat($local, $serveur, $base) {
    if ($local.hash -eq $serveur.hash) { return 'A_JOUR' }
    if (-not $serveur) { return 'A_ENVOYER' }
    if (-not $local) { return 'A_RECUPERER' }
    if ($local.taille -ne $serveur.taille) { return 'CONFLIT' }
    if ($local.hash -eq $base) { return 'A_RECUPERER' }
    if ($serveur.hash -eq $base) { return 'A_ENVOYER' }
    'CONFLIT'
}

# --- Adaptateurs remplacés par les tests ----------------------------------------------------
# $boutons : 'OK' / 'YesNo' (MessageBox) ou une liste de libellés (fenêtre à boutons nommés).
# Renvoie le nom du bouton MessageBox ('Yes', 'No', 'OK') ou le libellé choisi.
function Demander($texte, $boutons) {
    if ($boutons -is [string]) {
        $proprietaire = New-Object Windows.Forms.Form -Property @{ TopMost = $true }
        return [string][Windows.Forms.MessageBox]::Show($proprietaire, $texte, 'ndssync', $boutons)
    }
    $f = New-Object Windows.Forms.Form -Property @{
        Text = 'ndssync'; TopMost = $true; StartPosition = 'CenterScreen'; FormBorderStyle = 'FixedDialog'
        MaximizeBox = $false; MinimizeBox = $false; AutoSize = $true; AutoSizeMode = 'GrowAndShrink'; Padding = 14
        Font = New-Object Drawing.Font 'Segoe UI', 10
    }
    $pile = New-Object Windows.Forms.FlowLayoutPanel -Property @{ FlowDirection = 'TopDown'; AutoSize = $true }
    $pile.Controls.Add((New-Object Windows.Forms.Label -Property @{ Text = $texte; AutoSize = $true; MaximumSize = '480, 0'; Margin = '0, 0, 0, 14' }))
    $ligne = New-Object Windows.Forms.FlowLayoutPanel -Property @{ AutoSize = $true }
    $script:choix = $boutons[-1]  # fermer la fenêtre = dernier bouton, le plus prudent
    foreach ($b in $boutons) {
        $bouton = New-Object Windows.Forms.Button -Property @{ Text = $b; AutoSize = $true; Padding = '10, 4, 10, 4' }
        $bouton.Add_Click({ $script:choix = $this.Text; $this.FindForm().Close() })
        $ligne.Controls.Add($bouton)
    }
    $pile.Controls.Add($ligne); $f.Controls.Add($pile)
    $f.Add_Shown({ $ligne.Controls[$ligne.Controls.Count - 1].Focus() })  # Entrée = choix le plus prudent
    [void]$f.ShowDialog()
    $script:choix
}
# Information sans choix : notification Windows, rien à valider. Le Lanceur se termine juste après :
# on attend que la notification s'affiche, sinon Windows la retire avec l'icône.
function Notifier($texte) {
    $icone = New-Object Windows.Forms.NotifyIcon -Property @{ Icon = [Drawing.SystemIcons]::Information; Visible = $true; Text = 'ndssync' }
    $icone.ShowBalloonTip(8000, 'ndssync', $texte, 'Info')
    Start-Sleep -Seconds 8
    $icone.Dispose()
}
function Start-Jeu($ctx, $rom) {
    $s = $SYSTEMES[$ctx.systeme]
    $arguments = switch ($ctx.systeme) {
        { $_ -in 'GB', 'GBA' } { "-C `"savegamePath=$($ctx.cfg.($s.dossier))`" `"$rom`"" }  # jamais --config (mGBA quitte)
        '3DS' { "-f `"$rom`"" }
        default { "`"$rom`"" }
    }
    Start-Process $ctx.cfg.($s.exe) -ArgumentList $arguments -Wait
}
function Test-EmulateurActif($processus) { [bool](Get-Process $processus -ErrorAction SilentlyContinue) }

# --- Systèmes (ADR 0007) ------------------------------------------------------------------
# exe / dossier : clés de sync.config.json ; processus : nom à surveiller ; extensions : ROM reconnues.
$SYSTEMES = @{
    DS  = @{ exe = 'melonds'; processus = 'melonDS'; dossier = 'sauvegardes';     extensions = '.nds' }
    GB  = @{ exe = 'mgba';    processus = 'mGBA';    dossier = 'sauvegardes_gb';  extensions = '.gb', '.gbc' }
    GBA = @{ exe = 'mgba';    processus = 'mGBA';    dossier = 'sauvegardes_gba'; extensions = '.gba' }
    '3DS' = @{ exe = 'azahar'; processus = 'azahar'; extensions = '.3ds', '.cci', '.cxi' }
}
# Les 8 Pokémon 3DS synchronisés (ADR 0009) : un title ID par jeu, toutes régions.
$TITRES_3DS = @{
    '0004000000055D00' = 'Pokemon X'; '0004000000055E00' = 'Pokemon Y'
    '000400000011C400' = 'Pokemon Rubis Omega'; '000400000011C500' = 'Pokemon Saphir Alpha'
    '0004000000164800' = 'Pokemon Soleil'; '0004000000175E00' = 'Pokemon Lune'
    '00040000001B5000' = 'Pokemon Ultra-Soleil'; '00040000001B5100' = 'Pokemon Ultra-Lune'
}

# --- Disque -------------------------------------------------------------------------------
# @{systeme; cle; nom} d'une ROM (fichier ou .zip). cle = $null si illisible ou hors périmètre.
# nom = nom du .sav : le .nds interne pour melonDS, le zip lui-même pour mGBA (qui nomme d'après le zip).
function Get-RomInfo($rom) {
    $entete = New-Object byte[] 0x200
    $ext = [IO.Path]::GetExtension($rom).ToLower()
    $nom = [IO.Path]::GetFileNameWithoutExtension($rom)
    if ($ext -eq '.zip') {
        $archive = [IO.Compression.ZipFile]::OpenRead($rom)
        try {
            $entree = $archive.Entries | Where-Object { [IO.Path]::GetExtension($_.Name) -in '.nds', '.gb', '.gbc', '.gba' } | Select-Object -First 1
            if (-not $entree) { return @{} }
            $flux = $entree.Open(); [void]$flux.Read($entete, 0, $entete.Length); $flux.Dispose()
            $ext = [IO.Path]::GetExtension($entree.Name).ToLower()
            if ($ext -eq '.nds') { $nom = [IO.Path]::GetFileNameWithoutExtension($entree.Name) }
        } finally { $archive.Dispose() }
    } else {
        $flux = [IO.File]::OpenRead($rom)
        try { [void]$flux.Read($entete, 0, $entete.Length) } finally { $flux.Dispose() }
    }
    $systeme = ($SYSTEMES.Keys | Where-Object { $ext -in $SYSTEMES[$_].extensions } | Select-Object -First 1)
    $ascii = [Text.Encoding]::ASCII
    $cle = switch ($systeme) {
        'DS' { $c = $ascii.GetString($entete, 0x0C, 4); if ($c -cmatch '^[A-Z0-9]{4}$') { $c } }
        'GBA' { $c = $ascii.GetString($entete, 0xAC, 4); if ($c -cmatch '^[A-Z0-9]{4}$') { "GBA-$c" } }
        'GB' {
            $titre = $ascii.GetString($entete, 0x134, 15).Split([char]0)[0].ToUpper() -replace '[^A-Z0-9]', '_'
            if ($titre) { 'GB-{0}-{1:X2}{2:X2}' -f $titre, $entete[0x14E], $entete[0x14F] }
        }
        '3DS' { $id = Get-TitleId3DS $rom; if ($TITRES_3DS.ContainsKey($id)) { $nom = $TITRES_3DS[$id]; "3DS-$id" } }
    }
    @{ systeme = $systeme; cle = $cle; nom = $nom }
}

# Title ID d'une ROM 3DS : NCSD (.3ds/.cci) -> partition 0 à u32@0x120 × 0x200, ou NCCH direct (.cxi) ; program_id à NCCH + 0x118.
function Get-TitleId3DS($rom) {
    $flux = [IO.File]::OpenRead($rom)
    try {
        $o = New-Object byte[] 0x200; [void]$flux.Read($o, 0, 0x200)
        $magie = [Text.Encoding]::ASCII.GetString($o, 0x100, 4)
        $ncch = if ($magie -eq 'NCSD') { [long][BitConverter]::ToUInt32($o, 0x120) * 0x200 } elseif ($magie -eq 'NCCH') { 0 } else { return $null }
        [void]$flux.Seek($ncch + 0x118, 'Begin'); $id = New-Object byte[] 8; [void]$flux.Read($id, 0, 8)
        '{0:X16}' -f [BitConverter]::ToUInt64($id, 0)
    } finally { $flux.Dispose() }
}

# Fichier de sauvegarde et dossier des Copies de sécurité d'un Jeu.
function Get-Emplacement($cfg, $info) {
    if ($info.systeme -eq '3DS') {
        $id = $info.cle.Substring(4); $z = '0' * 32
        $dossier = Join-Path $cfg.azahar_sdmc "Nintendo 3DS\$z\$z\title\$($id.Substring(0, 8).ToLower())\$($id.Substring(8).ToLower())\data\00000001"
        return @{ fichier = Join-Path $dossier 'main'; copies = $cfg.copies_3ds }  # jamais de copie dans l'archive Azahar
    }
    $dossier = $cfg.($SYSTEMES[$info.systeme].dossier)
    @{ fichier = Join-Path $dossier "$($info.nom).sav"; copies = Join-Path $dossier 'ndssync-copies' }
}

function Get-Hash($octets) {
    -join ([Security.Cryptography.SHA256]::Create().ComputeHash($octets) | ForEach-Object { $_.ToString('x2') })
}

function Get-Base($ctx) {
    if (Test-Path $ctx.base) { (Get-Content $ctx.base -Raw | ConvertFrom-Json).($ctx.code) }
}
function Set-Base($ctx, $hash) {
    $tout = @{}
    if (Test-Path $ctx.base) { (Get-Content $ctx.base -Raw | ConvertFrom-Json).PSObject.Properties | ForEach-Object { $tout[$_.Name] = $_.Value } }
    $tout[$ctx.code] = $hash
    $tout | ConvertTo-Json | Set-Content $ctx.base -Encoding UTF8
}

# --- Réseau -------------------------------------------------------------------------------
function Invoke-Api($ctx, $methode, $chemin, $octets, $entetes = @{}) {
    $client = New-Object Net.Http.HttpClient
    $client.Timeout = [TimeSpan]::FromSeconds(10)
    $req = New-Object Net.Http.HttpRequestMessage($methode, "$($ctx.cfg.url)$chemin")
    $req.Headers.TryAddWithoutValidation('Authorization', "Bearer $($ctx.cfg.jeton)") | Out-Null
    $req.Headers.TryAddWithoutValidation('X-Client', 'pc') | Out-Null
    foreach ($nom in $entetes.Keys) { $req.Headers.TryAddWithoutValidation($nom, $entetes[$nom]) | Out-Null }
    if ($null -ne $octets) { $req.Content = New-Object Net.Http.ByteArrayContent(, $octets) }
    $r = $client.SendAsync($req).GetAwaiter().GetResult()
    $statut = [int]$r.StatusCode
    if ($statut -eq 401) { throw 'jeton refusé par le serveur (401)' }
    @{ statut = $statut; octets = $r.Content.ReadAsByteArrayAsync().Result }
}

# Version courante du Jeu : @{hash; taille; date; client} ou $null.
function Get-Serveur($ctx) {
    $r = Invoke-Api $ctx Get '/saves'
    if ($r.statut -ne 200) { throw "GET /saves : $($r.statut)" }
    $liste = [Text.Encoding]::UTF8.GetString($r.octets) | ConvertFrom-Json  # PS 5.1 : tableau = un seul objet,
    $jeu = $liste | Where-Object code -eq $ctx.code                         # la variable force l'énumération
    if ($jeu) { @{ hash = $jeu.hash; taille = [long]$jeu.taille; date = $jeu.date; client = $jeu.client; age = $jeu.age } }
}

# Sauvegarde locale : @{hash; taille; octets; horloge} ou $null.
# GB/GBA : hash, taille et octets portent sur la SRAM seule ; le Trailer d'horloge reste local (ADR 0008).
function Get-Local($ctx) {
    if (-not (Test-Path -LiteralPath $ctx.fichier)) { return }
    $octets = [IO.File]::ReadAllBytes($ctx.fichier)
    $horloge = New-Object byte[] 0
    $n = $octets.Length - $octets.Length % 1024  # les trailers font moins de 1 Kio
    if ($ctx.systeme -in 'GB', 'GBA' -and $n -gt 0 -and $n -lt $octets.Length) {
        $horloge = New-Object byte[] ($octets.Length - $n); [Array]::Copy($octets, $n, $horloge, 0, $horloge.Length)
        $sram = New-Object byte[] $n; [Array]::Copy($octets, $sram, $n); $octets = $sram
    }
    # vide : SRAM vierge (que des 0x00 ou que des 0xFF), écrite par TWiLight ou mGBA avant toute partie.
    $vide = $ctx.systeme -ne '3DS' -and ($octets.Length -eq 0 -or ($octets[0] -in 0, 255 -and @($octets -ne $octets[0]).Count -eq 0))
    @{ hash = Get-Hash $octets; taille = [long]$octets.Length; octets = $octets; horloge = $horloge; vide = $vide }
}

# --- Synchronisation ----------------------------------------------------------------------
# Envoi conditionnel : If-Match = Version courante vue par ce Client. Renvoie $false sur 412.
function Send-Sauvegarde($ctx, $local, $serveur) {
    $condition = if ($serveur) { @{ 'If-Match' = "`"$($serveur.hash)`"" } } else { @{ 'If-None-Match' = '*' } }
    $r = Invoke-Api $ctx Put "/saves/$($ctx.code)" $local.octets $condition
    if ($r.statut -eq 412) { return $false }
    if ($r.statut -ne 200) { throw "PUT : $($r.statut)" }
    Set-Base $ctx $local.hash
    $true
}

# Récupération : Copie de sécurité, fichier temporaire vérifié, puis remplacement (ADR 0004, design §3).
function Receive-Sauvegarde($ctx, $local, $serveur) {
    $r = Invoke-Api $ctx Get "/saves/$($ctx.code)"
    if ($r.statut -ne 200) { throw "GET /saves/$($ctx.code) : $($r.statut)" }
    if ((Get-Hash $r.octets) -ne $serveur.hash) { throw 'Version reçue corrompue (Hash différent)' }
    $dossier = Split-Path $ctx.fichier
    if ($ctx.systeme -eq '3DS') {
        # Pas de dossier = Azahar n'a jamais vu ce jeu : il faut son .metadata, on ne crée rien (ADR 0009).
        if (-not (Test-Path -LiteralPath $dossier)) { throw "jeu jamais lancé dans Azahar. Lance-le une fois et sauvegarde en jeu, puis relance-le ici." }
    } else {
        New-Item -ItemType Directory $dossier -Force -ErrorAction Stop | Out-Null
    }
    New-Item -ItemType Directory $ctx.copies -Force -ErrorAction Stop | Out-Null
    if ($local) {
        $nom = $ctx.nom + (Get-Date -Format '-yyyyMMdd-HHmmss') + [IO.Path]::GetExtension($ctx.fichier)
        Copy-Item -LiteralPath $ctx.fichier -Destination (Join-Path $ctx.copies $nom) -ErrorAction Stop
    }
    # Fichier temporaire hors du dossier du jeu (l'archive Azahar ne doit contenir que main).
    $tmp = Join-Path $ctx.copies "$($ctx.nom).ndssync-tmp"
    $contenu = $r.octets
    if ($local -and $local.horloge.Length) {  # GB/GBA : on remet le Trailer d'horloge de ce PC après la SRAM reçue
        $contenu = New-Object byte[] ($r.octets.Length + $local.horloge.Length)
        [Array]::Copy($r.octets, $contenu, $r.octets.Length)
        [Array]::Copy($local.horloge, 0, $contenu, $r.octets.Length, $local.horloge.Length)
    }
    [IO.File]::WriteAllBytes($tmp, $contenu)
    Move-Item -LiteralPath $tmp -Destination $ctx.fichier -Force -ErrorAction Stop
    Set-Base $ctx $serveur.hash
}

# « il y a 2 h » à partir de l'âge en secondes donné par le serveur.
function Format-Age($secondes) {
    if ($null -eq $secondes) { return '' }
    if ($secondes -lt 60) { return "à l'instant" }
    if ($secondes -lt 3600) { return "il y a $([int][Math]::Floor($secondes / 60)) min" }
    if ($secondes -lt 86400) { return "il y a $([int][Math]::Floor($secondes / 3600)) h" }
    "il y a $([int][Math]::Floor($secondes / 86400)) j"
}

# Choix de l'utilisateur sur un Conflit (Q5) : 'A_RECUPERER', 'FORCER', 'RIEN' ou 'ANNULER'.
function Resolve-Conflit($ctx, $local, $serveur, $apres) {
    $texte = "Conflit : $($ctx.nom)`n`n" +
        "La Sauvegarde du PC et celle du serveur ont toutes deux changé depuis la dernière Synchronisation.`n`n" +
        "PC : $($local.taille) octets ($($local.hash.Substring(0, 8)))`n" +
        "Serveur : $($serveur.taille) octets ($($serveur.hash.Substring(0, 8))), envoyée par $($serveur.client) $(Format-Age $serveur.age)`n`n" +
        "L'autre version reste dans l'Historique du serveur."
    if ($apres) {
        switch (Demander $texte @('Garder le PC', 'Ne rien faire')) { 'Garder le PC' { 'FORCER' } default { 'RIEN' } }
    } else {
        switch (Demander $texte @('Prendre le serveur', 'Garder le PC', 'Ne pas jouer')) {
            'Prendre le serveur' { 'A_RECUPERER' } 'Garder le PC' { 'FORCER' } default { 'ANNULER' }
        }
    }
}

# Une passe de Synchronisation. Renvoie 'ok' ou 'annule'.
function Sync-Jeu($ctx, $apres) {
    foreach ($essai in 1..3) {  # un 412 relance une passe avec la nouvelle Version courante
        $local = Get-Local $ctx
        $serveur = Get-Serveur $ctx
        # Une Sauvegarde vierge compte comme absente ; le fichier reste copié avant écrasement (Receive-Sauvegarde).
        $vu = if (-not $local.vide -or $local.hash -eq $serveur.hash) { $local }
        if (-not $vu -and -not $serveur) { return 'ok' }
        $action = Get-Etat $vu $serveur (Get-Base $ctx)
        if ($action -eq 'CONFLIT') { $action = Resolve-Conflit $ctx $local $serveur $apres }
        switch ($action) {
            'A_JOUR' { Set-Base $ctx $local.hash; return 'ok' }
            { $_ -in 'A_ENVOYER', 'FORCER' } { if (Send-Sauvegarde $ctx $local $serveur) { return 'ok' } }
            'A_RECUPERER' { Receive-Sauvegarde $ctx $local $serveur; return 'ok' }
            'RIEN' { return 'ok' }
            'ANNULER' { return 'annule' }
        }
    }
    throw 'le serveur change sans cesse (412 répétés)'
}

function Invoke-Lanceur($rom, $config = "$PSScriptRoot\sync.config.json") {
    # Chaque lancement laisse une trace, même s'il s'arrête avant la synchronisation.
    $journal = Join-Path (Split-Path $config) 'ndssync.log'
    $cfg = Get-Content $config -Raw | ConvertFrom-Json
    $info = Get-RomInfo $rom
    Add-Content $journal "$(Get-Date -Format s) lancement $($info.systeme) $($info.cle) : $rom"
    if (-not $info.systeme) { throw "ROM non reconnue (extension) : $rom" }
    $processus = $SYSTEMES[$info.systeme].processus
    if (Test-EmulateurActif $processus) {
        Add-Content $journal "$(Get-Date -Format s) refus : $processus tourne déjà"
        Notifier "$processus tourne déjà. Ferme-le avant de lancer un jeu."
        return 1
    }
    $ctx = @{ cfg = $cfg; systeme = $info.systeme; code = $info.cle; nom = $info.nom; base = Join-Path (Split-Path $config) 'base.json' }
    if (-not $info.cle) {  # homebrew, ou 3DS hors des 8 Pokémon
        Add-Content $journal "$(Get-Date -Format s) sans synchronisation : Clé illisible ou jeu hors périmètre"
        Start-Jeu $ctx $rom
        return 0
    }
    $ctx += Get-Emplacement $cfg $info
    try {
        if ((Sync-Jeu $ctx $false) -eq 'annule') { return 0 }
    } catch {
        Add-Content $journal "$(Get-Date -Format s) $($ctx.code) avant : $_"
        $r = Demander "Synchronisation impossible : $(Get-Erreur $_)`n`nJouer avec la Sauvegarde locale ?" 'YesNo'
        if ($r -ne 'Yes') { return 0 }
    }
    Start-Jeu $ctx $rom
    try {
        Sync-Jeu $ctx $true | Out-Null
    } catch {
        Add-Content $journal "$(Get-Date -Format s) $($ctx.code) après : $_"
        Notifier "Envoi impossible ($(Get-Erreur $_)).`nLa Sauvegarde reste sur le PC ; elle partira au prochain lancement."
    }
    0
}

# Message le plus précis d'une erreur (HttpClient emballe la vraie cause).
function Get-Erreur($e) {
    $x = $e.Exception
    while ($x.InnerException) { $x = $x.InnerException }
    $x.Message
}

if ($MyInvocation.InvocationName -ne '.') {
    # La fenêtre est cachée : une erreur imprévue doit s'afficher, pas disparaître.
    try { exit (Invoke-Lanceur $Rom $Config) } catch {
        Add-Content (Join-Path (Split-Path $Config) 'ndssync.log') "$(Get-Date -Format s) erreur : $(Get-Erreur $_) ($Rom)"
        Demander "Erreur du Lanceur ndssync : $(Get-Erreur $_)" 'OK' | Out-Null
        exit 1
    }
}
