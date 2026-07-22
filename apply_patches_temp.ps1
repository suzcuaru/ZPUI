$backupDir = "path\to\ZPUI\backup\" + (Get-Date -Format 'yyyyMMdd_HHmmss')
Write-Host "=== ZPUI Fix Patcher ==="
Write-Host "Backup dir: $backupDir"

New-Item -ItemType Directory -Path $backupDir -Force | Out-Null
New-Item -ItemType Directory -Path "internal\reports" -Force | Out-Null

Write-Host "[1/3] Creating backups..."
$files = @(
    "internal\app\app.go",
    "internal\xboxdns\manager.go",
    "internal\singleinstance\singleinstance.go",
    "internal\tray\tray.go",
    "internal\autoselect\autoselect.go",
    "internal\config\config.go",
    "internal\database\db.go",
    "internal\database\models.go",
    "internal\database\queries.go",
    "internal\logger\logger.go",
    "internal\monitor\traffic.go",
    "web\src\api.js",
    "web\src\pages\DashboardPage.jsx",
    "web\src\pages\SettingsPage.jsx",
    "web\src\hooks\useDebouncedSave.js"
)
foreach ($f in $files) {
    $src = Join-Path "." $f
    $dst = Join-Path $backupDir $f
    if (Test-Path $src) {
        $dir = Split-Path $dst -Parent
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        Copy-Item $src $dst
        Write-Host "  backup: $f"
    }
}

Write-Host "`n[2/3] Applying replacements..."
$replacements = @(
    "internal\app\app.go",
    "internal\xboxdns\manager.go",
    "internal\singleinstance\singleinstance.go",
    "internal\tray\tray.go",
    "internal\autoselect\autoselect.go",
    "internal\config\config.go",
    "internal\database\db.go",
    "internal\database\models.go",
    "internal\database\queries.go",
    "internal\logger\logger.go",
    "internal\monitor\traffic.go",
    "web\src\api.js",
    "web\src\pages\DashboardPage.jsx",
    "web\src\pages\SettingsPage.jsx",
    "web\src\hooks\useDebouncedSave.js"
)
foreach ($f in $replacements) {
    $src = Join-Path "path\to\ZPUI\replacements" $f
    $dst = Join-Path "." $f
    if (Test-Path $src) {
        $dir = Split-Path $dst -Parent
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        Copy-Item $src $dst
        Write-Host "  applied: $f"
    } else {
        Write-Host "  MISSING: $src"
    }
}

Write-Host "`n[3/3] Adding new files (reports system)..."
$newFiles = @(
    "internal\reports\generator.go",
    "internal\reports\scheduler.go",
    "internal\reports\uploader.go",
    "internal\app\app_api_reports.go"
)
foreach ($f in $newFiles) {
    $src = Join-Path "path\to\ZPUI\new_files" $f
    $dst = Join-Path "." $f
    if (Test-Path $src) {
        $dir = Split-Path $dst -Parent
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        Copy-Item $src $dst
        Write-Host "  added: $f"
    } else {
        Write-Host "  MISSING: $src"
    }
}

Write-Host "`n=== Done! All patches applied. ==="
Write-Host "Backup: $backupDir"
Write-Host "To revert: Copy-Item \"$backupDir\*\" . -Recurse"