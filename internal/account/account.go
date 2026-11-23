// Package account exports what makes up a Discord account, to find one's way back after losing it: the profile,
// the servers with links to join them again, the friends and the groups, and discordo's own settings. It imports
// the profile into another account, and writes a checklist of the rest, which is done by hand, as joining servers
// and adding friends in bulk is what Discord takes for spam.
package account

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
	"github.com/ayn2op/ningen/v3"
)

// FileName is the name of the export's data, in its folder.
const FileName = "account.json"

// Export is what is kept of an account.
type Export struct {
	ExportedAt time.Time `json:"exported_at"`
	User       User      `json:"user"`
	Profile    Profile   `json:"profile"`
	// Avatar is the file of the avatar in the export's folder, or "".
	Avatar  string   `json:"avatar,omitempty"`
	Servers []Server `json:"servers"`
	Friends []Friend `json:"friends"`
	Groups  []Group  `json:"groups"`
}

type User struct {
	ID         discord.UserID `json:"id"`
	Username   string         `json:"username"`
	GlobalName string         `json:"global_name,omitempty"`
}

// Profile is the "About me" and pronouns.
type Profile struct {
	Bio      string `json:"bio"`
	Pronouns string `json:"pronouns"`
}

// Server is a server the account was in, with the link that joins it, if there is one.
type Server struct {
	ID   discord.GuildID `json:"id"`
	Name string          `json:"name"`
	// Invite is a link that joins the server: its vanity link, or one made for the export if that was asked for.
	Invite string `json:"invite,omitempty"`
	// Nickname is the account's nickname there.
	Nickname string `json:"nickname,omitempty"`
}

type Friend struct {
	ID         discord.UserID `json:"id"`
	Username   string         `json:"username"`
	GlobalName string         `json:"global_name,omitempty"`
	// Nickname is the name the account gave the friend.
	Nickname string `json:"nickname,omitempty"`
}

// Group is a group direct message, by the people in it.
type Group struct {
	ID      discord.ChannelID `json:"id"`
	Name    string            `json:"name,omitempty"`
	Members []string          `json:"members"`
}

// profileResponse is the part of GET /users/@me/profile that is kept.
type profileResponse struct {
	UserProfile Profile `json:"user_profile"`
}

// Write exports the account of state into a new folder in dir, and returns the folder. invites makes a link to
// join each server that has no vanity link, where the account may: a link that does not expire, made once, which
// the server's members can see was made.
func Write(state *ningen.State, dir string, invites bool, configFiles []string) (string, error) {
	me, err := state.Me()
	if err != nil {
		return "", fmt.Errorf("who is logged in is not known yet: %w", err)
	}
	export := Export{
		ExportedAt: time.Now(),
		User:       User{ID: me.ID, Username: me.Username, GlobalName: me.DisplayName},
	}
	if profile, err := state.RequestJSON[profileResponse]("GET", api.EndpointMe+"/profile"); err != nil {
		slog.Error("failed to get own profile for the export", "err", err)
	} else {
		export.Profile = profile.UserProfile
	}

	folder := filepath.Join(dir, "discordo-export-"+safeName(me.Username)+"-"+export.ExportedAt.Format("2006-01-02-150405"))
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return "", err
	}

	if me.Avatar != "" {
		name := "avatar.png"
		url := me.AvatarURLWithType(discord.PNGImage) + "?size=1024"
		if strings.HasPrefix(string(me.Avatar), "a_") {
			name, url = "avatar.gif", me.AvatarURLWithType(discord.GIFImage)+"?size=1024"
		}
		if err := download(url, filepath.Join(folder, name)); err != nil {
			slog.Error("failed to download avatar for the export", "err", err)
		} else {
			export.Avatar = name
		}
	}

	export.Servers = servers(state, me.ID, invites)
	export.Friends = friends(state)
	export.Groups = groups(state)

	data, err := json.MarshalIndent(export, "", "\t")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(folder, FileName), data, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(folder, "checklist.md"), []byte(Checklist(export)), 0o600); err != nil {
		return "", err
	}
	// discordo's own settings and server list are kept with it, to be copied back beside the program.
	for _, path := range configFiles {
		if data, err := os.ReadFile(path); err == nil {
			_ = os.WriteFile(filepath.Join(folder, filepath.Base(path)), data, 0o600)
		}
	}
	return folder, nil
}

