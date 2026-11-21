package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

type fakeAPI struct {
	presences     []types.Presence
	chatPresences []types.ChatPresence
	chatTargets   []types.JID
	sentTo        []types.JID
	sent          []*waProto.Message
	editedID      string
	groupRequest  whatsmeow.ReqCreateGroup
	groupChanged  types.JID
	groupAction   whatsmeow.ParticipantChange
	groupMembers  []types.JID
	err           error
}

func (f *fakeAPI) SendPresence(_ context.Context, state types.Presence) error {
	f.presences = append(f.presences, state)
	return f.err
}

func (f *fakeAPI) SendChatPresence(_ context.Context, jid types.JID, state types.ChatPresence, _ types.ChatPresenceMedia) error {
	f.chatTargets = append(f.chatTargets, jid)
	f.chatPresences = append(f.chatPresences, state)
	return nil
}

func (f *fakeAPI) SendMessage(_ context.Context, to types.JID, message *waProto.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	f.sentTo = append(f.sentTo, to)
	f.sent = append(f.sent, message)
	return whatsmeow.SendResponse{}, f.err
}

func (f *fakeAPI) BuildEdit(_ types.JID, id types.MessageID, newContent *waProto.Message) *waProto.Message {
	f.editedID = id
	return newContent
}

func (f *fakeAPI) CreateGroup(_ context.Context, req whatsmeow.ReqCreateGroup) (*types.GroupInfo, error) {
	f.groupRequest = req
	if f.err != nil {
		return nil, f.err
	}
	return &types.GroupInfo{JID: types.NewJID("120363000000000001", "g.us")}, nil
}

func (f *fakeAPI) UpdateGroupParticipants(_ context.Context, jid types.JID, members []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error) {
	f.groupChanged, f.groupMembers, f.groupAction = jid, members, action
	return nil, f.err
}

type sendCall struct {
	recipient, message, mediaPath, replyTo string
}

type deleteCall struct {
	chatJID, messageID string
	forMe              bool
}

type harness struct {
	api         *fakeAPI
	mux         *http.ServeMux
	sendCalls   []sendCall
	deleteCalls []deleteCall
	sendResult  bool
	deleteOK    bool
}

func newHarness() *harness {
	h := &harness{api: &fakeAPI{}, mux: http.NewServeMux(), sendResult: true, deleteOK: true}
	registerRoutes(h.mux, h.api,
		func(recipient, message, mediaPath, replyTo string) (bool, string) {
			h.sendCalls = append(h.sendCalls, sendCall{recipient, message, mediaPath, replyTo})
			return h.sendResult, "result"
		},
		func(chatJID, messageID string, forMe bool) (bool, string) {
			h.deleteCalls = append(h.deleteCalls, deleteCall{chatJID, messageID, forMe})
			return h.deleteOK, "result"
		})
	return h
}

func (h *harness) do(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) SendMessageResponse {
	t.Helper()
	var resp SendMessageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("body is not a result JSON: %v (%q)", err, rec.Body.String())
	}
	return resp
}

