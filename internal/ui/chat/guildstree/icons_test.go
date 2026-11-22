package guildstree

import (
	"testing"

	"github.com/ayn2op/tview/text"
)

func TestNameIndex(t *testing.T) {
	if got := nameIndex([]text.Span{{Text: "name"}, {Text: " 1/5"}}); got != 0 {
		t.Fatalf("nameIndex without icons = %d, want 0", got)
	}
	if got := nameIndex(nil); got != 0 {
		t.Fatalf("nameIndex(nil) = %d, want 0", got)
	}
}
