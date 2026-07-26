package handler

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "patch newer", a: "v0.1.1", b: "v0.1.0", want: 1},
		{name: "minor numeric", a: "v0.10.0", b: "v0.2.0", want: 1},
		{name: "same", a: "v1.2.3", b: "v1.2.3", want: 0},
		{name: "dev older", a: "v0.1.0", b: "dev", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.a, tt.b)
			if tt.want > 0 && got <= 0 {
				t.Fatalf("compareVersions(%q, %q) = %d, want > 0", tt.a, tt.b, got)
			}
			if tt.want == 0 && got != 0 {
				t.Fatalf("compareVersions(%q, %q) = %d, want 0", tt.a, tt.b, got)
			}
		})
	}
}
