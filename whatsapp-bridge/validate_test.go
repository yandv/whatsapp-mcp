package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestParseRecipient(t *testing.T) {
	tests := []struct {
		name      string
		recipient string
		want      types.JID
		wantErr   bool
	}{
		{"bare phone number goes to the personal server", "5500000000001", types.JID{User: "5500000000001", Server: "s.whatsapp.net"}, false},
		{"full user JID is parsed", "5500000000001@s.whatsapp.net", types.JID{User: "5500000000001", Server: "s.whatsapp.net"}, false},
		{"group JID keeps the group server", "120363000000000001@g.us", types.JID{User: "120363000000000001", Server: "g.us"}, false},
		{"hidden user JID keeps the lid server", "100000000000001@lid", types.JID{User: "100000000000001", Server: "lid"}, false},
		{"JID with too many parts is rejected", "a.b.c.d@s.whatsapp.net", types.JID{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRecipient(tt.recipient)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseJIDs(t *testing.T) {
	t.Run("empty list gives an empty slice", func(t *testing.T) {
		got, err := parseJIDs(nil)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v", got, err)
		}
	})
	t.Run("keeps the order", func(t *testing.T) {
		got, err := parseJIDs([]string{"1@s.whatsapp.net", "2@s.whatsapp.net"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].User != "1" || got[1].User != "2" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("a single invalid entry fails the whole list", func(t *testing.T) {
		if _, err := parseJIDs([]string{"1@s.whatsapp.net", "a.b.c.d@s.whatsapp.net"}); err == nil {
			t.Error("expected an error")
		}
	})
}

func TestValidateSendRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     SendMessageRequest
		wantErr string
	}{
		{"text message is valid", SendMessageRequest{Recipient: "1", Message: "hi"}, ""},
		{"media only is valid", SendMessageRequest{Recipient: "1", MediaPath: "/tmp/a.png"}, ""},
		{"reply is valid", SendMessageRequest{Recipient: "1", Message: "hi", ReplyTo: "ABC"}, ""},
		{"missing recipient", SendMessageRequest{Message: "hi"}, "Recipient is required"},
		{"missing message and media", SendMessageRequest{Recipient: "1"}, "Message or media path is required"},
		{"recipient is checked before content", SendMessageRequest{}, "Recipient is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSendRequest(tt.req)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDeleteRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     DeleteMessageRequest
		wantErr string
	}{
		{"revoke for everyone", DeleteMessageRequest{ChatJID: "1@s.whatsapp.net", MessageID: "ABC"}, ""},
		{"delete for me", DeleteMessageRequest{ChatJID: "1@s.whatsapp.net", MessageID: "ABC", ForMe: true}, ""},
		{"missing chat", DeleteMessageRequest{MessageID: "ABC"}, "Chat JID is required"},
		{"missing message id", DeleteMessageRequest{ChatJID: "1@s.whatsapp.net"}, "Message ID is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeleteRequest(tt.req)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestTypingState(t *testing.T) {
	if got := typingState(true); got != types.ChatPresenceComposing {
		t.Errorf("typing true = %v", got)
	}
	if got := typingState(false); got != types.ChatPresencePaused {
		t.Errorf("typing false = %v", got)
	}
}
