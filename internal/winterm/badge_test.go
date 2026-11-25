package winterm

import "testing"

func TestBadgeImage(t *testing.T) {
	if badgeImage(Badge{}) != nil {
		t.Fatal("a badge was drawn for nothing unread")
	}
	img := badgeImage(Badge{Mentions: 3})
	if c := img.NRGBAAt(2, badgeSize/2); c.R < 200 || c.A != 255 {
		t.Fatalf("the badge is not red: %v", c)
	}
	white := false
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] == 255 && img.Pix[i+1] == 255 && img.Pix[i+2] == 255 {
			white = true
		}
	}
	if !white {
		t.Fatal("the number was not drawn")
	}
}
