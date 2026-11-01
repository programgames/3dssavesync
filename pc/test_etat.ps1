# Vérifie Get-Etat contre la table partagée tests/cas_etat.csv ("-" = absent).
# Usage : powershell -NoProfile -File pc\test_etat.ps1
. "$PSScriptRoot\sync.ps1"

function Obs($hash, $taille) {
    if ($hash -eq '-') { return $null }
    @{ hash = $hash; taille = [long]$taille }
}

$echecs = 0
$cas = Import-Csv "$PSScriptRoot\..\tests\cas_etat.csv" -Delimiter ';' -Encoding UTF8
foreach ($c in $cas) {
    $base = if ($c.base -eq '-') { $null } else { $c.base }
    $obtenu = Get-Etat (Obs $c.local_hash $c.local_taille) (Obs $c.serveur_hash $c.serveur_taille) $base
    if ($obtenu -ne $c.attendu) {
        Write-Host "ECHEC  $($c.cas) : attendu $($c.attendu), obtenu $obtenu"
        $echecs++
    }
}
Write-Host "$($cas.Count - $echecs)/$($cas.Count) cas OK"
exit $echecs
