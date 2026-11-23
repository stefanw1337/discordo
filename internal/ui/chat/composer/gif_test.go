package composer

import "testing"

func TestGIFQuery(t *testing.T) {
	for text, want := range map[string]string{"/gif": "", "/gif  happy cat ": "happy cat"} {
		if query, ok := gifQuery(text); !ok || query != want {
			t.Errorf("gifQuery(%q) = %q, %v", text, query, ok)
		}
	}
	for _, text := range []string{"/giffy", "a /gif", "gif cats"} {
		if _, ok := gifQuery(text); ok {
			t.Errorf("gifQuery(%q) took it for a command", text)
		}
	}
}
