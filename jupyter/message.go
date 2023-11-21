package jupyter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var (
	// Version of jupyter protocol.
	Version = "5.3"

	// ErrInvalidSignature is returned when received message with an invalid signature.
	ErrInvalidSignature = errors.New("Invalid jupyter protocol signature")
)

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
	Metadata map[string]interface{} `json:"metadata"`

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
	Metadata map[string]interface{} `json:"metadata"`

	// Content is the actual content of the message.
	// The structure depends on the message type.
	Content interface{} `json:"content"`
}

func (msg *Message) Encode(signKey []byte) (parts [][]byte, err error) {
	parts = make([][]byte, 6)

	for i, v := range []interface{}{msg.Header, msg.ParentHeader, msg.Metadata, msg.Content} {
		if v != nil {
			if parts[1+i], err = json.Marshal(v); err != nil {
				return
			}
		}
	}

	// Sign the message.
	if signKey != nil {
		if err = signMessage(parts[1:], signKey, &parts[0]); err != nil {
			return
		}
	}

	return
}

func signMessage(parts [][]byte, signKey []byte, signature *[]byte) (err error) {
	mac := hmac.New(sha256.New, signKey)
	for _, part := range parts {
		mac.Write(part)
	}
	*signature = make([]byte, hex.EncodedLen(mac.Size()))
	hex.Encode(*signature, mac.Sum(nil))
	return
}

func (msg *Message) Decode(parts [][]byte, signKey []byte) (err error) {
	var raw RawMessage
	if err = raw.Decode(parts, signKey); err != nil {
		return
	}
	if err = json.Unmarshal(raw.Content, &msg.Content); err != nil {
		return
	}
	return
}

func (msg *RawMessage) Decode(parts [][]byte, signKey []byte) error {
	index, err := findIndex(parts, "<IDS|MSG>")
	if err != nil {
		return err
	}

	// Validate signature.
	if err := validateSignature(parts, index, signKey); err != nil {
		return err
	}

	// Unmarshal contents.
	return unmarshalParts(parts, index+2, &msg.Header, &msg.ParentHeader, &msg.Metadata, &msg.Content)
}

func findIndex(parts [][]byte, target string) (int, error) {
	for i, part := range parts {
		if string(part) == target {
			return i, nil
		}
	}
	return 0, errors.New("Target not found in parts")
}

func validateSignature(parts [][]byte, index int, signKey []byte) error {
	if signKey == nil {
		return nil
	}

	mac := hmac.New(sha256.New, signKey)
	for _, msgpart := range parts[index+2 : index+6] {
		mac.Write(msgpart)
	}

	signature := make([]byte, hex.DecodedLen(len(parts[index+1])))
	hex.Decode(signature, parts[index+1])

	if !hmac.Equal(mac.Sum(nil), signature) {
		return ErrInvalidSignature
	}

	return nil
}

func unmarshalParts(parts [][]byte, startIndex int, values ...interface{}) error {
	for j, v := range values {
		if parts[startIndex+j] != nil {
			if err := json.Unmarshal(parts[startIndex+j], v); err != nil {
				return err
			}
		}
	}
	return nil
}