func TestSendHandler(t *testing.T) {
	t.Run("forwards a plain message", func(t *testing.T) {
		h := newHarness()
		rec := h.do("POST", "/api/send", `{"recipient":"5500000000001","message":"hi"}`)
		if rec.Code != 200 || len(h.sendCalls) != 1 || h.sendCalls[0] != (sendCall{"5500000000001", "hi", "", ""}) {
			t.Fatalf("status %d calls %v", rec.Code, h.sendCalls)
		}
		if resp := decode(t, rec); !resp.Success || resp.Message != "result" {
			t.Errorf("resp %+v", resp)
		}
	})
	t.Run("forwards reply_to and media_path", func(t *testing.T) {
		h := newHarness()
		h.do("POST", "/api/send", `{"recipient":"1@s.whatsapp.net","message":"cap","media_path":"/tmp/a.png","reply_to":"ABC"}`)
		if h.sendCalls[0] != (sendCall{"1@s.whatsapp.net", "cap", "/tmp/a.png", "ABC"}) {
			t.Errorf("calls %v", h.sendCalls)
		}
	})
	t.Run("answers 500 with the failure message when sending fails", func(t *testing.T) {
		h := newHarness()
		h.sendResult = false
		rec := h.do("POST", "/api/send", `{"recipient":"1","message":"hi"}`)
		if rec.Code != 500 || decode(t, rec).Success {
			t.Errorf("status %d body %s", rec.Code, rec.Body)
		}
	})
	t.Run("rejects other methods", func(t *testing.T) {
		h := newHarness()
		if rec := h.do("GET", "/api/send", ""); rec.Code != 405 || len(h.sendCalls) != 0 {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("rejects malformed and incomplete bodies without sending", func(t *testing.T) {
		for _, body := range []string{`{`, `{"message":"hi"}`, `{"recipient":"1"}`} {
			h := newHarness()
			if rec := h.do("POST", "/api/send", body); rec.Code != 400 || len(h.sendCalls) != 0 {
				t.Errorf("body %s: status %d calls %v", body, rec.Code, h.sendCalls)
			}
		}
	})
}

func TestDeleteHandler(t *testing.T) {
	t.Run("revokes for everyone by default", func(t *testing.T) {
		h := newHarness()
		rec := h.do("POST", "/api/delete", `{"chat_jid":"1@s.whatsapp.net","message_id":"ABC"}`)
		if rec.Code != 200 || h.deleteCalls[0] != (deleteCall{"1@s.whatsapp.net", "ABC", false}) {
			t.Errorf("status %d calls %v", rec.Code, h.deleteCalls)
		}
	})
	t.Run("passes for_me through", func(t *testing.T) {
		h := newHarness()
		h.do("POST", "/api/delete", `{"chat_jid":"1@s.whatsapp.net","message_id":"ABC","for_me":true}`)
		if !h.deleteCalls[0].forMe {
			t.Errorf("calls %v", h.deleteCalls)
		}
	})
	t.Run("answers 500 when deleting fails", func(t *testing.T) {
		h := newHarness()
		h.deleteOK = false
		if rec := h.do("POST", "/api/delete", `{"chat_jid":"1@s.whatsapp.net","message_id":"ABC"}`); rec.Code != 500 {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("rejects invalid requests without deleting", func(t *testing.T) {
		for _, body := range []string{`{`, `{"message_id":"ABC"}`, `{"chat_jid":"1@s.whatsapp.net"}`} {
			h := newHarness()
			if rec := h.do("POST", "/api/delete", body); rec.Code != 400 || len(h.deleteCalls) != 0 {
				t.Errorf("body %s: status %d", body, rec.Code)
			}
		}
	})
	t.Run("rejects other methods", func(t *testing.T) {
		if rec := newHarness().do("GET", "/api/delete", ""); rec.Code != 405 {
			t.Errorf("status %d", rec.Code)
		}
	})
}

func TestTypingHandler(t *testing.T) {
	t.Run("typing on marks us available then composing", func(t *testing.T) {
		h := newHarness()
		rec := h.do("POST", "/api/typing", `{"recipient":"1@s.whatsapp.net","typing":true}`)
		if rec.Code != 200 || decode(t, rec).Message != "composing" {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		if len(h.api.presences) != 1 || h.api.presences[0] != types.PresenceAvailable {
			t.Errorf("presences %v", h.api.presences)
		}
		if len(h.api.chatPresences) != 1 || h.api.chatPresences[0] != types.ChatPresenceComposing || h.api.chatTargets[0].User != "1" {
			t.Errorf("chat presences %v to %v", h.api.chatPresences, h.api.chatTargets)
		}
	})
	t.Run("typing off pauses then sets the account unavailable", func(t *testing.T) {
		h := newHarness()
		rec := h.do("POST", "/api/typing", `{"recipient":"1@s.whatsapp.net","typing":false}`)
		if rec.Code != 200 || decode(t, rec).Message != "paused" {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		if len(h.api.presences) != 1 || h.api.presences[0] != types.PresenceUnavailable || h.api.chatPresences[0] != types.ChatPresencePaused {
			t.Errorf("presences %v chat %v", h.api.presences, h.api.chatPresences)
		}
	})
	t.Run("missing typing flag counts as off", func(t *testing.T) {
		h := newHarness()
		h.do("POST", "/api/typing", `{"recipient":"1@s.whatsapp.net"}`)
		if h.api.chatPresences[0] != types.ChatPresencePaused {
			t.Errorf("chat presences %v", h.api.chatPresences)
		}
	})
	t.Run("a presence failure answers 500 and skips the chat presence", func(t *testing.T) {
		h := newHarness()
		h.api.err = errors.New("not connected")
		rec := h.do("POST", "/api/typing", `{"recipient":"1@s.whatsapp.net","typing":true}`)
		if rec.Code != 500 || decode(t, rec).Message != "not connected" || len(h.api.chatPresences) != 0 {
			t.Errorf("status %d body %s", rec.Code, rec.Body)
		}
	})
	t.Run("rejects an invalid JID or body", func(t *testing.T) {
		for _, body := range []string{`{`, `{"recipient":"a.b.c.d@s.whatsapp.net","typing":true}`} {
			h := newHarness()
			if rec := h.do("POST", "/api/typing", body); rec.Code != 400 || len(h.api.presences) != 0 {
				t.Errorf("body %s: status %d", body, rec.Code)
			}
		}
	})
}

func TestEditHandler(t *testing.T) {
	t.Run("sends an edit with the new text", func(t *testing.T) {
		h := newHarness()
		rec := h.do("POST", "/api/edit", `{"chat_jid":"1@s.whatsapp.net","message_id":"ABC","message":"fixed"}`)
		if rec.Code != 200 || decode(t, rec).Message != "ABC" {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		if h.api.editedID != "ABC" || h.api.sent[0].GetConversation() != "fixed" || h.api.sentTo[0].User != "1" {
			t.Errorf("edited %q sent %v to %v", h.api.editedID, h.api.sent, h.api.sentTo)
		}
	})
	t.Run("answers 500 when WhatsApp refuses", func(t *testing.T) {
		h := newHarness()
		h.api.err = errors.New("boom")
		if rec := h.do("POST", "/api/edit", `{"chat_jid":"1@s.whatsapp.net","message_id":"ABC","message":"x"}`); rec.Code != 500 {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("rejects an invalid JID or body without sending", func(t *testing.T) {
		for _, body := range []string{`{`, `{"chat_jid":"a.b.c.d@s.whatsapp.net","message_id":"ABC","message":"x"}`} {
			h := newHarness()
			if rec := h.do("POST", "/api/edit", body); rec.Code != 400 || len(h.api.sent) != 0 {
				t.Errorf("body %s: status %d", body, rec.Code)
			}
		}
	})
}

func TestGroupCreateHandler(t *testing.T) {
	t.Run("creates the group and returns its JID", func(t *testing.T) {
		h := newHarness()
		rec := h.do("POST", "/api/group/create", `{"name":"Team","participants":["1@s.whatsapp.net","2@s.whatsapp.net"]}`)
		if rec.Code != 200 || decode(t, rec).Message != "120363000000000001@g.us" {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		if h.api.groupRequest.Name != "Team" || len(h.api.groupRequest.Participants) != 2 {
			t.Errorf("request %+v", h.api.groupRequest)
		}
	})
	t.Run("answers 500 when creation fails", func(t *testing.T) {
		h := newHarness()
		h.api.err = errors.New("boom")
		if rec := h.do("POST", "/api/group/create", `{"name":"Team","participants":[]}`); rec.Code != 500 {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("rejects an invalid participant without creating", func(t *testing.T) {
		h := newHarness()
		if rec := h.do("POST", "/api/group/create", `{"name":"Team","participants":["a.b.c.d@s.whatsapp.net"]}`); rec.Code != 400 || h.api.groupRequest.Name != "" {
			t.Errorf("status %d", rec.Code)
		}
	})
}

func TestGroupParticipantsHandler(t *testing.T) {
	for _, action := range []string{"add", "remove", "promote", "demote"} {
		t.Run("passes the "+action+" action through", func(t *testing.T) {
			h := newHarness()
			rec := h.do("POST", "/api/group/participants", `{"group":"120363000000000001@g.us","participants":["1@s.whatsapp.net"],"action":"`+action+`"}`)
			if rec.Code != 200 || decode(t, rec).Message != action {
				t.Fatalf("status %d body %s", rec.Code, rec.Body)
			}
			if string(h.api.groupAction) != action || h.api.groupChanged.User != "120363000000000001" || len(h.api.groupMembers) != 1 {
				t.Errorf("action %q group %v members %v", h.api.groupAction, h.api.groupChanged, h.api.groupMembers)
			}
		})
	}
	t.Run("answers 500 when the change fails", func(t *testing.T) {
		h := newHarness()
		h.api.err = errors.New("boom")
		if rec := h.do("POST", "/api/group/participants", `{"group":"120363000000000001@g.us","participants":[],"action":"add"}`); rec.Code != 500 {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("rejects an invalid group or participant", func(t *testing.T) {
		for _, body := range []string{`{`, `{"group":"a.b.c.d@g.us","participants":[],"action":"add"}`, `{"group":"120363000000000001@g.us","participants":["a.b.c.d@s.whatsapp.net"],"action":"add"}`} {
			h := newHarness()
			if rec := h.do("POST", "/api/group/participants", body); rec.Code != 400 || h.api.groupAction != "" {
				t.Errorf("body %s: status %d", body, rec.Code)
			}
		}
	})
}
