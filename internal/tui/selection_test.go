package tui

import "testing"

func TestNormalizeLogSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		s, e    int
		maxLen  int
		wantS   int
		wantE   int
	}{
		{"in range", 10, 50, 100, 10, 50},
		{"swap reversed", 50, 10, 100, 10, 50},
		{"clamp end", 10, 200, 100, 10, 100},
		{"clamp start past end", 230230, 240000, 229125, 229125, 229125},
		{"negative start", -5, 20, 100, 0, 20},
		{"empty content", 5, 10, 0, 0, 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotS, gotE := normalizeLogSelection(tt.s, tt.e, tt.maxLen)
			if gotS != tt.wantS || gotE != tt.wantE {
				t.Fatalf("normalizeLogSelection(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.s, tt.e, tt.maxLen, gotS, gotE, tt.wantS, tt.wantE)
			}
		})
	}
}
