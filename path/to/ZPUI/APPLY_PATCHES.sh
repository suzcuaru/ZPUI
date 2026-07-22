#!/bin/bash
# ZPUI Fixes — Auto-applier
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_DIR="$SCRIPT_DIR/backup/$(date +%Y%m%d_%H%M%S)"

echo "=== ZPUI Fix Patcher ==="
echo "Project root: $PROJECT_ROOT"
echo "Backup dir:   $BACKUP_DIR"

mkdir -p "$BACKUP_DIR"
mkdir -p "$PROJECT_ROOT/internal/reports"

copy_backup() {
    local src="$PROJECT_ROOT/$1"
    local dst="$BACKUP_DIR/$1"
    if [ -f "$src" ]; then
        mkdir -p "$(dirname "$dst")"
        cp "$src" "$dst"
        echo "  backup: $1"
    fi
}

echo "[1/3] Creating backups..."
for f in \
    internal/app/app.go \
    internal/xboxdns/manager.go \
    internal/singleinstance/singleinstance.go \
    internal/tray/tray.go \
    internal/autoselect/autoselect.go \
    internal/config/config.go \
    internal/database/db.go \
    internal/database/models.go \
    internal/database/queries.go \
    internal/logger/logger.go \
    internal/monitor/traffic.go \
    web/src/api.js \
    web/src/pages/DashboardPage.jsx \
    web/src/pages/SettingsPage.jsx \
    web/src/hooks/useDebouncedSave.js; do
    copy_backup "$f"
done

echo ""
echo "[2/3] Applying replacements..."
cp_f() {
    local src="$1" dst="$PROJECT_ROOT/$2"
    if [ -f "$src" ]; then
        mkdir -p "$(dirname "$dst")"
        cp "$src" "$dst"
        echo "  applied: $2"
    else
        echo "  MISSING: $src"
    fi
}

cp_f "$SCRIPT_DIR/replacements/internal/app/app.go" "internal/app/app.go"
cp_f "$SCRIPT_DIR/replacements/internal/xboxdns/manager.go" "internal/xboxdns/manager.go"
cp_f "$SCRIPT_DIR/replacements/internal/singleinstance/singleinstance.go" "internal/singleinstance/singleinstance.go"
cp_f "$SCRIPT_DIR/replacements/internal/tray/tray.go" "internal/tray/tray.go"
cp_f "$SCRIPT_DIR/replacements/internal/autoselect/autoselect.go" "internal/autoselect/autoselect.go"
cp_f "$SCRIPT_DIR/replacements/internal/config/config.go" "internal/config/config.go"
cp_f "$SCRIPT_DIR/replacements/internal/database/db.go" "internal/database/db.go"
cp_f "$SCRIPT_DIR/replacements/internal/database/models.go" "internal/database/models.go"
cp_f "$SCRIPT_DIR/replacements/internal/database/queries.go" "internal/database/queries.go"
cp_f "$SCRIPT_DIR/replacements/internal/logger/logger.go" "internal/logger/logger.go"
cp_f "$SCRIPT_DIR/replacements/internal/monitor/traffic.go" "internal/monitor/traffic.go"
cp_f "$SCRIPT_DIR/replacements/web/src/api.js" "web/src/api.js"
cp_f "$SCRIPT_DIR/replacements/web/src/pages/DashboardPage.jsx" "web/src/pages/DashboardPage.jsx"
cp_f "$SCRIPT_DIR/replacements/web/src/pages/SettingsPage.jsx" "web/src/pages/SettingsPage.jsx"
cp_f "$SCRIPT_DIR/replacements/web/src/hooks/useDebouncedSave.js" "web/src/hooks/useDebouncedSave.js"

echo ""
echo "[3/3] Adding new files (reports system)..."
cp_f "$SCRIPT_DIR/new_files/internal/reports/generator.go" "internal/reports/generator.go"
cp_f "$SCRIPT_DIR/new_files/internal/reports/scheduler.go" "internal/reports/scheduler.go"
cp_f "$SCRIPT_DIR/new_files/internal/reports/uploader.go" "internal/reports/uploader.go"
cp_f "$SCRIPT_DIR/new_files/internal/app/app_api_reports.go" "internal/app/app_api_reports.go"

echo ""
echo "=== Done! All patches applied. ==="
echo "Backup: $BACKUP_DIR"
echo "To revert: cp -r $BACKUP_DIR/* $PROJECT_ROOT/"