package profilecard

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/arikawa/v3/state/store/defaultstore"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui/images"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// newTestModel returns a card for user 5, a member of guild 1 named "Nick" with two roles, logged in as user 2.
func newTestModel(t *testing.T) (Model, discord.User) {
	t.Helper()
	s := ningen.FromState(state.NewWithStore("", defaultstore.New()))
	user := discord.User{ID: 5, Username: "ola", DisplayName: "Ola"}
	joined := discord.NewTimestamp(time.Date(2024, 3, 7, 0, 0, 0, 0, time.UTC))
	if err := errors.Join(
		s.MyselfSet(discord.User{ID: 2}, false),
		s.GuildSet(&discord.Guild{ID: 1}, false),
		s.RoleSet(1, &discord.Role{ID: 10, Name: "Low", Position: 1}, false),
		s.RoleSet(1, &discord.Role{ID: 11, Name: "High", Position: 5}, false),
		s.MemberSet(1, &discord.Member{User: user, Nick: "Nick", RoleIDs: []discord.RoleID{10, 11}, Joined: joined}, false),
	); err != nil {
		t.Fatal(err)
	}

	m := NewModel(testConfig(t), s)
	m.user, m.guildID = user, 1
	return m, user
}

func cardText(m Model) string {
	var got strings.Builder
	header, body := m.lines()
	for _, span := range append(header, body...) {
		got.WriteString(span.Text)
	}
	return got.String()
}

func TestModelLines(t *testing.T) {
	t.Run("before the profile loads", func(t *testing.T) {
		m, user := newTestModel(t)
		got := cardText(m)
		for _, want := range []string{"Nick\n@ola", "Roles: High, Low", "Member since 7 Mar 2024", "Account created " + user.ID.Time().Format(dateFormat)} {
			if !strings.Contains(got, want) {
				t.Errorf("card %q lacks %q", got, want)
			}
		}
		if strings.Contains(got, "Mutual servers") {
			t.Errorf("card %q counts mutual servers before the profile loads", got)
		}
	})

	t.Run("loaded", func(t *testing.T) {
		m, user := newTestModel(t)
		profile := &Profile{User: profileUser{User: user}}
		profile.UserProfile.Bio, profile.UserProfile.Pronouns = "own bio", "he/him"
		profile.MutualGuilds = make([]struct {
			ID discord.GuildID `json:"id"`
		}, 3)
		m, _ = m.Update(loadedMsg{request: m.request, profile: profile})
		got := cardText(m)
		for _, want := range []string{"@ola · he/him", "own bio", "Mutual servers: 3"} {
			if !strings.Contains(got, want) {
				t.Errorf("card %q lacks %q", got, want)
			}
		}

		// The bio and pronouns set for the guild come first.
		profile.GuildMemberProfile = &struct {
			Bio      string       `json:"bio"`
			Pronouns string       `json:"pronouns"`
			Banner   discord.Hash `json:"banner"`
		}{Bio: "guild bio", Pronouns: "they/them"}
		if got := cardText(m); !strings.Contains(got, "guild bio") || !strings.Contains(got, "they/them") || strings.Contains(got, "own bio") {
			t.Errorf("card %q does not prefer the guild's bio and pronouns", got)
		}
	})

	t.Run("outside a guild", func(t *testing.T) {
		m, _ := newTestModel(t)
		m.guildID = 0
		got := cardText(m)
		if !strings.HasPrefix(got, "Ola\n") || strings.Contains(got, "Roles") || strings.Contains(got, "Member since") {
			t.Errorf("card %q shows guild details outside a guild", got)
		}
	})
}

func TestModelUpdate(t *testing.T) {
	t.Run("drops a replaced request", func(t *testing.T) {
		m, user := newTestModel(t)
		m.request = 2
		m, _ = m.Update(loadedMsg{request: 1, profile: &Profile{User: profileUser{User: user}}})
		if m.profile != nil {
			t.Fatal("the answer for a replaced request was kept")
		}
	})

	t.Run("closes on cancel", func(t *testing.T) {
		m, _ := newTestModel(t)
		_, command := m.Update(tcell.NewEventKey(tcell.KeyEscape, "", tcell.ModNone))
		if command == nil {
			t.Fatal("escape did not close the card")
		}
	})
}

