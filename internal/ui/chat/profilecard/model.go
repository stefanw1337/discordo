// Package profilecard shows the profile of a user: their names, pronouns, bio, roles and dates.
package profilecard

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/keybind"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
)

// dateFormat is how the dates of a profile are shown.
const dateFormat = "2 Jan 2006"

// Width is the number of columns the card takes, borders and padding included.
const Width = 50

type Model struct {
	cfg   *config.Config
	state *ningen.State

	// user is whose profile is shown, as known before it loads, in guildID if valid.
	user    discord.User
	guildID discord.GuildID
	// profile is what Discord answered, or nil while it loads or if it failed with err.
	profile *Profile
	err     error
	// request numbers the profiles asked for, so that the answer for one replaced by another is dropped.
	request int
}

func NewModel(cfg *config.Config, state *ningen.State) Model {
	return Model{cfg: cfg, state: state}
}

// Open shows the profile of user, as a member of guildID if it is valid, and asks Discord for the rest of it.
func (m *Model) Open(user discord.User, guildID discord.GuildID) tview.Cmd {
	m.request++
	m.user, m.guildID, m.profile, m.err = user, guildID, nil, nil
	request, client := m.request, m.state.Client
	return cmd.Task(func() tview.Msg {
		profile, err := fetch(client, user.ID, guildID)
		return loadedMsg{request: request, profile: profile, err: err}
	})
}

// Profile is the part of Discord's answer to GET /users/{id}/profile that the card shows.
type Profile struct {
	User        discord.User `json:"user"`
	UserProfile struct {
		Bio      string `json:"bio"`
		Pronouns string `json:"pronouns"`
	} `json:"user_profile"`
	// GuildMemberProfile is the bio and pronouns set for the guild, which come before the user's own.
	GuildMemberProfile *struct {
		Bio      string `json:"bio"`
		Pronouns string `json:"pronouns"`
	} `json:"guild_member_profile"`
	MutualGuilds []struct {
		ID discord.GuildID `json:"id"`
	} `json:"mutual_guilds"`
}

// fetch asks Discord for the profile of userID, as a member of guildID if it is valid.
func fetch(client *api.Client, userID discord.UserID, guildID discord.GuildID) (*Profile, error) {
	endpoint := api.EndpointUsers + userID.String() + "/profile?with_mutual_guilds=true"
	if guildID.IsValid() {
		endpoint += "&guild_id=" + guildID.String()
	}
	return client.RequestJSON[*Profile]("GET", endpoint)
}

type loadedMsg struct {
	request int
	profile *Profile
	err     error
}

// OpenMsg asks to show the profile of User, as a member of GuildID if it is valid.
type OpenMsg struct {
	User    discord.User
	GuildID discord.GuildID
}

// CancelMsg asks to close the card.
type CancelMsg struct{}

var _ tview.Model[Model] = Model{}

func (Model) Init() tview.Cmd { return nil }

func (m Model) Update(msg tview.Msg) (Model, tview.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		if msg.request == m.request {
			m.profile, m.err = msg.profile, msg.err
		}
	case tview.KeyMsg:
		keybinds := m.cfg.Keybinds.Picker
		if keybind.Matches(msg, keybinds.Cancel.Keybind, keybinds.Select.Keybind) {
			return m, cmd.Task(func() tview.Msg { return CancelMsg{} })
		}
	}
	return m, nil
}

// View shows the profile in a box.
func (m Model) View() tview.Widget {
	footer := ""
	switch {
	case m.err != nil:
		footer = "Could not load the full profile"
	case m.profile == nil:
		footer = "Loading..."
	}

	content := text.Rich(m.lines()...)
	return ui.Box(content, m.cfg, true).
		Title(ui.Label("Profile", m.cfg.Theme.Title, true)).
		Footer(ui.Label(footer, m.cfg.Theme.Footer, true))
}

