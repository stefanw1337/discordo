package profilecard

import (
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/gdamore/tcell/v3"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
)

func TestActivityLines(t *testing.T) {
	start := time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC)
	song := discord.Activity{
		Type: discord.ListeningActivity, Name: "Spotify", Details: "ODD YOUTH", State: "A; B",
		Assets:     &discord.ActivityAssets{LargeText: "The Album"},
		Timestamps: &discord.ActivityTimestamps{Start: discord.UnixMsTimestamp(start.UnixMilli()), End: discord.UnixMsTimestamp(start.Add(225 * time.Second).UnixMilli())},
	}
	lines := activityLines([]discord.Activity{{Type: discord.CustomActivity, State: "hi"}, song}, start.Add(83*time.Second))
	want := [][2]string{{"Listening to Spotify", ""}, {"", "ODD YOUTH"}, {"", "by A, B"}, {"", "on The Album"}, {"", "1:23 / 3:45"}}
	if len(lines) != len(want) {
		t.Fatalf("lines = %v", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %v, want %v", i, lines[i], want[i])
		}
	}
}

func TestCustomStatus(t *testing.T) {
	status := customStatus([]discord.Activity{{Type: discord.GameActivity, Name: "x"}, {Type: discord.CustomActivity, State: "have you heard", Emoji: &discord.Emoji{Name: "🔑"}}})
	if status != "🔑 have you heard" {
		t.Fatalf("customStatus = %q", status)
	}
}

func TestPlainBio(t *testing.T) {
	got := plainBio("<:honeypot:1450060724943720600> Ban users\n-# ↪ *link*")
	if got != ":honeypot: Ban users\n↪ *link*" {
		t.Fatalf("plainBio = %q", got)
	}
}

func TestWriteLinked(t *testing.T) {
	var builder markdown.Builder
	writeLinked(&builder, "“see https://pastebin.com/nFHuAt3v”", tcell.StyleDefault)
	var url string
	for _, span := range builder.Finish() {
		if _, u := span.Style.GetUrl(); u != "" {
			url = u
		}
	}
	if url != "https://pastebin.com/nFHuAt3v" {
		t.Fatalf("link = %q", url)
	}
}

func TestComposePlate(t *testing.T) {
	// A nameplate fills its right half, as a flag does.
	plate := image.NewNRGBA(image.Rect(0, 0, 672, 126))
	for y := range 126 {
		for x := 336; x < 672; x++ {
			plate.SetNRGBA(x, y, color.NRGBA{200, 0, 0, 255})
		}
	}
	img := composePlate(plate, 370, 40, 2, []plateLine{{text: "stefanw1337", color: color.NRGBA{255, 255, 0, 255}, bold: true}, {text: "@stefanw1337", color: color.NRGBA{150, 150, 150, 255}}})
	nrgba := img.(*image.NRGBA)
	if nrgba.Bounds().Dx() != 370 || nrgba.Bounds().Dy() != 40 {
		t.Fatalf("size = %v", nrgba.Bounds())
	}
	if right := nrgba.NRGBAAt(365, 5); right.A != 255 || right.R != 200 {
		t.Fatalf("the nameplate is not kept to the right: %v", right)
	}
	if left := nrgba.NRGBAAt(370-213-5, 30); left.A != 0 {
		t.Fatalf("the nameplate's empty half is drawn: %v", left)
	}
	yellow := false
	for x := range 150 {
		for y := range 20 {
			if c := nrgba.NRGBAAt(x, y); c.R > 200 && c.G > 200 && c.B < 80 {
				yellow = true
			}
		}
	}
	if !yellow {
		t.Fatal("the name was not drawn on the first row, beside the nameplate")
	}
}
