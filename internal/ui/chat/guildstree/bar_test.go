package guildstree

import "testing"

func TestBadgeLabel(t *testing.T) {
	for _, c := range []struct {
		entry barEntry
		want  string
	}{
		{barEntry{mentions: 3}, " 3 "},
		{barEntry{mentions: 120}, " 99+ "},
		{barEntry{unread: true}, "new"},
		{barEntry{unread: true, open: true}, ""},
		{barEntry{}, ""},
	} {
		if got := badgeLabel(c.entry); got != c.want {
			t.Errorf("badgeLabel(%+v) = %q, want %q", c.entry, got, c.want)
		}
	}
}
