package composer

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDroppedFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "SC_in_WoW.mp4")
	b := filepath.Join(dir, "with space.png")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := droppedFiles(a + "\r"); !slices.Equal(got, []string{a}) {
		t.Errorf("one file: %v", got)
	}
	if got := droppedFiles(a + ` "` + b + `"`); !slices.Equal(got, []string{a, b}) {
		t.Errorf("two files: %v", got)
	}
	for _, text := range []string{"hello", a + " and more", filepath.Join(dir, "missing.mp4"), dir, "SC_in_WoW.mp4"} {
		if got := droppedFiles(text); got != nil {
			t.Errorf("droppedFiles(%q) = %v", text, got)
		}
	}
}
