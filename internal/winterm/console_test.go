//go:build !windows

package winterm

import "testing"

func TestRelaunchOutsideWindows(t *testing.T) {
	if OldConsole() || Relaunch() {
		t.Fatal("got an old console outside Windows")
	}
}
