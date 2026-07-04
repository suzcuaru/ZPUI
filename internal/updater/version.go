package updater

import (
	"strconv"
	"strings"
)

// normalizeVersion приводит строку версии к каноничному виду MAJOR.MINOR.PATCH:
// убирает пробелы, кавычки, 'v'/'V'-префикс и суффиксы (-rc, +build и т.п.).
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, `"`)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

// CompareVersions сравнивает две semver-подобные версии.
// Возвращает -1 если a < b, 0 если равны, 1 если a > b.
// Нечисловые/пустые сегаты трактуются как 0, поэтому сравнение не падает.
func CompareVersions(a, b string) int {
	a = normalizeVersion(a)
	b = normalizeVersion(b)
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var na, nb int
		if i < len(pa) {
			na, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			nb, _ = strconv.Atoi(pb[i])
		}
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
	}
	return 0
}

// IsNewer возвращает true, если latest строго новее current.
func IsNewer(current, latest string) bool {
	return CompareVersions(current, latest) < 0
}

// VersionsEqual возвращает true, если две версии эквивалентны после нормализации.
func VersionsEqual(a, b string) bool {
	return CompareVersions(a, b) == 0
}
