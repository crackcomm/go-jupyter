package jupyter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// Version of jupyter protocol.
	Version = "5.3"

	// ErrInvalidSignature is returned when received message with an invalid signature.
	ErrInvalidSignature = errors.New("jupyter protocol: invalid signature")
)

// Header represents a Jupyter message header structure.
// https://jupyter-protocol.readthedocs.io/en/latest/messaging.html#general-message-format
type Header struct {
	// MsgID is a unique identifier for the message, typically a UUID.
	MsgID string `json:"msg_id"`

	// Username is the username for the process that generated the message.
	Username string `json:"username"`

	// Session is a unique identifier for the session, typically a UUID.
	Session string `json:"session"`

	// Date is an ISO 8601 timestamp for when the message is created.
	Date string `json:"date"`

	// MsgType is the type of the message.
	MsgType string `json:"msg_type"`

	// Version is the message protocol version.
	Version string `json:"version"`
}

// RawMessage represents a Jupyter message structure.
// https://jupyter-protocol.readthedocs.io/en/latest/messaging.html#general-message-format
type RawMessage struct {
	// Header contains the message header.
	Header Header `json:"header"`

	// ParentHeader contains the header from the parent message.
	ParentHeader Header `json:"parent_header"`

	// Metadata contains any metadata associated with the message.
	Metadata map[string]any `json:"metadata"`

	// Content is the actual content of the message.
	// The structure depends on the message type.
	Content json.RawMessage `json:"content"`
}

// Message represents a Jupyter message structure.
// https://jupyter-protocol.readthedocs.io/en/latest/messaging.html#general-message-format
type Message struct {
	// Header contains the message header.
	Header Header `json:"header"`

	// ParentHeader contains the header from the parent message.
	ParentHeader Header `json:"parent_header"`

	// Metadata contains any metadata associated with the message.
	Metadata map[string]any `json:"metadata"`

	// Content is the actual content of the message.
	// The structure depends on the message type.
	Content any `json:"content"`
}

func (msg *Message) Encode(signKey SignKey) (parts [][]byte, err error) {
	parts = make([][]byte, 6)
	err = msg.EncodeTo(signKey, parts)
	return
}

func (msg *Message) EncodeTo(signKey SignKey, parts [][]byte) (err error) {
	if l := len(parts); l < 6 {
		return fmt.Errorf("out of range: got %d expected 6", l)
	}

	for i, v := range []any{msg.Header, msg.ParentHeader, msg.Metadata, msg.Content} {
		if v != nil {
			if parts[i+1], err = json.Marshal(v); err != nil {
				return err
			}
		}
	}

	// Sign the message.
	if signKey != nil {
		if err := signMessage(parts[1:], signKey, &parts[0]); err != nil {
			return err
		}
	}

	return
}

func signMessage(parts [][]byte, signKey SignKey, signature *[]byte) error {
	mac := hmac.New(sha256.New, signKey)
	for _, part := range parts {
		if _, err := mac.Write(part); err != nil {
			return err
		}
	}
	*signature = make([]byte, hex.EncodedLen(mac.Size()))
	hex.Encode(*signature, mac.Sum(nil))
	return nil
}

func (msg *RawMessage) Decode(parts [][]byte, signKey SignKey) error {
	index, ok := findIndex(parts, "<IDS|MSG>")
	if !ok {
		return fmt.Errorf("invalid raw message")
	}

	// Validate signature.
	if signKey != nil {
		if err := validateSignature(parts, index, signKey); err != nil {
			return err
		}
	}

	// Unmarshal contents.
	return unmarshalParts(parts, index+2, &msg.Header, &msg.ParentHeader, &msg.Metadata, &msg.Content)
}

func findIndex(parts [][]byte, target string) (int, bool) {
	for i, part := range parts {
		if string(part) == target {
			return i, true
		}
	}
	return 0, false
}

func validateSignature(parts [][]byte, index int, signKey SignKey) error {
	mac := hmac.New(sha256.New, signKey)
	for _, msgpart := range parts[index+2 : index+6] {
		if _, err := mac.Write(msgpart); err != nil {
			return err
		}
	}

	signature := make([]byte, hex.DecodedLen(len(parts[index+1])))
	_, err := hex.Decode(signature, parts[index+1])
	if err != nil {
		return err
	}

	if !hmac.Equal(mac.Sum(nil), signature) {
		return ErrInvalidSignature
	}

	return nil
}

func unmarshalParts(parts [][]byte, startIndex int, values ...any) error {
	for j, v := range values {
		if parts[startIndex+j] != nil {
			if err := json.Unmarshal(parts[startIndex+j], v); err != nil {
				return err
			}
		}
	}
	return nil
}
