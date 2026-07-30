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
//
// Поддерживает буквенные суффиксы сегментов, которые использует zapret
// (например 1.9.9a, 1.9.9d): каждый сегмент разбивается на числовую часть
// и остаток-суффикс. Сначала сравнивается число, при равенстве — суффикс
// лексикографически (пустой суффикс меньше любого непустого, поэтому
// «9» < «9a» < «9b»). Без этого 1.9.9a и 1.9.9d ошибочно считались равными,
// т.к. strconv.Atoi("9a") падает и возвращает 0.
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
		var sa, sb string
		if i < len(pa) {
			sa = pa[i]
		}
		if i < len(pb) {
			sb = pb[i]
		}
		na, sfa := parseSegment(sa)
		nb, sfb := parseSegment(sb)
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
		if c := strings.Compare(sfa, sfb); c != 0 {
			return c
		}
	}
	return 0
}

// parseSegment разбивает сегмент версии на числовую часть и суффикс.
// "9a" -> (9, "a"), "10" -> (10, ""), "rc1" -> (0, "rc1"), "" -> (0, "").
func parseSegment(s string) (int, string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	num, _ := strconv.Atoi(s[:i])
	return num, s[i:]
}

// IsNewer возвращает true, если latest строго новее current.
func IsNewer(current, latest string) bool {
	return CompareVersions(current, latest) < 0
}

// VersionsEqual возвращает true, если две версии эквивалентны после нормализации.
func VersionsEqual(a, b string) bool {
	return CompareVersions(a, b) == 0
}

// IsDevVersion возвращает true, если версия содержит 4+ сегментов и последний != 0.
// Стабильные версии: 3 сегмента (1.5.5) или 4 сегмента с 0 (1.5.5.0).
// Dev-версии: 4+ сегмента с последним > 0 (1.5.5.1, 1.5.5.2).
func IsDevVersion(version string) bool {
	v := normalizeVersion(version)
	parts := strings.Split(v, ".")
	if len(parts) < 4 {
		return false
	}
	return parts[len(parts)-1] != "0"
}

// VersionBranch возвращает "dev" для dev-версий (4+ сегментов) и "stable" для остальных.
func VersionBranch(version string) string {
	if IsDevVersion(version) {
		return "dev"
	}
	return "stable"
}

// SourcePreference решает, откуда скачивать обновление, сравнивая версии
// GitHub и Яндекс.Диска. Возвращает "yandex" или "github".
// Правила:
//   - если доступен только один источник — выбирается он;
//   - при равенстве версий или если Яндекс новее — выбирается "yandex";
//   - GitHub выбирается только когда строго новее Яндекса.
//
// Такой приоритет Яндекса связан с тем, что в регионе приложения он
// стабильнее и быстрее GitHub (который может блокироваться DPI).
func SourcePreference(githubVer, yandexVer string) string {
	gh := normalizeVersion(githubVer)
	ya := normalizeVersion(yandexVer)
	if ya == "" {
		return "github"
	}
	if gh == "" {
		return "yandex"
	}
	if CompareVersions(ya, gh) >= 0 {
		return "yandex"
	}
	return "github"
}
