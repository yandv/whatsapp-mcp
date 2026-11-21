package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestQuotedParticipant(t *testing.T) {
	ownJID := types.JID{User: "5500000000099", Server: "s.whatsapp.net", Device: 7}
	ownLID := types.JID{User: "100000000000099", Server: "lid", Device: 7}
	tests := []struct {
		name     string
		sender   string
		chat     types.JID
		isFromMe bool
		want     string
	}{
		{"direct chat quotes the sender on the chat's server", "5500000000001", types.NewJID("5500000000001", "s.whatsapp.net"), false, "5500000000001@s.whatsapp.net"},
		{"lid chat quotes the sender on the lid server", "100000000000001", types.NewJID("100000000000001", "lid"), false, "100000000000001@lid"},
		{"group quotes the sender on the phone server", "5500000000001", types.NewJID("120363000000000001", "g.us"), false, "5500000000001@s.whatsapp.net"},
		{"own message in a phone chat uses our phone JID without device", "5500000000099", types.NewJID("5500000000001", "s.whatsapp.net"), true, "5500000000099@s.whatsapp.net"},
		{"own message in a group uses our phone JID without device", "5500000000099", types.NewJID("120363000000000001", "g.us"), true, "5500000000099@s.whatsapp.net"},
		{"own message in a lid chat uses our lid without device", "5500000000099", types.NewJID("100000000000001", "lid"), true, "100000000000099@lid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quotedParticipant(tt.sender, tt.chat, tt.isFromMe, ownJID, ownLID)
			if got.String() != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestBuildQuotedMessage(t *testing.T) {
	media := storedMedia{Content: "caption", URL: "https://example.invalid/m", MediaKey: []byte{1}, FileSHA256: []byte{2}, FileEncSHA256: []byte{3}, FileLength: 42}

	t.Run("text is quoted as a conversation", func(t *testing.T) {
		got := buildQuotedMessage(storedMedia{Content: "hello"})
		if got.GetConversation() != "hello" || got.ImageMessage != nil || got.AudioMessage != nil {
			t.Errorf("got %v", got)
		}
	})
	t.Run("unknown media type falls back to text", func(t *testing.T) {
		m := media
		m.MediaType = "sticker"
		if got := buildQuotedMessage(m); got.GetConversation() != "caption" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("image keeps caption, url, keys and length", func(t *testing.T) {
		m := media
		m.MediaType = "image"
		img := buildQuotedMessage(m).GetImageMessage()
		if img == nil || img.GetCaption() != "caption" || img.GetURL() != media.URL || img.GetFileLength() != 42 || img.GetMediaKey()[0] != 1 || img.GetFileSHA256()[0] != 2 || img.GetFileEncSHA256()[0] != 3 {
			t.Errorf("got %v", img)
		}
	})
	t.Run("video keeps the caption", func(t *testing.T) {
		m := media
		m.MediaType = "video"
		if v := buildQuotedMessage(m).GetVideoMessage(); v == nil || v.GetCaption() != "caption" {
			t.Errorf("got %v", v)
		}
	})
	t.Run("document keeps the caption", func(t *testing.T) {
		m := media
		m.MediaType = "document"
		if d := buildQuotedMessage(m).GetDocumentMessage(); d == nil || d.GetCaption() != "caption" {
			t.Errorf("got %v", d)
		}
	})
	t.Run("audio is quoted as a voice note without caption", func(t *testing.T) {
		m := media
		m.MediaType = "audio"
		a := buildQuotedMessage(m).GetAudioMessage()
		if a == nil || !a.GetPTT() || a.GetMimetype() != "audio/ogg; codecs=opus" {
			t.Errorf("got %v", a)
		}
	})
}
