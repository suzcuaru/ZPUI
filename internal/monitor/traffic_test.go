package monitor

import "testing"

func TestTrimNonNumeric(t *testing.T) {
	cases := map[string]string{
		"123abc456": "123456",
		"1.2.3":     "1.2.3",
		"abc":       "",
		"":          "",
		"v1.0.27":   "1.0.27",
	}
	for in, want := range cases {
		if got := trimNonNumeric(in); got != want {
			t.Errorf("trimNonNumeric(%q) = %q, want %q", in, got, want)
		}
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
