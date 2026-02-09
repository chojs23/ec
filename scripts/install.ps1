Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$App = "ec"
$Alias = "easy-conflict"
$Owner = "chojs23"
$Repo = "ec"

if ([string]::IsNullOrWhiteSpace($env:PREFIX)) {
    $Prefix = Join-Path $HOME ".local"
} else {
    $Prefix = $env:PREFIX
}

$BinDir = Join-Path $Prefix "bin"
$Version = if ([string]::IsNullOrWhiteSpace($env:VERSION)) { "latest" } else { $env:VERSION }

$archName = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
switch ($archName.ToUpperInvariant()) {
    "AMD64" { $Arch = "amd64" }
    "ARM64" { $Arch = "arm64" }
    default { throw "Unsupported architecture: $archName" }
}

$Asset = "$App-windows-$Arch.exe"

if ($Version -eq "latest") {
    $BaseUrl = "https://github.com/$Owner/$Repo/releases/latest/download"
    $TagLabel = "latest"
} else {
    if ($Version.StartsWith("v")) {
        $Tag = $Version
    } else {
        $Tag = "v$Version"
    }
    $BaseUrl = "https://github.com/$Owner/$Repo/releases/download/$Tag"
    $TagLabel = $Tag
}

$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $TempDir | Out-Null

try {
    $AssetPath = Join-Path $TempDir $Asset
    $ChecksumsPath = Join-Path $TempDir "checksums.txt"

    Invoke-WebRequest -Uri "$BaseUrl/$Asset" -OutFile $AssetPath
    Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $ChecksumsPath

    $expectedChecksum = $null
    foreach ($line in Get-Content -Path $ChecksumsPath) {
        $parts = $line.Trim() -split '\s+', 2
        if ($parts.Count -eq 2 -and $parts[1] -eq $Asset) {
            $expectedChecksum = $parts[0].ToLowerInvariant()
            break
        }
    }

    if ([string]::IsNullOrWhiteSpace($expectedChecksum)) {
        throw "Checksum not found for $Asset in $TagLabel"
    }

    $actualChecksum = (Get-FileHash -Path $AssetPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expectedChecksum -ne $actualChecksum) {
        throw "Checksum mismatch for $Asset"
    }

    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null

    $appTarget = Join-Path $BinDir "$App.exe"
    $aliasTarget = Join-Path $BinDir "$Alias.exe"

    Copy-Item -Path $AssetPath -Destination $appTarget -Force
    Copy-Item -Path $appTarget -Destination $aliasTarget -Force

    Write-Host "Installed $App to $appTarget"
    Write-Host "Installed $Alias to $aliasTarget"
} finally {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
