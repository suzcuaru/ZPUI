package database

import (
	"time"
)


// === Component Versions ===

// SaveComponentVersion сохраняет или обновляет версию компонента.
func SaveComponentVersion(v *ComponentVersion) error {
	if v.LocalUpdatedAt.IsZero() {
		v.LocalUpdatedAt = time.Now()
	}
	_, err := DB().Exec(`
		INSERT INTO component_versions (id, installed_version, remote_version, remote_source, remote_updated_at, local_updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			installed_version = excluded.installed_version,
			remote_version = COALESCE(excluded.remote_version, component_versions.remote_version),
			remote_source = COALESCE(excluded.remote_source, component_versions.remote_source),
			remote_updated_at = COALESCE(excluded.remote_updated_at, component_versions.remote_updated_at),
			local_updated_at = excluded.local_updated_at
	`, v.ID, v.InstalledVersion, v.RemoteVersion, v.RemoteSource, v.RemoteUpdatedAt, v.LocalUpdatedAt)
	return err
}

// UpsertComponentInstalled обновляет установленную версию компонента,
// не трогая remote-поля (используется, когда проверка обновлений не удалась).
func UpsertComponentInstalled(id, version string) error {
	_, err := DB().Exec(`
		INSERT INTO component_versions (id, installed_version, local_updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			installed_version = excluded.installed_version,
			local_updated_at = excluded.local_updated_at
	`, id, version, time.Now())
	return err
}

// InvalidateComponentRemote сбрасывает известную удалённую версию компонента:
// проверка не удалась, и строка не должна продолжать показывать протухший remote.
func InvalidateComponentRemote(id string) error {
	_, err := DB().Exec(`
		UPDATE component_versions
		SET remote_version = '', remote_source = '', remote_updated_at = NULL
		WHERE id = ?
	`, id)
	return err
}

// DeleteComponentVersion удаляет строку компонента (модуль удалён из продукта).
func DeleteComponentVersion(id string) error {
	_, err := DB().Exec(`DELETE FROM component_versions WHERE id = ?`, id)
	return err
}

// GetAllComponentVersions возвращает все версии компонентов.
func GetAllComponentVersions() ([]ComponentVersion, error) {
	rows, err := DB().Query(`
		SELECT id, installed_version, remote_version, remote_source, remote_updated_at, local_updated_at
		FROM component_versions ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []ComponentVersion
	for rows.Next() {
		var v ComponentVersion
		if err := rows.Scan(&v.ID, &v.InstalledVersion, &v.RemoteVersion, &v.RemoteSource, &v.RemoteUpdatedAt, &v.LocalUpdatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}
