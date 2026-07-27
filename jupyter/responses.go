package jupyter

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Status represents possible status values for reply messages.
// https://jupyter-protocol.readthedocs.io/en/latest/messaging.html#request-reply
type Status string

const (
	// StatusOk indicates that the request was processed successfully.
	StatusOk Status = "ok"

	// StatusError indicates that the request failed due to an error.
	// Additional error information should be present in the reply.
	StatusError Status = "error"

	// StatusAbort indicates that the request is aborted.
	// Deprecated in version 5.1; kernels should send StatusError instead.
	StatusAbort Status = "abort"
)

// ExecutionResult represents the result of a code execution request.
// https://jupyter-protocol.readthedocs.io/en/latest/messaging.html#execution-results
type ExecutionResult struct {
	// Status indicates the result status and can be one of: 'ok', 'error', or 'abort'.
	Status Status `json:"status"`

	// ExecutionCount is the global kernel counter that increases with each request storing history.
	// Typically used by clients to display prompt numbers to the user.
	// If the request did not store history, this will be the current value of the counter in the kernel.
	ExecutionCount int `json:"execution_count"`

	// Payload is a list of payload dictionaries (optional and considered deprecated).
	// Each payload dict must have a 'source' key, classifying the payload (e.g., 'page').
	Payload []map[string]any `json:"payload,omitempty"`

	// UserExpressions contains results for user_expressions if the status is 'ok'.
	UserExpressions map[string]DisplayData `json:"user_expressions,omitempty"`

	// Metadata is a dictionary containing additional metadata associated with the execution result.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// DisplayData represents a message type for displaying data.
type DisplayData struct {
	// Data contains key/value pairs where keys are MIME types,
	// and values are the raw data of the representation in that format.
	Data map[string]any `json:"data"`

	// Metadata is any metadata that describes the data.
	Metadata map[string]any `json:"metadata"`

	// Transient contains optional transient data introduced in version 5.1.
	// This information is not persisted to a notebook or other documents
	// and is intended to live only during a live kernel session.
	Transient map[string]any `json:"transient"`
}

// InspectReply represents the content of an inspect_reply message in the Jupyter protocol.
type InspectReply struct {
	// Status indicates whether the request succeeded ('ok') or encountered an error ('error').
	Status string `json:"status"`

	// Found is true if an object was found, false otherwise.
	Found bool `json:"found"`

	// Data is a dictionary containing information about the inspected object.
	// It can be empty if nothing is found.
	Data map[string]any `json:"data"`

	// Metadata is a dictionary containing additional metadata associated with the inspection result.
	Metadata map[string]any `json:"metadata"`
}

// CompleteReply represents the content of a complete_reply message in the Jupyter protocol.
type CompleteReply struct {
	// Matches is the list of all matches to the completion request.
	// Example: ['a.isalnum', 'a.isalpha'] for the provided code context.
	Matches []string `json:"matches"`

	// CursorStart is the start position of the text that should be replaced by the completion matches.
	// Typically, CursorEnd is the same as CursorPos in the request.
	CursorStart int `json:"cursor_start"`

	// CursorEnd is the end position of the text that should be replaced by the completion matches.
	CursorEnd int `json:"cursor_end"`

	// Metadata is information that frontend plugins might use for extra display information about completions.
	Metadata map[string]any `json:"metadata"`

	// Status should be 'ok' unless an exception was raised during the request.
	// If there is an error, Status will be 'error' along with the usual error message content.
	Status string `json:"status"`
}

// HistoryItem represents a single history item with session, line number, and optional output.
type HistoryItem struct {
	Session    int
	LineNumber int
	Input      string
	Output     any
}

// HistoryReply represents the content of a history_reply message in the Jupyter protocol.
type HistoryReply struct {
	// History is a list of history items.
	History []HistoryItem `json:"history"`
}

// UnmarshalJSON implements the json.Unmarshaler interface for HistoryItem.
func (item *HistoryItem) UnmarshalJSON(data []byte) error {
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if len(raw) < 3 {
		return errors.New("invalid history item format")
	}

	var ok bool
	item.Session, ok = anyToInt(raw[0])
	if !ok {
		return fmt.Errorf("invalid history item: %#v", raw)
	}

	item.LineNumber, ok = anyToInt(raw[1])
	if !ok {
		return fmt.Errorf("invalid history item: %#v", raw)
	}

	switch io := raw[2].(type) {
	case string:
		item.Input = io
	case []any:
		if len(io) != 2 {
			return fmt.Errorf("invalid history item: %#v", raw)
		}
		item.Input, _ = io[0].(string)
		item.Output, _ = io[1].(string)
	default:
		return fmt.Errorf("invalid history item: %#v", raw)
	}

	if len(raw) > 3 {
		item.Output = raw[3]
	}

	return nil
}

func anyToInt(num any) (int, bool) {
	switch n := num.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// MarshalJSON implements the json.Marshaler interface for HistoryItem.
func (item *HistoryItem) MarshalJSON() ([]byte, error) {
	var raw []any
	raw = append(raw, item.Session, item.LineNumber)

	if item.Output == nil {
		raw = append(raw, item.Input)
	} else {
		raw = append(raw, []any{item.Input, item.Output})
	}

	return json.Marshal(raw)
}