func servers(state *ningen.State, me discord.UserID, invites bool) []Server {
	guilds, err := state.Cabinet.Guilds()
	if err != nil {
		slog.Error("failed to list servers for the export", "err", err)
		return nil
	}
	var servers []Server
	for _, guild := range guilds {
		server := Server{ID: guild.ID, Name: guild.Name}
		if member, err := state.Cabinet.Member(guild.ID, me); err == nil {
			server.Nickname = member.Nick
		}
		switch {
		case guild.VanityURLCode != "":
			server.Invite = "https://discord.gg/" + guild.VanityURLCode
		case invites:
			server.Invite = makeInvite(state, guild.ID, me)
		}
		servers = append(servers, server)
	}
	slices.SortFunc(servers, func(a, b Server) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return servers
}

// makeInvite makes a link that does not expire to the first channel of guild the account may invite to, or "".
func makeInvite(state *ningen.State, guildID discord.GuildID, me discord.UserID) string {
	channels, err := state.Cabinet.Channels(guildID)
	if err != nil {
		return ""
	}
	slices.SortFunc(channels, func(a, b discord.Channel) int { return cmp.Compare(a.Position, b.Position) })
	for _, channel := range channels {
		if channel.Type != discord.GuildText {
			continue
		}
		permissions, err := state.Permissions(channel.ID, me)
		if err != nil || !permissions.Has(discord.PermissionCreateInstantInvite) {
			continue
		}
		type inviteData struct {
			MaxAge  int  `json:"max_age"`
			MaxUses int  `json:"max_uses"`
			Unique  bool `json:"unique"`
		}
		// Not unique: a link made before with the same settings is given back, rather than another made.
		invite, err := state.RequestJSON[discord.Invite]("POST", api.EndpointChannels+channel.ID.String()+"/invites",
			httputil.WithJSONBody(inviteData{}))
		if err != nil {
			slog.Error("failed to make invite for the export", "guild_id", guildID, "err", err)
			return ""
		}
		return "https://discord.gg/" + invite.Code
	}
	return ""
}

func friends(state *ningen.State) []Friend {
	var friends []Friend
	state.RelationshipState.Each(func(userID discord.UserID, kind discord.RelationshipType) bool {
		if kind != discord.FriendRelationship {
			return false
		}
		if relationship, ok := state.RelationshipState.FullRelationship(userID); ok {
			friend := Friend{ID: userID, Username: relationship.User.Username, GlobalName: relationship.User.DisplayName}
			if relationship.Nickname != nil {
				friend.Nickname = *relationship.Nickname
			}
			friends = append(friends, friend)
		}
		return false
	})
	slices.SortFunc(friends, func(a, b Friend) int { return cmp.Compare(strings.ToLower(a.Username), strings.ToLower(b.Username)) })
	return friends
}

func groups(state *ningen.State) []Group {
	channels, err := state.PrivateChannels()
	if err != nil {
		return nil
	}
	var groups []Group
	for _, channel := range channels {
		if channel.Type != discord.GroupDM {
			continue
		}
		group := Group{ID: channel.ID, Name: channel.Name}
		for _, user := range channel.DMRecipients {
			group.Members = append(group.Members, user.Username)
		}
		groups = append(groups, group)
	}
	return groups
}

// Checklist is what to do by hand to bring the account back, in Markdown.
func Checklist(export Export) string {
	var b strings.Builder
	name := export.User.Username
	fmt.Fprintf(&b, "# Bringing back %s\n\nExported %s. Discordo's \"Import profile\" (settings, ctrl+s) restores the display name, avatar, About me and pronouns into the account logged in. The rest is done by hand, as Discord takes joining servers and adding friends in bulk for spam.\n\n",
		name, export.ExportedAt.Format("2 January 2006 15:04"))

	b.WriteString("## Profile\n\n")
	fmt.Fprintf(&b, "- [ ] Display name: %s\n", cmp.Or(export.User.GlobalName, "(none)"))
	if export.Avatar != "" {
		fmt.Fprintf(&b, "- [ ] Avatar: %s\n", export.Avatar)
	}
	if export.Profile.Pronouns != "" {
		fmt.Fprintf(&b, "- [ ] Pronouns: %s\n", export.Profile.Pronouns)
	}
	if export.Profile.Bio != "" {
		fmt.Fprintf(&b, "- [ ] About me:\n\n%s\n", quote(export.Profile.Bio))
	}

	fmt.Fprintf(&b, "\n## Servers (%d)\n\n", len(export.Servers))
	for _, server := range export.Servers {
		line := "- [ ] " + server.Name
		if server.Invite != "" {
			line += ": " + server.Invite
		} else {
			line += ": ask a member for an invite"
		}
		if server.Nickname != "" {
			line += " (nickname " + server.Nickname + ")"
		}
		b.WriteString(line + "\n")
	}

	fmt.Fprintf(&b, "\n## Friends (%d)\n\nAdd each by username under Friends, Add Friend.\n\n", len(export.Friends))
	for _, friend := range export.Friends {
		line := "- [ ] " + friend.Username
		if friend.GlobalName != "" && friend.GlobalName != friend.Username {
			line += " (" + friend.GlobalName + ")"
		}
		if friend.Nickname != "" {
			line += ", named " + friend.Nickname
		}
		b.WriteString(line + "\n")
	}

	if len(export.Groups) > 0 {
		fmt.Fprintf(&b, "\n## Groups (%d)\n\nAsk someone still in each to add you back.\n\n", len(export.Groups))
		for _, group := range export.Groups {
			line := "- [ ] " + strings.Join(group.Members, ", ")
			if group.Name != "" {
				line = "- [ ] " + group.Name + ": " + strings.Join(group.Members, ", ")
			}
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n## Discordo\n\n- [ ] Copy config.toml and tree.json from this folder beside discordo, for its settings and the order of the server bar.\n")
	return b.String()
}

func quote(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i, line := range lines {
		lines[i] = "  > " + line
	}
	return strings.Join(lines, "\n")
}

// safeName keeps the letters, digits, dots, dashes and underscores of name, for a folder name.
func safeName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			return r
		}
		return '_'
	}, name)
}

