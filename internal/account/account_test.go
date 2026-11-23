package account

import (
	"strings"
	"testing"
)

func TestChecklist(t *testing.T) {
	export := Export{
		User:    User{Username: "stefan", GlobalName: "Stefan"},
		Profile: Profile{Bio: "line one\nline two", Pronouns: "he/him"},
		Avatar:  "avatar.png",
		Servers: []Server{{Name: "Star Citizen", Invite: "https://discord.gg/starcitizen"}, {Name: "Small", Nickname: "stef"}},
		Friends: []Friend{{Username: "kari", GlobalName: "Kari", Nickname: "K"}},
		Groups:  []Group{{Members: []string{"kari", "ola"}}},
	}
	list := Checklist(export)
	for _, want := range []string{
		"- [ ] Display name: Stefan",
		"  > line one\n  > line two",
		"- [ ] Star Citizen: https://discord.gg/starcitizen",
		"- [ ] Small: ask a member for an invite (nickname stef)",
		"- [ ] kari (Kari), named K",
		"- [ ] kari, ola",
	} {
		if !strings.Contains(list, want) {
			t.Errorf("checklist lacks %q:\n%s", want, list)
		}
	}
	if got := safeName("a b/c:d"); got != "a_b_c_d" {
		t.Errorf("safeName = %q", got)
	}
}