func TestModelView(t *testing.T) {
	m, user := newTestModel(t)
	profile := &Profile{User: profileUser{User: user}}
	profile.UserProfile.Bio = "Lager klienter i terminalen."
	m, _ = m.Update(loadedMsg{request: m.request, profile: profile})

	width, height := Width, 16
	screen, err := tcell.NewTerminfoScreenFromTty(vt.NewMockTerm(vt.MockOptSize{X: vt.Col(width), Y: vt.Row(height)}))
	if err != nil {
		t.Fatal(err)
	}
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	m.View().Draw(screen, tview.Rectangle{Width: width, Height: height})

	var drawn strings.Builder
	for y := range height {
		for x := range width {
			str, _, _ := screen.Get(x, y)
			drawn.WriteString(str)
		}
		drawn.WriteByte('\n')
	}
	for _, want := range []string{"Profile", "Nick", "@ola", "Lager klienter i terminalen.", "Roles: High, Low"} {
		if !strings.Contains(drawn.String(), want) {
			t.Errorf("%q is not drawn in:\n%s", want, drawn.String())
		}
	}
}

func TestModelCloses(t *testing.T) {
	m, _ := newTestModel(t)
	area := tview.Rectangle{X: 10, Y: 5, Width: Width, Height: 12}
	view := m.View()

	for _, key := range []tview.KeyMsg{
		tcell.NewEventKey(tcell.KeyEscape, "", tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, "q", tcell.ModNone),
		tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone),
	} {
		if got := view.Handle(key, area); got != (CancelMsg{}) {
			t.Errorf("%s: got %#v, want the card closed", keybindName(key), got)
		}
	}

	inside := tview.MouseMsg{EventMouse: tcell.NewEventMouse(12, 6, tcell.ButtonPrimary, tcell.ModNone), Action: tview.MouseLeftDown}
	if got := view.Handle(inside, area); got != nil {
		t.Fatalf("a click inside the card got through: %#v", got)
	}
	outside := tview.MouseMsg{EventMouse: tcell.NewEventMouse(1, 1, tcell.ButtonPrimary, tcell.ModNone), Action: tview.MouseLeftDown}
	if got := view.Handle(outside, area); got == nil {
		t.Fatal("a click outside the card was dropped")
	}
	if _, command := m.Update(outside); command == nil {
		t.Fatal("a click outside the card did not close it")
	}
}

func keybindName(key tview.KeyMsg) string { return key.Name() }

func TestModelImages(t *testing.T) {
	m, user := newTestModel(t)
	profile := &Profile{User: profileUser{User: user}}
	profile.UserProfile.Banner = "abc"
	profile.UserProfile.ThemeColors = []discord.Color{0xFF0000, 0x00FF00}

	_, command := m.Update(loadedMsg{request: m.request, profile: profile})
	task, ok := command.(cmd.Task)
	if !ok {
		t.Fatal("the avatar and banner were not asked for")
	}
	request := task().(images.RequestedMsg)
	if _, ok := request.Squares[m.avatarURL()]; !ok {
		t.Errorf("the avatar %q was not asked for: %v", m.avatarURL(), request.Squares)
	}
	m.profile = profile
	if fit, ok := request.Fitted[m.bannerURL()]; !ok || fit.MaxCols > Width {
		t.Errorf("the banner %q was not asked for to fit the card: %v", m.bannerURL(), request.Fitted)
	}
	if !strings.Contains(m.bannerURL(), "/banners/5/abc.png") {
		t.Errorf("banner URL = %q", m.bannerURL())
	}
}

func TestNameplateURL(t *testing.T) {
	m, user := newTestModel(t)
	if m.nameplateURL() != "" {
		t.Fatal("a nameplate before the profile loads")
	}
	var profile Profile
	if err := json.Unmarshal([]byte(`{"user":{"id":"1","collectibles":{"nameplate":{"asset":"nameplates/flags/norway/","label":"Norway"}}}}`), &profile); err != nil {
		t.Fatal(err)
	}
	profile.User.User = user
	m.profile = &profile
	if got, want := m.nameplateURL(), "https://cdn.discordapp.com/assets/collectibles/nameplates/flags/norway/static.png"; got != want {
		t.Fatalf("nameplateURL() = %q, want %q", got, want)
	}
}
