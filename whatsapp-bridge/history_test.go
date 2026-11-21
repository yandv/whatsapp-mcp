package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

type fakeHistoryAPI struct {
	anchor  *types.MessageInfo
	count   int
	sent    []*waProto.Message
	sendErr error
	built   *waProto.Message
}

func (f *fakeHistoryAPI) BuildHistorySyncRequest(anchor *types.MessageInfo, count int) *waProto.Message {
	f.anchor, f.count = anchor, count
	return f.built
}

func (f *fakeHistoryAPI) SendPeerMessage(_ context.Context, message *waProto.Message) (whatsmeow.SendResponse, error) {
	f.sent = append(f.sent, message)
	return whatsmeow.SendResponse{}, f.sendErr
}

func TestHistoryHandler(t *testing.T) {
	oldestAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	found := func(chatJID string) (types.MessageInfo, error) {
		info := types.MessageInfo{}
		info.ID, info.IsFromMe, info.Timestamp = "OLDEST", true, oldestAt
		return info, nil
	}
	missing := func(string) (types.MessageInfo, error) { return types.MessageInfo{}, errors.New("no rows") }

	rejected := []struct {
		name   string
		body   string
		lookup oldestMessageFunc
		status int
	}{
		{"malformed json", `{`, found, http.StatusBadRequest},
		{"empty body object", `{}`, found, http.StatusBadRequest},
		{"empty chat_jid", `{"chat_jid":""}`, found, http.StatusBadRequest},
		{"chat_jid with the wrong type", `{"chat_jid":5}`, found, http.StatusBadRequest},
		{"chat_jid that is not a jid", `{"chat_jid":"a.b.c.d@s.whatsapp.net"}`, found, http.StatusBadRequest},
		{"chat with no stored messages", `{"chat_jid":"1@s.whatsapp.net"}`, missing, http.StatusNotFound},
	}
	for _, tc := range rejected {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			api := &fakeHistoryAPI{}
			mux := http.NewServeMux()
			registerHistoryRoute(mux, api, tc.lookup)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/history", strings.NewReader(tc.body)))
			if rec.Code != tc.status {
				t.Errorf("status %d, want %d (%q)", rec.Code, tc.status, rec.Body.String())
			}
			if len(api.sent) != 0 {
				t.Errorf("sent %d peer messages, want none", len(api.sent))
			}
		})
	}

	accepted := []struct {
		name      string
		body      string
		wantCount int
	}{
		{"defaults the count to 50", `{"chat_jid":"1@s.whatsapp.net"}`, 50},
		{"treats a zero count as the default", `{"chat_jid":"1@s.whatsapp.net","count":0}`, 50},
		{"keeps an explicit count", `{"chat_jid":"1@s.whatsapp.net","count":7}`, 7},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeHistoryAPI{built: &waProto.Message{}}
			mux := http.NewServeMux()
			registerHistoryRoute(mux, api, found)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/history", strings.NewReader(tc.body)))
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"success":true`) || !strings.Contains(rec.Body.String(), `"oldest_known":"2026-03-04T05:06:07Z"`) {
				t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
			}
			if api.count != tc.wantCount {
				t.Errorf("count %d, want %d", api.count, tc.wantCount)
			}
			if api.anchor.ID != "OLDEST" || api.anchor.Chat != types.NewJID("1", types.DefaultUserServer) || !api.anchor.Timestamp.Equal(oldestAt) {
				t.Errorf("anchor %+v", api.anchor)
			}
			if len(api.sent) != 1 || api.sent[0] != api.built {
				t.Errorf("sent %v, want the built request once", api.sent)
			}
		})
	}

	t.Run("answers 500 when the peer message fails", func(t *testing.T) {
		api := &fakeHistoryAPI{built: &waProto.Message{}, sendErr: errors.New("not connected")}
		mux := http.NewServeMux()
		registerHistoryRoute(mux, api, found)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/history", strings.NewReader(`{"chat_jid":"1@s.whatsapp.net"}`)))
		if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "not connected") {
			t.Errorf("status %d body %q", rec.Code, rec.Body.String())
		}
	})
}

func TestOldestMessage(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("returns the earliest message of the requested chat only", func(t *testing.T) {
		store := newTestStore(t)
		for _, chat := range []string{"1@s.whatsapp.net", "2@s.whatsapp.net"} {
			if err := store.StoreChat(chat, "x", base); err != nil {
				t.Fatal(err)
			}
		}
		store.StoreMessage("NEW", "1@s.whatsapp.net", "1", "b", base.Add(time.Hour), false, "", "", "", nil, nil, nil, 0)
		store.StoreMessage("OLD", "1@s.whatsapp.net", "1", "a", base, true, "", "", "", nil, nil, nil, 0)
		store.StoreMessage("OTHER", "2@s.whatsapp.net", "2", "c", base.Add(-time.Hour), false, "", "", "", nil, nil, nil, 0)

		oldest, err := store.OldestMessage("1@s.whatsapp.net")
		if err != nil || oldest.ID != "OLD" || !oldest.IsFromMe || !oldest.Timestamp.Equal(base) {
			t.Errorf("oldest %+v err %v", oldest, err)
		}
	})

	t.Run("fails for a chat without messages", func(t *testing.T) {
		store := newTestStore(t)
		if _, err := store.OldestMessage("1@s.whatsapp.net"); err == nil {
			t.Error("want an error")
		}
	})
}
