// Package profilecard shows the profile of a user: their names, pronouns, bio, roles and dates.
package profilecard

import (
	"cmp"
	"fmt"
	"image"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/discordo/internal/ui/images"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/column"
	"github.com/ayn2op/tview/keybind"
	"github.com/ayn2op/tview/layout"
	"github.com/ayn2op/tview/row"
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
	// images are the chat model's, which loads them, by URL.
	images map[discord.URL]images.Widget
	// links are where the card's links were drawn.
	links links
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
	m.links = links{}
	request, client := m.request, m.state.Client
	return cmd.Batch{m.loadImages(), cmd.Task(func() tview.Msg {
		profile, err := fetch(client, user.ID, guildID)
		return loadedMsg{request: request, profile: profile, err: err}
	})}
}

const (
	// avatarRows is how tall the avatar is drawn, beside the names.
	avatarRows = 4
	// bannerRows is the most rows the banner takes, and accentRows the rows of the accent color without one.
	bannerRows = 8
	accentRows = 2
	// bannerSize is the size of a banner in pixels, 5 by 2.
	bannerWidth, bannerHeight = 600, 240
	// nameplateRows is the most rows the nameplate takes, under the names, and nameplateSize its usual size.
	nameplateRows                   = 3
	nameplateCols                   = 14
	nameplateWidth, nameplateHeight = 600, 116
	// innerWidth is the width inside the card's borders and padding.
	innerWidth = Width - 4
	// cdn is where Discord keeps avatars and banners.
	cdn = "https://cdn.discordapp.com"
)

// avatarURL returns the avatar of the user, the one set for the guild if any.
func (m Model) avatarURL() discord.URL {
	if member := m.member(); member != nil && member.Avatar != "" {
		return fmt.Sprintf("%s/guilds/%s/users/%s/avatars/%s.png?size=128", cdn, m.guildID, m.user.ID, member.Avatar)
	}
	return m.user.AvatarURLWithType(discord.PNGImage) + "?size=128"
}

// bannerURL returns the banner of the user, the one set for the guild if any, or "" if they have none.
func (m Model) bannerURL() discord.URL {
	if m.profile == nil {
		return ""
	}
	userID := m.user.ID
	if guild := m.profile.GuildMemberProfile; guild != nil && guild.Banner != "" {
		return fmt.Sprintf("%s/guilds/%s/users/%s/banners/%s.png?size=600", cdn, m.guildID, userID, guild.Banner)
	}
	banner := m.profile.UserProfile.Banner
	if banner == "" {
		banner = m.profile.User.Banner
	}
	if banner == "" {
		return ""
	}
	return fmt.Sprintf("%s/banners/%s/%s.png?size=600", cdn, userID, banner)
}

// nameplateURL returns the still picture of the user's nameplate, or "" if they wear none.
func (m Model) nameplateURL() discord.URL {
	if m.profile == nil || m.profile.User.Collectibles.Nameplate == nil || m.profile.User.Collectibles.Nameplate.Asset == "" {
		return ""
	}
	return cdn + "/assets/collectibles/" + m.profile.User.Collectibles.Nameplate.Asset + "static.png"
}

func (m Model) hasImage(url discord.URL) bool {
	_, ok := m.images[url]
	return ok
}

// loadImages asks for the avatar and banner not loaded yet.
func (m Model) loadImages() tview.Cmd {
	request := images.RequestedMsg{Squares: map[discord.URL]uint{}, Fitted: map[discord.URL]images.Fit{}}
	if url := m.avatarURL(); !m.hasImage(url) {
		request.Squares[url] = avatarRows
	}
	if url := m.bannerURL(); url != "" && !m.hasImage(url) {
		request.Fitted[url] = images.Fit{Size: image.Pt(bannerWidth, bannerHeight), MaxCols: innerWidth, MaxRows: bannerRows, Fill: true}
	}
	if url := m.nameplateURL(); url != "" && !m.hasImage(url) {
		request.Fitted[url] = images.Fit{Size: image.Pt(nameplateWidth, nameplateHeight), MaxCols: nameplateCols, MaxRows: nameplateRows, Mirror: true}
	}
	if len(request.Squares) == 0 && len(request.Fitted) == 0 {
		return nil
	}
	return cmd.Task(func() tview.Msg { return request })
}

// Profile is the part of Discord's answer to GET /users/{id}/profile that the card shows.
type Profile struct {
	User        profileUser `json:"user"`
	UserProfile struct {
		Bio      string       `json:"bio"`
		Pronouns string       `json:"pronouns"`
		Banner   discord.Hash `json:"banner"`
		// AccentColor is shown in place of a banner, and ThemeColors, of a Nitro profile theme, around the card.
		AccentColor discord.Color   `json:"accent_color"`
		ThemeColors []discord.Color `json:"theme_colors"`
	} `json:"user_profile"`
	// GuildMemberProfile is the bio, pronouns and banner set for the guild, which come before the user's own.
	GuildMemberProfile *struct {
		Bio      string       `json:"bio"`
		Pronouns string       `json:"pronouns"`
		Banner   discord.Hash `json:"banner"`
	} `json:"guild_member_profile"`
	MutualGuilds []struct {
		ID discord.GuildID `json:"id"`
	} `json:"mutual_guilds"`
}

