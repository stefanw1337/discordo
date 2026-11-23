// Package friendrequests lists the friend requests sent to the user, which they accept or ignore, as Discord's
// Pending tab does.
package friendrequests

import (
	"log/slog"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui/chat/profilecard"
	uipicker "github.com/ayn2op/discordo/internal/ui/picker"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/picker"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
)

type Model struct {
	uipicker.Model
	cfg   *config.Config
	state *ningen.State
}

func NewModel(cfg *config.Config, state *ningen.State) Model {
	return Model{Model: uipicker.NewModel(cfg, "Friend requests"), cfg: cfg, state: state}
}

// Pending returns the users who sent the user a friend request not yet answered.
func Pending(state *ningen.State) []discord.User {
	var users []discord.User
	state.RelationshipState.Each(func(userID discord.UserID, kind discord.RelationshipType) bool {
		if kind == discord.IncomingFriendRequest {
			if relationship, ok := state.RelationshipState.FullRelationship(userID); ok {
				users = append(users, relationship.User)
			}
		}
		return false
	})
	return users
}

// profile is looking at the profile of a user who sent a request, before answering it.
type profile struct{ user discord.User }

// action is accepting or ignoring the request of a user.
type action struct {
	user   discord.User
	accept bool
}

// Open lists the requests, each with what can be done to it.
func (m *Model) Open() {
	users := Pending(m.state)
	if len(users) == 0 {
		m.SetItems(picker.Items{notice("No friend requests")})
		return
	}
	items := make(picker.Items, 0, 2*len(users))
	for _, user := range users {
		name := user.DisplayOrUsername()
		if user.Username != "" && user.Username != name {
			name += " (" + user.Username + ")"
		}
		items = append(items,
			picker.Item{Content: text.Rich(text.Span{Text: "👤 Profile of " + name}), FilterText: name, Reference: profile{user}},
			picker.Item{Content: text.Rich(text.Span{Text: "✓ Accept " + name, Style: tcell.StyleDefault.Foreground(color.Green)}), FilterText: name, Reference: action{user, true}},
			picker.Item{Content: text.Rich(text.Span{Text: "✗ Ignore " + name, Style: tcell.StyleDefault.Dim(true)}), FilterText: name, Reference: action{user, false}},
		)
	}
	m.SetItems(items)
}

func notice(message string) picker.Item {
	return picker.Item{Content: text.Rich(text.Span{Text: message, Style: tcell.StyleDefault.Dim(true)}), Reference: nil}
}

// CancelMsg closes the list; AnsweredMsg reports a request answered.
type (
	CancelMsg   struct{}
	AnsweredMsg struct{}
)

type selectedMsg struct{ action }

var _ tview.Model[Model] = Model{}

func (m Model) Update(msg tview.Msg) (Model, tview.Cmd) {
	if msg, ok := msg.(selectedMsg); ok {
		client, userID, accept := m.state.Client, msg.user.ID, msg.accept
		return m, cmd.Task(func() tview.Msg {
			var err error
			if accept {
				// As Discord's own client accepts one: the relationship is put, with nothing more.
				err = client.FastRequest("PUT", api.EndpointMe+"/relationships/"+userID.String(), httputil.WithJSONBody(struct{}{}))
			} else {
				err = client.DeleteRelationship(userID)
			}
			if err != nil {
				slog.Error("failed to answer friend request", "user_id", userID, "accept", accept, "err", err)
			}
			return AnsweredMsg{}
		})
	}
	var command tview.Cmd
	m.Model, command = m.Model.Update(msg)
	return m, command
}

func (m Model) View() tview.Widget {
	return m.Model.View(func(item picker.Item) tview.Msg {
		switch ref := item.Reference.(type) {
		case action:
			return selectedMsg{ref}
		case profile:
			return profilecard.OpenMsg{User: ref.user}
		}
		return nil
	}, CancelMsg{})
}
