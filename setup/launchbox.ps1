# Fait passer les émulateurs MelonDS, Mgba et Azahar de LaunchBox par le Lanceur.
# Copie Emulators.xml avant toute modification. LaunchBox doit être fermé.
param([Parameter(Mandatory)][string]$Emulateurs, [Parameter(Mandatory)][string]$Lanceur)
$ErrorActionPreference = 'Stop'

Copy-Item $Emulateurs "$Emulateurs.avant-ndssync-$(Get-Date -Format yyyyMMdd-HHmmss)"
[xml]$x = Get-Content $Emulateurs -Raw -Encoding UTF8
$params = "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$Lanceur`""
$plateformes = @{
    'MelonDS' = @('Nintendo DS', 'Nintendo DSi')
    'Mgba'    = @('Nintendo Game Boy', 'Nintendo Game Boy Color', 'Nintendo Game Boy Advance')
    'Azahar'  = @('Nintendo 3DS')
}
foreach ($titre in $plateformes.Keys) {
    $emu = $x.LaunchBox.Emulator | Where-Object Title -eq $titre
    if (-not $emu) { "introuvable : $titre"; continue }
    $emu.ApplicationPath = 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe'
    $emu.HideConsole = 'true'
    $emu.CommandLine = ''  # le Lanceur ajoute lui-même -f (Azahar) et -C savegamePath (mGBA)
    $x.LaunchBox.EmulatorPlatform | Where-Object { $_.Emulator -eq $emu.ID -and $_.Platform -in $plateformes[$titre] } |
        ForEach-Object { $_.CommandLine = $params; "$titre -> $($_.Platform)" }
}
$x.Save($Emulateurs)