// profileUser is the user of a profile, with the collectibles they wear.
type profileUser struct {
	discord.User
	Collectibles struct {
		// Nameplate is the picture shown behind their name in member lists, a path under the CDN's collectibles.
		Nameplate *struct {
			Asset string `json:"asset"`
			Label string `json:"label"`
		} `json:"nameplate"`
	} `json:"collectibles"`
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
			return m, m.loadImages()
		}
	case images.ChangedMsg:
		m.images = msg
	case tview.KeyMsg:
		if m.closes(msg) {
			return m, cmd.Task(func() tview.Msg { return CancelMsg{} })
		}
	case tview.MouseMsg:
		// A click outside the card, which the card itself lets through, closes it.
		if msg.Action == tview.MouseLeftDown {
			return m, cmd.Task(func() tview.Msg { return CancelMsg{} })
		}
	}
	return m, nil
}

// closes reports whether key closes the card: the picker's cancel or select keybind, or q.
func (m Model) closes(key tview.KeyMsg) bool {
	keybinds := m.cfg.Keybinds.Picker
	return keybind.Matches(key, keybinds.Cancel.Keybind, keybinds.Select.Keybind) || key.Key() == tcell.KeyEscape || key.Str() == "q"
}

// card is the profile card's widget. It closes the card on its keys itself, as a picker does, and keeps clicks
// inside it from reaching what is behind it, so that only a click outside closes it.
type card struct {
	tview.Widget
	model Model
}

// OpenLinkMsg asks to open a link clicked on the profile.
type OpenLinkMsg struct{ URL string }

// Draw draws the card, noting where its links are, for a click on one to open it.
func (w card) Draw(screen tview.Screen, area tview.Rectangle) {
	w.Widget.Draw(screen, area)
	if w.model.links == nil {
		return
	}
	clear(w.model.links)
	for y := area.Y; y < area.Y+area.Height; y++ {
		for x := area.X; x < area.X+area.Width; x++ {
			if _, style, _ := screen.Get(x, y); style != (tview.Style{}) {
				if _, url := style.GetUrl(); url != "" {
					w.model.links[[2]int{x, y}] = url
				}
			}
		}
	}
}

func (w card) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg {
	switch msg := msg.(type) {
	case tview.KeyMsg:
		if w.model.closes(msg) {
			return CancelMsg{}
		}
	case tview.MouseMsg:
		if area.Contains(msg.Position()) {
			if msg.Action == tview.MouseLeftClick && w.model.links != nil {
				x, y := msg.Position()
				if url, ok := w.model.links[[2]int{x, y}]; ok {
					return OpenLinkMsg{URL: url}
				}
			}
			return nil
		}
	}
	return w.Widget.Handle(msg, area)
}

// links are where the links of the card were drawn last, shared by the copies of the model.
type links map[[2]int]string

// writeLinked writes text in style, its links as links, which a click opens.
func writeLinked(builder *markdown.Builder, text string, style tcell.Style) {
	for text != "" {
		loc := linkPattern.FindStringIndex(text)
		if loc == nil {
			builder.Write(text, style)
			return
		}
		if loc[0] > 0 {
			builder.Write(text[:loc[0]], style)
		}
		url := strings.TrimRight(text[loc[0]:loc[1]], ".,;:!?)\"”'*")
		builder.Write(url, style.Underline(true).Url(url))
		text = text[loc[0]+len(url):]
	}
}

// linkPattern matches a link in a bio or a status.
var linkPattern = regexp.MustCompile(`https?://[^\s<>]+`)

