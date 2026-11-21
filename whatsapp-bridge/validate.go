package main

import (
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// parseRecipient accepts either a full JID or a bare phone number, which is addressed on the personal-chat server.
func parseRecipient(recipient string) (types.JID, error) {
	if strings.Contains(recipient, "@") {
		return types.ParseJID(recipient)
	}
	return types.JID{User: recipient, Server: types.DefaultUserServer}, nil
}

func parseJIDs(values []string) ([]types.JID, error) {
	jids := make([]types.JID, 0, len(values))
	for _, value := range values {
		jid, err := types.ParseJID(value)
		if err != nil {
			return nil, err
		}
		jids = append(jids, jid)
	}
	return jids, nil
}

func validateSendRequest(req SendMessageRequest) error {
	if req.Recipient == "" {
		return errors.New("Recipient is required")
	}
	if req.Message == "" && req.MediaPath == "" {
		return errors.New("Message or media path is required")
	}
	return nil
}

func validateDeleteRequest(req DeleteMessageRequest) error {
	if req.ChatJID == "" {
		return errors.New("Chat JID is required")
	}
	if req.MessageID == "" {
		return errors.New("Message ID is required")
	}
	return nil
}

func jidError(err error) string {
	return fmt.Sprintf("Error parsing JID: %v", err)
}

// typingState maps the API flag to the chat presence WhatsApp expects.
func typingState(typing bool) types.ChatPresence {
	if typing {
		return types.ChatPresenceComposing
	}
	return types.ChatPresencePaused
}
