package main

import (
	"context"
	"encoding/json"
	"net/http"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// whatsappAPI is the slice of *whatsmeow.Client the typing, edit and group handlers use, so tests can fake it.
type whatsappAPI interface {
	SendPresence(ctx context.Context, state types.Presence) error
	SendChatPresence(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error
	SendMessage(ctx context.Context, to types.JID, message *waProto.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
	BuildEdit(chat types.JID, id types.MessageID, newContent *waProto.Message) *waProto.Message
	CreateGroup(ctx context.Context, req whatsmeow.ReqCreateGroup) (*types.GroupInfo, error)
	UpdateGroupParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error)
}

type sendFunc func(recipient, message, mediaPath, replyTo string) (bool, string)
type deleteFunc func(chatJID, messageID string, forMe bool) (bool, string)

func writeResult(w http.ResponseWriter, success bool, message string) {
	w.Header().Set("Content-Type", "application/json")
	if !success {
		w.WriteHeader(http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(SendMessageResponse{Success: success, Message: message})
}

func registerRoutes(mux *http.ServeMux, api whatsappAPI, send sendFunc, del deleteFunc) {
	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
		if err := validateSendRequest(req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		success, message := send(req.Recipient, req.Message, req.MediaPath, req.ReplyTo)
		writeResult(w, success, message)
	})

	mux.HandleFunc("/api/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req DeleteMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
		if err := validateDeleteRequest(req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		success, message := del(req.ChatJID, req.MessageID, req.ForMe)
		writeResult(w, success, message)
	})

	// WhatsApp only shows "typing..." while we are marked available
	mux.HandleFunc("/api/typing", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Recipient string `json:"recipient"`
			Typing    bool   `json:"typing"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
		jid, err := types.ParseJID(req.Recipient)
		if err != nil {
			http.Error(w, jidError(err), http.StatusBadRequest)
			return
		}
		state := typingState(req.Typing)
		if req.Typing {
			err = api.SendPresence(r.Context(), types.PresenceAvailable)
		}
		if err == nil {
			err = api.SendChatPresence(r.Context(), jid, state, types.ChatPresenceMediaText)
		}
		// Staying available would keep the phone from getting its own notifications
		if err == nil && !req.Typing {
			err = api.SendPresence(r.Context(), types.PresenceUnavailable)
		}
		if err != nil {
			writeResult(w, false, err.Error())
			return
		}
		writeResult(w, true, string(state))
	})

	mux.HandleFunc("/api/edit", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ChatJID   string `json:"chat_jid"`
			MessageID string `json:"message_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
		chat, err := types.ParseJID(req.ChatJID)
		if err != nil {
			http.Error(w, jidError(err), http.StatusBadRequest)
			return
		}
		_, err = api.SendMessage(r.Context(), chat, api.BuildEdit(chat, req.MessageID, &waProto.Message{Conversation: proto.String(req.Message)}))
		if err != nil {
			writeResult(w, false, err.Error())
			return
		}
		writeResult(w, true, req.MessageID)
	})

	mux.HandleFunc("/api/group/create", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name         string   `json:"name"`
			Participants []string `json:"participants"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
		participants, err := parseJIDs(req.Participants)
		if err != nil {
			http.Error(w, jidError(err), http.StatusBadRequest)
			return
		}
		group, err := api.CreateGroup(r.Context(), whatsmeow.ReqCreateGroup{Name: req.Name, Participants: participants})
		if err != nil {
			writeResult(w, false, err.Error())
			return
		}
		writeResult(w, true, group.JID.String())
	})

	mux.HandleFunc("/api/group/participants", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Group        string   `json:"group"`
			Participants []string `json:"participants"`
			Action       string   `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
		group, err := types.ParseJID(req.Group)
		if err != nil {
			http.Error(w, jidError(err), http.StatusBadRequest)
			return
		}
		participants, err := parseJIDs(req.Participants)
		if err != nil {
			http.Error(w, jidError(err), http.StatusBadRequest)
			return
		}
		_, err = api.UpdateGroupParticipants(r.Context(), group, participants, whatsmeow.ParticipantChange(req.Action))
		if err != nil {
			writeResult(w, false, err.Error())
			return
		}
		writeResult(w, true, req.Action)
	})
}

var _ whatsappAPI = (*whatsmeow.Client)(nil)
