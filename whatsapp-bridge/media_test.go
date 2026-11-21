package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/util/cbcutil"
	"go.mau.fi/whatsmeow/util/hkdfutil"
)

// Builds a CDN payload the way WhatsApp does: AES-CBC ciphertext followed by a 10 byte truncated HMAC over iv+ciphertext.
func encryptForCDN(t *testing.T, plain, mediaKey []byte, mediaType whatsmeow.MediaType) []byte {
	t.Helper()
	keys := hkdfutil.SHA256(mediaKey, nil, []byte(mediaType), 112)
	iv, cipherKey, macKey := keys[:16], keys[16:48], keys[48:80]
	encrypted, err := cbcutil.Encrypt(cipherKey, iv, plain)
	if err != nil {
		t.Fatal(err)
	}
	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(encrypted)
	return append(encrypted, h.Sum(nil)[:10]...)
}

func TestDecryptReuploadedMedia(t *testing.T) {
	mediaKey := make([]byte, 32)
	for i := range mediaKey {
		mediaKey[i] = byte(i + 1)
	}
	plain := []byte("a reuploaded picture")
	plainSum := sha256.Sum256(plain)

	t.Run("returns the plaintext when mac and checksum match", func(t *testing.T) {
		data := encryptForCDN(t, plain, mediaKey, whatsmeow.MediaImage)
		got, err := decryptReuploadedMedia(data, mediaKey, plainSum[:], whatsmeow.MediaImage, "/v/t62/x", "image/jpeg")
		if err != nil || string(got) != string(plain) {
			t.Errorf("got %q err %v", got, err)
		}
	})

	flipped := encryptForCDN(t, plain, mediaKey, whatsmeow.MediaImage)
	flipped[0] ^= 0xff
	badMAC := encryptForCDN(t, plain, mediaKey, whatsmeow.MediaImage)
	badMAC[len(badMAC)-1] ^= 0xff
	otherSum := sha256.Sum256([]byte("something else"))

	rejected := []struct {
		name      string
		data      []byte
		mediaType whatsmeow.MediaType
		fileSHA   []byte
		want      error
	}{
		{"an empty body", nil, whatsmeow.MediaImage, plainSum[:], whatsmeow.ErrTooShortFile},
		{"a body of exactly the mac length", make([]byte, 10), whatsmeow.MediaImage, plainSum[:], whatsmeow.ErrTooShortFile},
		{"a tampered ciphertext", flipped, whatsmeow.MediaImage, plainSum[:], whatsmeow.ErrInvalidMediaHMAC},
		{"a tampered mac", badMAC, whatsmeow.MediaImage, plainSum[:], whatsmeow.ErrInvalidMediaHMAC},
		{"a payload for another media type", encryptForCDN(t, plain, mediaKey, whatsmeow.MediaVideo), whatsmeow.MediaImage, plainSum[:], whatsmeow.ErrInvalidMediaHMAC},
		{"a plaintext that does not match file_sha256", encryptForCDN(t, plain, mediaKey, whatsmeow.MediaImage), whatsmeow.MediaImage, otherSum[:], whatsmeow.ErrInvalidMediaSHA256},
	}
	for _, tc := range rejected {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			got, err := decryptReuploadedMedia(tc.data, mediaKey, tc.fileSHA, tc.mediaType, "/v/t62/x", "image/jpeg")
			if !errors.Is(err, tc.want) || got != nil {
				t.Errorf("got %q err %v, want %v", got, err, tc.want)
			}
		})
	}
}
