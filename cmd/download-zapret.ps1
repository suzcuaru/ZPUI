param(
    [Parameter(Mandatory=$true)]
    [string]$DistDir
)

$d = $DistDir
Write-Host '   [PS] Zapret downloader started...'
$tag = $null; $url = $null
try {
    $r = Invoke-RestMethod -Uri 'https://api.github.com/repos/Flowseal/zapret-discord-youtube/releases/latest' -Headers @{'Accept' = 'application/vnd.github+json'} -TimeoutSec 15 -ErrorAction Stop
    $tag = $r.tag_name
    Write-Host '   [PS] API tag: ' + $tag
    foreach ($a in $r.assets) {
        if ($a.name -match '\.zip$' -and $a.name -notmatch 'src|tiny|minimal') {
            $url = $a.browser_download_url
            Write-Host '   [PS] API asset: ' + $url
        }
    }
} catch {
    Write-Host '   [PS] API failed: ' + $_.Exception.Message
}

if (!$tag) {
    try {
        $resp = Invoke-WebRequest -Uri 'https://github.com/Flowseal/zapret-discord-youtube/releases/latest' -UseBasicParsing -MaximumRedirection 0 -ErrorAction SilentlyContinue
        $loc = $resp.Headers.Location
        if ($loc) {
            $tag = $loc -replace '.*/tag/', '' -replace '.*/releases/', ''
            Write-Host '   [PS] Redirect tag: ' + $tag
        } else {
            Write-Host '   [PS] No Location header'
        }
    } catch {
        Write-Host '   [PS] Redirect failed: ' + $_.Exception.Message
    }
}

if (!$tag) {
    Write-Host '   [PS] Trying known tags...'
    foreach ($try in @('1.10.0', '1.9.9d', '1.9.9')) {
        $test = 'https://github.com/Flowseal/zapret-discord-youtube/releases/tag/' + $try
        try {
            $r = Invoke-WebRequest -Uri $test -UseBasicParsing -TimeoutSec 10 -ErrorAction Stop
            if ($r.StatusCode -eq 200) { $tag = $try; Write-Host '   [PS] Found tag: ' + $try; break }
        } catch { }
    }
}

if (!$tag) {
    Write-Host '   [PS] Could not find zapret tag, skipping download.'
    exit 1
}

$ver = $tag -replace '^v', ''

if (!$url) {
    $candidates = @(
        'https://github.com/Flowseal/zapret-discord-youtube/releases/download/' + $tag + '/zapret-discord-youtube-' + $tag + '.zip',
        'https://github.com/Flowseal/zapret-discord-youtube/releases/download/' + $tag + '/zapret-discord-youtube-v' + $tag + '.zip',
        'https://github.com/Flowseal/zapret-discord-youtube/releases/download/' + $tag + '/zapret-discord-youtube-' + $ver + '.zip'
    )
    foreach ($u in $candidates) {
        try {
            Write-Host '   [PS] Trying URL: ' + $u
            $req = [System.Net.HttpWebRequest]::Create($u); $req.Method = 'HEAD'; $req.Timeout = 10000
            $rsp = $req.GetResponse()
            $code = [int]$rsp.StatusCode; $rsp.Close(); $rsp.Dispose()
            if ($code -eq 200 -or $code -eq 302) { $url = $u; Write-Host '   [PS] URL OK'; break }
        } catch {
            Write-Host '   [PS] URL fail: ' + $_.Exception.Message
        }
    }
}

if (!$url) {
    Write-Host '   [PS] Could not find download URL for zapret, skipping.'
    exit 1
}

Write-Host '   [PS] Downloading: ' + $url
$zipPath = Join-Path $d 'zapret-temp.zip'
try {
    $wc = New-Object System.Net.WebClient
    $wc.Headers.Add('User-Agent', 'ZPUI-Build')
    $wc.DownloadFile($url, $zipPath)
    Write-Host '   [PS] Download OK (' + (Get-Item $zipPath).Length + ' bytes)'
} catch {
    Write-Host '   [PS] Download failed: ' + $_.Exception.Message
    Remove-Item $zipPath -Force -ErrorAction SilentlyContinue
    exit 1
}

Write-Host '   [PS] Extracting...'
$dest1 = $d + '\zapret'
$dest2 = Join-Path (Split-Path $d -Parent) ('Zapret ' + $ver)

foreach ($dest in @($dest1, $dest2)) {
    if (Test-Path $dest) { Remove-Item -Path $dest -Recurse -Force -ErrorAction SilentlyContinue }
    New-Item -ItemType Directory -Path $dest -Force | Out-Null
}

Add-Type -Assembly 'System.IO.Compression.FileSystem'
$z = [System.IO.Compression.ZipFile]::OpenRead($zipPath)
$rootDir = $null
foreach ($e in $z.Entries) {
    $parts = $e.FullName.Split('/')
    if ($parts.Length -ge 2) {
        $r = $parts[0]
        if ($rootDir -eq $null) { $rootDir = $r } elseif ($r -ne $rootDir) { $rootDir = ''; break }
    }
}
$z.Dispose()

if ($rootDir) {
    Write-Host '   [PS] Zip root: ' + $rootDir
    [System.IO.Compression.ZipFile]::ExtractToDirectory($zipPath, $d + '\zapret-extract-temp')
    $src = Join-Path ($d + '\zapret-extract-temp') $rootDir
    foreach ($dest in @($dest1, $dest2)) {
        Copy-Item -Path ($src + '\*') -Destination $dest -Recurse -Force
    }
    Remove-Item -Path ($d + '\zapret-extract-temp') -Recurse -Force
} else {
    Write-Host '   [PS] Flat zip'
    foreach ($dest in @($dest1, $dest2)) {
        [System.IO.Compression.ZipFile]::ExtractToDirectory($zipPath, $dest)
    }
}

Remove-Item $zipPath -Force

[System.IO.File]::WriteAllText((Join-Path $dest1 'version.txt'), $ver, (New-Object Text.UTF8Encoding $false))
[System.IO.File]::WriteAllText((Join-Path $dest2 'version.txt'), $ver, (New-Object Text.UTF8Encoding $false))
Write-Host '   [PS] Zapret ' + $ver + ' installed to 2 locations'

# Generate checksums for all zapret locations
$targets = @()
$p1 = Join-Path $d 'zapret'
if (Test-Path (Join-Path $p1 'bin\winws.exe')) { $targets += $p1 }
$p3 = Join-Path (Split-Path $d -Parent) ('Zapret ' + $ver)
if (Test-Path (Join-Path $p3 'bin\winws.exe')) { $targets += $p3 }

foreach ($zp in $targets) {
    $exePath = Join-Path $zp 'bin\winws.exe'
    if (Test-Path $exePath) {
        $h = (Get-FileHash $exePath -Algorithm SHA256).Hash.ToLower()
        $sz = (Get-Item $exePath).Length
        $chkPath = Join-Path $zp 'checksum.sha256'
        [IO.File]::WriteAllText($chkPath, $h, (New-Object Text.UTF8Encoding $false))
        $name = [System.IO.Path]::GetFileName($zp)
        Write-Host ('   [OK] checksum.sha256 for ' + $name + ' (' + $sz + ' bytes)')
    }
}
