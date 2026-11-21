package main

import (
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// storedMedia holds the columns of a stored message that the quote bubble needs.
type storedMedia struct {
	Content       string
	MediaType     string
	URL           string
	MediaKey      []byte
	FileSHA256    []byte
	FileEncSHA256 []byte
	FileLength    uint64
}

// quotedParticipant decides who the quote attributes the message to. Only the user part of the sender is stored;
// in a direct chat it shares the chat's server (lid or phone), and our own messages are quoted under the identity
// that chat addresses us by.
func quotedParticipant(sender string, chat types.JID, isFromMe bool, ownJID, ownLID types.JID) types.JID {
	if isFromMe {
		if chat.Server == types.HiddenUserServer {
			return ownLID.ToNonAD()
		}
		return ownJID.ToNonAD()
	}
	if chat.Server == types.GroupServer {
		return types.NewJID(sender, types.DefaultUserServer)
	}
	return types.NewJID(sender, chat.Server)
}

// buildQuotedMessage rebuilds the quoted message body according to its media type.
func buildQuotedMessage(m storedMedia) *waProto.Message {
	quoted := &waProto.Message{}
	switch m.MediaType {
	case "image":
		quoted.ImageMessage = &waProto.ImageMessage{Caption: proto.String(m.Content), URL: proto.String(m.URL), MediaKey: m.MediaKey, FileSHA256: m.FileSHA256, FileEncSHA256: m.FileEncSHA256, FileLength: proto.Uint64(m.FileLength)}
	case "audio":
		quoted.AudioMessage = &waProto.AudioMessage{PTT: proto.Bool(true), Mimetype: proto.String("audio/ogg; codecs=opus"), URL: proto.String(m.URL), MediaKey: m.MediaKey, FileSHA256: m.FileSHA256, FileEncSHA256: m.FileEncSHA256, FileLength: proto.Uint64(m.FileLength)}
	case "video":
		quoted.VideoMessage = &waProto.VideoMessage{Caption: proto.String(m.Content), URL: proto.String(m.URL), MediaKey: m.MediaKey, FileSHA256: m.FileSHA256, FileEncSHA256: m.FileEncSHA256, FileLength: proto.Uint64(m.FileLength)}
	case "document":
		quoted.DocumentMessage = &waProto.DocumentMessage{Caption: proto.String(m.Content), URL: proto.String(m.URL), MediaKey: m.MediaKey, FileSHA256: m.FileSHA256, FileEncSHA256: m.FileEncSHA256, FileLength: proto.Uint64(m.FileLength)}
	default:
		quoted.Conversation = proto.String(m.Content)
	}
	return quoted
}
