package updater

// Реестр источников обновлений (спека Документы/Задачи/2026-11-12-update-sources-zapret2.md).
// Режим приходит из config.updates_source; auto = приоритет сервера ZPUI Live,
// недоступен — остальные каналы по порядку (решение владельца 12.11.2026).

// Products — что качаем. Цепочки для zapret2 пока только серверные: на
// Яндекс.Диске zapret2 нет, а апстрим публикует лишь Linux-архивы (ghproxy
// для него бессмысленен до появления Windows-ассета).
const (
	ProductZPUI    = "zpui"
	ProductZapret  = "zapret"
	ProductZapret2 = "zapret2"
)

// Chain — порядок источников для продукта в выбранном режиме. Явный режим
// даёт единственный источник (фолбэка нет — владелец выбрал сам).
func Chain(mode, product string) []string {
	switch mode {
	case "server", "yandex", "github", "ghproxy":
		return []string{mode}
	}
	if product == ProductZapret2 {
		return []string{"server"}
	}
	return []string{"server", "yandex", "github"}
}

// ghProxyBases — зеркала GitHub (те же хосты, что использует сервер
// для зеркалирования релизов: ZPUI-Server core/zapretsync.go).
var ghProxyBases = []string{
	"https://gh-proxy.com/",
	"https://ghfast.top/",
}

// GitHubURLs — варианты URL для скачивания в режиме mode: github — прямо,
// ghproxy — через зеркала с финальным прямым ретраем.
func GitHubURLs(mode, rawURL string) []string {
	if mode == "ghproxy" && rawURL != "" {
		out := make([]string, 0, len(ghProxyBases)+1)
		for _, b := range ghProxyBases {
			out = append(out, b+rawURL)
		}
		out = append(out, rawURL)
		return out
	}
	if rawURL == "" {
		return nil
	}
	return []string{rawURL}
}
