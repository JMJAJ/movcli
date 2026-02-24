$ErrorActionPreference = 'Stop'
$exePath = "$env:TEMP\movcli.exe"
Write-Host "🍿 Grabbing the popcorn (Loading MovCLI)..." -ForegroundColor Cyan
$url = (irm https://api.github.com/repos/JMJAJ/movcli/releases/latest).assets[0].browser_download_url
Invoke-RestMethod -Uri $url -OutFile $exePath -UseBasicParsing
Clear-Host
& $exePath