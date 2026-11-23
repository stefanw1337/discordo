package guildstree

import (
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/voice"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview"
)

// voiceOptions returns the voice settings of cfg.
func voiceOptions(cfg *config.Config) voice.Options {
	return voice.Options{
		Sensitivity:  cfg.Voice.Sensitivity,
		Volume:       cfg.Voice.Volume,
		InputDevice:  cfg.Voice.InputDevice,
		OutputDevice: cfg.Voice.OutputDevice,
		PushToTalk:   cfg.Voice.PushToTalk,
	}
}

func newVoiceSession(state *ningen.State, cfg *config.Config) *voice.Session {
	session := voice.New(state.State, cfg.Voice.Sensitivity, cfg.Voice.Volume)
	session.SetOptions(voiceOptions(cfg))
	return session
}

// ApplyVoiceSettings applies the voice settings as they are now, which the settings page changed.
func (m Model) ApplyVoiceSettings() {
	m.voiceSession.SetOptions(voiceOptions(m.cfg))
}

// JoinCall joins the call of a direct message or group, answering it.
func (m Model) JoinCall(channelID discord.ChannelID) tview.Cmd {
	return voiceCmd("answer call", func() error { return m.voiceSession.Join(channelID) })
}
