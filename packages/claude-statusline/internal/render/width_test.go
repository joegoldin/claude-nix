package render

import "testing"

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"plain ascii", "hello", 5},
		{"with ansi", Cyan("hello"), 5},
		{"nested ansi", Cyan(Dim("hi")), 2},
		{"with hyperlink", Hyperlink("u", "x"), 1},
		{"cjk counts double", "日本語", 6},
		{"emoji counts double", "🎤", 2},
		{"empty", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := VisibleWidth(tc.in); got != tc.want {
				t.Errorf("VisibleWidth(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("hello world", 5); got != "hell…" {
		t.Errorf("got %q want %q", got, "hell…")
	}
	if got := Truncate("hi", 5); got != "hi" {
		t.Errorf("got %q want %q", got, "hi")
	}
	// Truncation must close any open OSC 8 hyperlink.
	link := Hyperlink("https://example.com", "very long text goes here")
	got := Truncate(link, 5)
	wantSuffix := "\x1b]8;;\x1b\\"
	if len(got) < len(wantSuffix) || got[len(got)-len(wantSuffix):] != wantSuffix {
		t.Errorf("Truncate did not close hyperlink: got %q", got)
	}
}
