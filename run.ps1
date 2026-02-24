$ErrorActionPreference = 'Stop'
$exePath = "$env:TEMP\movcli.exe"
$url = "https://raw.githubusercontent.com/JMJAJ/movcli/main/install.ps1"

Write-Host "🍿 Grabbing the popcorn (Loading MovCLI)..." -ForegroundColor Cyan
Invoke-RestMethod -Uri $url -OutFile $exePath -UseBasicParsing

Clear-Host

& $exePath