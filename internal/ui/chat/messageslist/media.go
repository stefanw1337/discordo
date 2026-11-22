package messageslist

import (
	"os/exec"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/stream"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
)

// viewMediaMsg asks to show the image, GIF or video at url, called name, at its full size.
type viewMediaMsg struct{ url, name string }

func (viewMediaMsg) isMsg() {}

// clickable is a widget that a left click on turns into msg.
type clickable struct {
	tview.Widget
	msg tview.Msg
}

func (w clickable) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg {
	if mouse, ok := msg.(tview.MouseMsg); ok && mouse.Action == tview.MouseLeftDown && area.Contains(mouse.Position()) {
		return w.msg
	}
	return w.Widget.Handle(msg, area)
}

// mediaViewerArgs show one image, GIF or video in mpv's window until it is closed, as an image viewer would.
var mediaViewerArgs = []string{"--no-terminal", "--force-window=immediate", "--keep-open=yes", "--image-display-duration=inf", "--loop-file=inf", "--hwdec=auto-safe"}

// viewMedia shows the media at url in the stream player, mpv by default, which shows images, GIFs and videos at
// their full size, or in the browser if there is none.
func (m Model) viewMedia(url, name string) tview.Cmd {
	if url == "" {
		return nil
	}
	player := m.cfg.Stream.Player
	if player == "" {
		return openURL(url)
	}

	return cmd.Task(func() tview.Msg {
		args := append([]string{}, mediaViewerArgs...)
		if name != "" {
			args = append(args, "--title="+name)
		}
		viewer := exec.Command(stream.FindPlayer(player), append(args, url)...)
		if err := viewer.Start(); err != nil {
			// Without the player, the browser shows it instead.
			if err := openURLNow(url); err != nil {
				return ui.ErrorDialog("show "+name, err)
			}
			return nil
		}
		go viewer.Wait() //nolint:errcheck
		return nil
	})
}

// viewSelectedMedia shows the first image, GIF or video of the selected message at its full size.
func (m Model) viewSelectedMedia() tview.Cmd {
	message, ok := m.selectedMessage()
	if !ok {
		return nil
	}
	if sources := m.previewSources(*message); len(sources) != 0 {
		return m.viewMedia(sources[0].url, sources[0].name)
	}
	for _, attachment := range message.Attachments {
		if isMedia(attachment.ContentType) {
			return m.viewMedia(attachment.URL, attachment.Filename)
		}
	}
	return nil
}

// hasMedia reports whether message has an image, GIF or video to show.
func (m Model) hasMedia(message discord.Message) bool {
	if len(m.previewSources(message)) != 0 {
		return true
	}
	for _, attachment := range message.Attachments {
		if isMedia(attachment.ContentType) {
			return true
		}
	}
	return false
}

// isMedia reports whether contentType is an image or video, which the viewer shows.
func isMedia(contentType string) bool {
	return len(contentType) > 6 && (contentType[:6] == "image/" || contentType[:6] == "video/")
}