// View shows the profile in a box: the banner, or the accent color, the avatar beside the names, then the rest.
func (m Model) View() tview.Widget {
	footer := ""
	switch {
	case m.err != nil:
		footer = "Could not load the full profile"
	case m.profile == nil:
		footer = "Loading..."
	}

	header, status, body := m.lines()
	var names tview.Widget = text.Rich(header...)
	// The nameplate, which Discord shows behind the name at its right end, is shown beside the names, at the right,
	// as text cannot be drawn over an image.
	if nameplate, ok := m.images[m.nameplateURL()]; ok && m.nameplateURL() != "" {
		names = row.New(names, nameplate).Spacing(1).Height(layout.Shrink)
	}
	if avatar, ok := m.images[m.avatarURL()]; ok {
		names = row.New(avatar, names).Spacing(1).Height(layout.Shrink)
	}

	content := column.New().Height(layout.Shrink)
	if banner, ok := m.images[m.bannerURL()]; ok && m.bannerURL() != "" {
		content = content.Push(banner)
	} else if accent := m.accentColor(); accent != discord.NullColor && m.profile != nil {
		content = content.Push(colorBar{rows: accentRows, style: tcell.StyleDefault.Background(tcell.NewHexColor(int32(accent)))})
	}
	content = content.Push(names)
	if len(status) != 0 {
		content = content.Push(text.Rich(status...))
	}
	content = content.Push(text.Rich(body...))

	box := ui.Box(content, m.cfg, true).
		Title(ui.Label("Profile", m.cfg.Theme.Title, true)).
		Footer(ui.Label(footer, m.cfg.Theme.Footer, true))
	// A Nitro profile theme colors the card's border.
	if colors := m.themeColors(); len(colors) != 0 {
		box = box.BorderStyle(tcell.StyleDefault.Foreground(tcell.NewHexColor(int32(colors[0]))))
	}
	return card{Widget: box, model: m}
}

// colorBar is a bar of a color, rows tall and as wide as it is let be.
type colorBar struct {
	rows  int
	style tcell.Style
}

func (b colorBar) Size() (layout.Length, layout.Length) { return layout.Fill, layout.Fixed(b.rows) }

func (b colorBar) Layout(limits layout.Limits) layout.Size {
	return layout.Atomic(limits, layout.Fill, layout.Fixed(b.rows))
}

func (b colorBar) Draw(screen tview.Screen, area tview.Rectangle) {
	screen.FillArea(area.X, area.Y, area.Width, area.Height, ' ', b.style)
}

func (colorBar) Handle(msg tview.Msg, _ tview.Rectangle) tview.Msg { return msg }

// accentColor returns the color of the user's profile, or discord.NullColor if they have none.
func (m Model) accentColor() discord.Color {
	if m.profile != nil && m.profile.UserProfile.AccentColor != 0 {
		return m.profile.UserProfile.AccentColor
	}
	if m.profile != nil && m.profile.User.Accent != 0 {
		return m.profile.User.Accent
	}
	return discord.NullColor
}

// themeColors returns the colors of the user's Nitro profile theme, if any.
func (m Model) themeColors() []discord.Color {
	if m.profile == nil {
		return nil
	}
	return m.profile.UserProfile.ThemeColors
}

// lines returns the spans of the card's names, shown beside the avatar, and of the rest of its text.
func (m Model) lines() (header, status, body []text.Span) {
	user := m.user
	if m.profile != nil && m.profile.User.ID.IsValid() {
		user = m.profile.User.User
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
	header = builder.Finish()
	builder = markdown.Builder{}

	// What the user says and does comes right under the names, whatever else the profile has, as the members
	// list cuts it: the custom status in quotes, then each activity with what it tells, a blank line before each.
	if presence, _ := m.state.Cabinet.Presence(m.guildID, user.ID); presence != nil {
		if status := customStatus(presence.Activities); status != "" {
			builder.NewLine()
			writeLinked(&builder, "“"+status+"”", tcell.StyleDefault.Italic(true))
			builder.NewLine()
		}
		for _, line := range activityLines(presence.Activities, time.Now()) {
			switch {
			case line[0] != "" && user.Bot:
				// A bot writes its activity itself, as a person writes a custom status: it is shown as one.
				builder.NewLine()
				writeLinked(&builder, "“"+line[0]+"”", tcell.StyleDefault.Italic(true))
			case line[0] != "":
				builder.NewLine()
				builder.Write(line[0], tcell.StyleDefault.Bold(true))
			default:
				writeLinked(&builder, line[1], tcell.StyleDefault)
			}
			builder.NewLine()
		}
	}
	status = builder.Finish()
	builder = markdown.Builder{}

	if bio := m.bio(); bio != "" {
		builder.NewLine()
		writeLinked(&builder, bio, tcell.StyleDefault)
		builder.NewLine()
	}

	if roles := m.roleNames(member); len(roles) != 0 {
		builder.NewLine()
		builder.Write("Roles: ", dim)
		builder.Write(strings.Join(roles, ", "), tcell.StyleDefault)
		builder.NewLine()
	}

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

	return header, status, builder.Finish()
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
		return plainBio(guild.Bio)
	}
	return plainBio(m.profile.UserProfile.Bio)
}

// customEmoji matches a custom emoji as a bio holds it, <:name:id> or <a:name:id>.
var customEmoji = regexp.MustCompile(`<a?:(\w+):\d+>`)

// plainBio writes a bio as it reads: custom emojis, which a terminal cannot draw, by their names, and without the
// marks of small text ("-# ").
func plainBio(bio string) string {
	bio = customEmoji.ReplaceAllString(bio, ":$1:")
	lines := strings.Split(strings.TrimSpace(bio), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "-# ")
	}
	return strings.Join(lines, "\n")
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
