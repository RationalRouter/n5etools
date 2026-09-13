package ingest

import "testing"

func TestNormalizeForCompare(t *testing.T) {
	got := normalizeForCompare("Armor,  Cost.\nPadded Cloth (100 Ryo)!")
	want := "armor cost padded cloth 100 ryo"
	if got != want {
		t.Errorf("normalizeForCompare() = %q, want %q", got, want)
	}
}

func TestTokenSimilarity(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{"identical", "armor cost padded cloth", "armor cost padded cloth", 1, 1},
		{"reordered still matches", "armor cost padded cloth", "padded cloth armor cost", 1, 1},
		{"both empty", "", "", 1, 1},
		{"one empty", "armor cost", "", 0, 0},
		{"totally different", "armor cost padded cloth", "weapon damage dice type", 0, 0},
		{"mostly overlapping", "armor cost padded cloth one hundred ryo", "armor cost padded cloth one hundrde ryo", 0.7, 0.99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenSimilarity(tt.a, tt.b)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("tokenSimilarity(%q, %q) = %v, want in [%v, %v]", tt.a, tt.b, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestExcerpt(t *testing.T) {
	if got := excerpt("short", 10); got != "short" {
		t.Errorf("excerpt short = %q", got)
	}
	if got := excerpt("this is a long string", 10); got != "this is a …" {
		t.Errorf("excerpt long = %q", got)
	}
}
