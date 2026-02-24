$ErrorActionPreference = 'Stop'
$exePath = "$env:TEMP\movcli.exe"
$url = "https://github.com/JMJAJ/movcli/releases/download/v1.0.0/movcli.exe"

Write-Host "🍿 Grabbing the popcorn (Loading MovCLI)..." -ForegroundColor Cyan
Invoke-RestMethod -Uri $url -OutFile $exePath -UseBasicParsing

Clear-Host

& $exePath