package messageslist

import (
	"github.com/ayn2op/tview/cmd"
	"image"
	"slices"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/ui/emojis"
	"github.com/ayn2op/discordo/internal/ui/images"
	tviewimage "github.com/ayn2op/tview/image"
)

func TestModelPreviewSources(t *testing.T) {
	for _, tt := range []struct {
		name    string
		message discord.Message
		want    []previewSource
	}{
		{"file attachment", discord.Message{Attachments: []discord.Attachment{{Proxy: "a"}}}, nil},
		{"embed image over thumbnail", discord.Message{Embeds: []discord.Embed{{
			Type:      discord.ImageEmbed,
			Image:     &discord.EmbedImage{Proxy: "i", Width: 2, Height: 1},
			Thumbnail: &discord.EmbedThumbnail{Proxy: "t", Width: 2, Height: 1},
		}}}, []previewSource{{proxy: "i", width: 2, height: 1}}},
		{"gif thumbnail", discord.Message{Embeds: []discord.Embed{{
			Type:      discord.GIFVEmbed,
			Thumbnail: &discord.EmbedThumbnail{Proxy: "t", Width: 2, Height: 1},
		}}}, []previewSource{{proxy: "t", width: 2, height: 1}}},
		{"youtube video plays from its page", discord.Message{Embeds: []discord.Embed{{
			Type:      discord.VideoEmbed,
			URL:       "https://www.youtube.com/watch?v=x",
			Title:     "v",
			Provider:  &discord.EmbedProvider{Name: "YouTube"},
			Video:     &discord.EmbedVideo{URL: "https://www.youtube.com/embed/x"},
			Thumbnail: &discord.EmbedThumbnail{Proxy: "t", Width: 2, Height: 1},
		}}}, []previewSource{{proxy: "t", width: 2, height: 1, url: "https://www.youtube.com/watch?v=x", name: "v"}}},
		{"video file plays itself", discord.Message{Embeds: []discord.Embed{{
			Type:      discord.VideoEmbed,
			URL:       "https://example.com/a.mp4",
			Video:     &discord.EmbedVideo{URL: "https://cdn.example.com/a.mp4"},
			Thumbnail: &discord.EmbedThumbnail{Proxy: "t", Width: 2, Height: 1},
		}}}, []previewSource{{proxy: "t", width: 2, height: 1, url: "https://cdn.example.com/a.mp4"}}},
		{"article thumbnail", discord.Message{Embeds: []discord.Embed{{
			Type:      discord.ArticleEmbed,
			Thumbnail: &discord.EmbedThumbnail{Proxy: "t", Width: 2, Height: 1},
		}}}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ml := newTestModel(t, testConfig(t))
			if got := ml.previewSources(tt.message); !slices.Equal(got, tt.want) {
				t.Fatalf("previewSources() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("embeds disabled", func(t *testing.T) {
		ml := newTestModel(t, testConfig(t))
		ml.cfg.UI.MessagesList.Embeds.Enabled = false
		message := discord.Message{
			Attachments: []discord.Attachment{{Proxy: "a", Width: 2, Height: 1}},
			Embeds: []discord.Embed{{
				Type:  discord.ImageEmbed,
				Image: &discord.EmbedImage{Proxy: "i", Width: 2, Height: 1},
			}},
		}
		want := []previewSource{{proxy: "a", width: 2, height: 1}}
		if got := ml.previewSources(message); !slices.Equal(got, want) {
			t.Fatalf("previewSources() = %v, want %v", got, want)
		}
	})
}

func TestModelEmojiLine(t *testing.T) {
	ml := newTestModel(t, testConfig(t))
	t.Run("not loaded", func(t *testing.T) {
		if got := ml.emojiLine("5"); got != nil {
			t.Fatalf("line = %v, want nil before the emoji is loaded", got)
		}
	})
	t.Run("loaded", func(t *testing.T) {
		ml.images = map[discord.URL]images.Widget{emojis.URLFromID(5): tviewimage.New(image.NewAlpha(image.Rect(0, 0, 2, 2))).Width(2)}
		if got := ml.emojiLine("5"); len(got) != 2 {
			t.Fatalf("line = %v, want the two cells of the emoji", got)
		}
	})
}

func TestModelLoadImages(t *testing.T) {
	ml := newTestModel(t, testConfig(t))
	ml.setMessages([]discord.Message{{ID: 2, ChannelID: 1, Content: "hi <:cat:5>"}})
	msg := ml.LoadImages().(cmd.Task)().(images.RequestedMsg)
	t.Run("content emoji", func(t *testing.T) {
		if rows := msg.Squares[emojis.URLFromID(5)]; rows != emojis.Rows {
			t.Fatalf("the emoji in the content was asked for %d rows tall, want %d", rows, emojis.Rows)
		}
	})
	t.Run("keeps the parsed content", func(t *testing.T) {
		if ml.items[len(ml.items)-1].root == nil {
			t.Fatal("the parsed content was not kept for rendering")
		}
	})
}

func TestMessageContentComponents(t *testing.T) {
	message := discord.Message{
		Content: "hi",
		Components: discord.TopLevelComponents{
			&discord.ContainerComponent{Components: []discord.Component{
				&discord.TextDisplayComponent{Content: "won the week"},
				&discord.SectionComponent{Components: []discord.Component{&discord.TextDisplayComponent{Content: "see the leaderboard"}}},
			}},
		},
	}
	if got, want := messageContent(message), "hi\nwon the week\nsee the leaderboard"; got != want {
		t.Fatalf("messageContent = %q, want %q", got, want)
	}
}

func TestBoostText(t *testing.T) {
	for _, test := range []struct {
		message discord.Message
		want    string
	}{
		{discord.Message{Type: discord.NitroBoostMessage}, "boosted the server!"},
		{discord.Message{Type: discord.NitroBoostMessage, Content: "2"}, "boosted the server 2 times!"},
		{discord.Message{Type: discord.NitroTier2Message}, "boosted the server. The server has reached level 2!"},
	} {
		if got := boostText(test.message); got != test.want {
			t.Errorf("boostText(%v) = %q, want %q", test.message.Type, got, test.want)
		}
	}
}

func TestTenorGIF(t *testing.T) {
	embed := discord.Embed{
		Type:  discord.GIFVEmbed,
		Video: &discord.EmbedVideo{URL: "https://media.tenor.com/Ab1Cd2Ef3GhAAAPo/cat-dance.mp4"},
	}
	if got, want := tenorGIF(embed), discord.URL("https://media.tenor.com/Ab1Cd2Ef3GhAAAAC/cat-dance.gif"); got != want {
		t.Fatalf("tenorGIF = %q, want %q", got, want)
	}
	embed.Video.URL = "https://static.klipy.com/ii/abc/rock.mp4"
	if got := tenorGIF(embed); got != "" {
		t.Fatalf("tenorGIF of another site = %q, want none", got)
	}
}

func TestVideoFrame(t *testing.T) {
	got := videoFrame("https://media.discordapp.net/attachments/1/2/a.mp4?ex=1&hm=2")
	if got != "https://media.discordapp.net/attachments/1/2/a.mp4?ex=1&format=jpeg&hm=2" {
		t.Fatalf("videoFrame = %s", got)
	}
}
