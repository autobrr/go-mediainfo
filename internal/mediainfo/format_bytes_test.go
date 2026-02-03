package mediainfo

import "testing"

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		size int64
		want string
	}{
		{size: 0, want: "0 Bytes"},
		{size: 512, want: "512 Bytes"},
		{size: 1024, want: "1.00 KiB"},
		{size: 1024 * 10, want: "10.0 KiB"},
		{size: 1024 * 100, want: "100 KiB"},
		{size: 1024 * 1024, want: "1.00 MiB"},
	}

	for _, tc := range cases {
		if got := formatBytes(tc.size); got != tc.want {
			t.Fatalf("formatBytes(%d)=%q want %q", tc.size, got, tc.want)
		}
	}
}
