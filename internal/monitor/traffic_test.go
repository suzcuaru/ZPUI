package monitor

import "testing"

// sample дважды подряд: вторая выборка не nil и счётчики не уменьшились —
// сессионный накопитель монотонен (guard от сбросов счётчиков интерфейсов).
func TestSampleSessionMonotonic(t *testing.T) {
	m := &TrafficMonitor{}
	first := m.sample()
	if first == nil {
		t.Skip("net interfaces unavailable")
	}
	second := m.sample()
	if second == nil {
		t.Fatal("second sample is nil")
	}
	if second.DownloadBytes < first.DownloadBytes || second.UploadBytes < first.UploadBytes {
		t.Errorf("session counters decreased: dl %d -> %d, ul %d -> %d",
			first.DownloadBytes, second.DownloadBytes, first.UploadBytes, second.UploadBytes)
	}
}

// counterDelta: обычная дельта, сброс счётчика (ложных гигабайт быть не
// должно) и переполнение через 2^32.
func TestCounterDelta(t *testing.T) {
	if got := counterDelta(100, 900); got != 800 {
		t.Errorf("normal delta = %d, want 800", got)
	}
	if got := counterDelta(1000, 1000); got != 0 {
		t.Errorf("zero delta = %d, want 0", got)
	}
	if got := counterDelta(500, 400); got != 0 {
		t.Errorf("counter reset = %d, want 0 (no spike)", got)
	}
	wrap := counterDelta(0xFFFFFFF0, 0x0000000F)
	if want := uint64(0x1F); wrap != want {
		t.Errorf("wrap delta = %d, want %d", wrap, want)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{1099511627776, "1.00 TB"},
	}
	for _, c := range cases {
		if got := FormatBytes(c.bytes); got != c.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}

func TestFormatSpeed(t *testing.T) {
	cases := []struct {
		bps  float64
		want string
	}{
		{0, "0 B/s"},
		{500, "500 B/s"},
		{1024, "1.00 KB/s"},
		{1048576, "1.00 MB/s"},
		{1073741824, "1.00 GB/s"},
	}
	for _, c := range cases {
		if got := FormatSpeed(c.bps); got != c.want {
			t.Errorf("FormatSpeed(%v) = %q, want %q", c.bps, got, c.want)
		}
	}
}
