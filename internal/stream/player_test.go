package stream

import (
	"runtime"
	"slices"
	"testing"
)

func TestPlatformArgs(t *testing.T) {
	got := PlatformArgs("mpv")
	if runtime.GOOS == "windows" {
		if !slices.Equal(got, []string{"--d3d11-flip=no"}) {
			t.Fatalf("PlatformArgs(mpv) = %v", got)
		}
	} else if got != nil {
		t.Fatalf("PlatformArgs(mpv) = %v, want none outside Windows", got)
	}
	if got := PlatformArgs("vlc"); got != nil {
		t.Fatalf("PlatformArgs(vlc) = %v, want none", got)
	}
}
