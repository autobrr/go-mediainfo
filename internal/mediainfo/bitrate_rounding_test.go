package mediainfo

import "testing"

func TestNormalizeAACBitRate(t *testing.T) {
	t.Parallel()

	tests := map[int64]int64{
		45_999:  45_999,
		46_000:  48_000,
		50_000:  48_000,
		50_001:  50_001,
		64_827:  66_150,
		67_196:  66_150,
		67_473:  66_150,
		67_474:  67_474,
		129_654: 132_300,
		132_309: 132_300,
		141_792: 144_000,
		189_375: 192_000,
		192_216: 192_000,
		251_111: 251_111,
	}
	for input, want := range tests {
		if got := normalizeAACBitRate(input); got != want {
			t.Errorf("normalizeAACBitRate(%d) = %d, want %d", input, got, want)
		}
	}
}