// lines returns the spans of the card's text, one paragraph after another.
func (m Model) lines() []text.Span {
	user := m.user
	if m.profile != nil && m.profile.User.ID.IsValid() {
		user = m.profile.User
	}

	member := m.member()
	var builder markdown.Builder
	name := user.DisplayOrUsername()
	if member != nil && member.Nick != "" {
		name = member.Nick
	}
	builder.Write(name, m.nameStyle(member))
	if user.Bot {
		builder.Write(" BOT", tcell.StyleDefault.Reverse(true))
	}

	builder.NewLine()
	dim := tcell.StyleDefault.Dim(true)
	handle := "@" + user.Username
	if pronouns := m.pronouns(); pronouns != "" {
		handle += " · " + pronouns
	}
	builder.Write(handle, dim)

	if bio := m.bio(); bio != "" {
		builder.NewLine()
		builder.NewLine()
		builder.Write(bio, tcell.StyleDefault)
	}

	if roles := m.roleNames(member); len(roles) != 0 {
		builder.NewLine()
		builder.NewLine()
		builder.Write("Roles: ", dim)
		builder.Write(strings.Join(roles, ", "), tcell.StyleDefault)
	}

	builder.NewLine()
	builder.NewLine()
	if member != nil && member.Joined.IsValid() {
		builder.Write("Member since ", dim)
		builder.Write(member.Joined.Time().Format(dateFormat), tcell.StyleDefault)
		builder.NewLine()
	}
	builder.Write("Account created ", dim)
	builder.Write(user.ID.Time().Format(dateFormat), tcell.StyleDefault)
	if m.profile != nil && !ui.IsMe(m.state, user.ID) {
		builder.NewLine()
		builder.Write("Mutual servers: ", dim)
		builder.Write(strconv.Itoa(len(m.profile.MutualGuilds)), tcell.StyleDefault)
	}

	return builder.Finish()
}

// member returns the user as a member of the guild, or nil outside of one or if they are not known there.
func (m Model) member() *discord.Member {
	if !m.guildID.IsValid() {
		return nil
	}
	member, err := m.state.Cabinet.Member(m.guildID, m.user.ID)
	if err != nil {
		return nil
	}
	return member
}

// nameStyle returns the style of the name: bold, in the color of member's roles if it has one.
func (m Model) nameStyle(member *discord.Member) tcell.Style {
	style := tcell.StyleDefault.Bold(true)
	if member == nil {
		return style
	}

	color, ok := state.MemberColor(member, func(id discord.RoleID) *discord.Role {
		role, _ := m.state.Cabinet.Role(m.guildID, id)
		return role
	})
	if ok {
		style = style.Foreground(tcell.NewHexColor(int32(color)))
	}
	return style
}

// bio returns the bio set for the guild, or else the user's own, or "" before the profile loads.
func (m Model) bio() string {
	if m.profile == nil {
		return ""
	}
	if guild := m.profile.GuildMemberProfile; guild != nil && guild.Bio != "" {
		return strings.TrimSpace(guild.Bio)
	}
	return strings.TrimSpace(m.profile.UserProfile.Bio)
}

// pronouns returns the pronouns set for the guild, or else the user's own, or "" before the profile loads.
func (m Model) pronouns() string {
	if m.profile == nil {
		return ""
	}
	if guild := m.profile.GuildMemberProfile; guild != nil && guild.Pronouns != "" {
		return guild.Pronouns
	}
	return m.profile.UserProfile.Pronouns
}

// roleNames returns the names of member's roles, highest first.
func (m Model) roleNames(member *discord.Member) []string {
	if member == nil {
		return nil
	}

	var roles []discord.Role
	for _, id := range member.RoleIDs {
		if role, err := m.state.Cabinet.Role(m.guildID, id); err == nil {
			roles = append(roles, *role)
		}
	}
	slices.SortFunc(roles, func(a, b discord.Role) int { return cmp.Compare(b.Position, a.Position) })

	names := make([]string, len(roles))
	for i, role := range roles {
		names[i] = role.Name
	}
	return names
}