func download(url, path string) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return errors.New(response.Status)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// Import sets the profile of the account of state to the one exported in folder: the display name, avatar, About
// me and pronouns. It returns what was done.
func Import(state *ningen.State, folder string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(folder, FileName))
	if err != nil {
		return nil, fmt.Errorf("no export in %s: %w", folder, err)
	}
	var export Export
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("could not read the export: %w", err)
	}

	var done []string
	type userData struct {
		GlobalName string `json:"global_name,omitempty"`
		Avatar     string `json:"avatar,omitempty"`
	}
	user := userData{GlobalName: export.User.GlobalName}
	if export.Avatar != "" {
		if image, err := os.ReadFile(filepath.Join(folder, export.Avatar)); err == nil {
			kind := "image/png"
			if strings.HasSuffix(export.Avatar, ".gif") {
				kind = "image/gif"
			}
			user.Avatar = "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(image)
		}
	}
	if user != (userData{}) {
		if err := state.FastRequest("PATCH", api.EndpointMe, httputil.WithJSONBody(user)); err != nil {
			return done, fmt.Errorf("could not set the name and avatar: %w", err)
		}
		done = append(done, "display name and avatar")
	}
	if export.Profile != (Profile{}) {
		if err := state.FastRequest("PATCH", api.EndpointMe+"/profile", httputil.WithJSONBody(export.Profile)); err != nil {
			return done, fmt.Errorf("could not set About me and pronouns: %w", err)
		}
		done = append(done, "About me and pronouns")
	}
	return done, nil
}
