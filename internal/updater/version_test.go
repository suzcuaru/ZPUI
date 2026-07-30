package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.9.9a", "1.9.9d", -1},
		{"1.9.9d", "1.9.9a", 1},
		{"1.9.9a", "1.9.9a", 0},
		{"1.9.9a", "1.10.0", -1},
		{"1.10.0", "1.9.9d", 1},
		{"1.5.3", "1.5.3", 0},
		{"3.0.8", "3.0.7", 1},
		{"3.0.7", "3.0.8", -1},
		{"1.9.9", "1.9.9a", -1},
		{"1.9.9a", "1.9.9", 1},
		{"1.9.9d", "1.9.10", -1},
		{"v1.9.9a", "1.9.9d", -1},
		{"1.9.9a-rc", "1.9.9d", -1},
		{"", "", 0},
		{"unknown", "1.9.9d", -1},
	}
	for _, c := range cases {
		got := CompareVersions(c.a, c.b)
		if got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("1.9.9a", "1.9.9d") {
		t.Error("IsNewer(1.9.9a, 1.9.9d) should be true (letter suffix bug regression)")
	}
	if IsNewer("1.9.9d", "1.9.9a") {
		t.Error("IsNewer(1.9.9d, 1.9.9a) should be false")
	}
	if !IsNewer("1.9.9a", "1.10.0") {
		t.Error("IsNewer(1.9.9a, 1.10.0) should be true")
	}
	if IsNewer("1.5.3", "1.5.3") {
		t.Error("IsNewer(1.5.3, 1.5.3) should be false")
	}
}

func TestSourcePreference(t *testing.T) {
	cases := []struct {
		gh, ya string
		want   string
	}{
		{"1.9.9d", "1.9.9d", "yandex"},  // равны → Яндекс
		{"1.10.0", "1.9.9d", "github"},  // GitHub строго новее → GitHub
		{"1.9.9d", "1.10.0", "yandex"},  // Яндекс новее → Яндекс
		{"1.9.9a", "1.9.9d", "yandex"},  // буквенные суффиксы: Яндекс новее
		{"", "1.9.9d", "yandex"},        // только Яндекс
		{"1.9.9d", "", "github"},        // только GitHub
	}
	for _, c := range cases {
		got := SourcePreference(c.gh, c.ya)
		if got != c.want {
			t.Errorf("SourcePreference(%q, %q) = %q, want %q", c.gh, c.ya, got, c.want)
		}
	}
}
