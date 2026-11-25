//go:build !windows

package winterm

// SetBadge does nothing here, where discordo knows no taskbar to show it on.
func SetBadge(Badge) {}
