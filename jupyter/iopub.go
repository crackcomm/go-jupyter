package jupyter

import (
	"encoding/json"
	"fmt"
)

// StreamMessage represents the content of a stream message in the Jupyter protocol.
type StreamMessage struct {
	// Name of the stream, one of 'stdout', 'stderr'.
	Name string `json:"name"`

	// Text is an arbitrary string to be written to the stream.
	Text string `json:"text"`
}

// DisplayDataMessage represents the content of a display_data message in the Jupyter protocol.
type DisplayDataMessage struct {
	// Data contains key/value pairs, where keys are MIME types, and values are raw data of the representation in that format.
	Data map[string]interface{} `json:"data"`

	// Metadata contains any metadata that describes the data.
	Metadata map[string]interface{} `json:"metadata"`

	// Transient contains optional transient data introduced in 5.1.
	// This information is not persisted to a notebook or other documents and is intended to live only during a live kernel session.
	Transient map[string]interface{} `json:"transient"`
}

// UpdateDisplayDataMessage represents the content of an update_display_data message in the Jupyter protocol.
type UpdateDisplayDataMessage struct {
	// Data contains key/value pairs, where keys are MIME types, and values are raw data of the representation in that format.
	Data map[string]interface{} `json:"data"`

	// Metadata contains any metadata that describes the data.
	Metadata map[string]interface{} `json:"metadata"`

	// Transient contains information not to be persisted to a notebook or other environment.
	// Intended to live only during a kernel session.
	// The only transient key currently defined in Jupyter is display_id
	Transient map[string]interface{} `json:"transient"`
}

// ClearOutputMessage represents a Jupyter message for clearing output.
// This message type is used to clear the output, optionally waiting for new output to be available.
// Useful for creating animations with minimal flickering.
type ClearOutputMessage struct {
	// Wait indicates whether to clear the output immediately before new output is displayed.
	// If true, the output is cleared only when new output is available.
	Wait bool `json:"wait"`
}

// ExecuteInputMessage represents the content of an execute_input message in the Jupyter protocol.
type ExecuteInputMessage struct {
	// Code is the source code to be executed, one or more lines.
	Code string `json:"code"`

	// ExecutionCount is the counter for this execution.
	ExecutionCount int `json:"execution_count"`
}

// ExecuteResultMessage represents the content of an execute_result message in the Jupyter protocol.
type ExecuteResultMessage struct {
	// ExecutionCount is the counter for this execution.
	ExecutionCount int `json:"execution_count"`

	// Data and Metadata are identical to a display_data message.
	// The object being displayed is that passed to the display hook, i.e., the result of the execution.
	Data     map[string]interface{} `json:"data"`
	Metadata map[string]interface{} `json:"metadata"`
}

// ErrorMessage represents the content of an error message in the Jupyter protocol.
type ErrorMessage struct {
	// EName is the exception name, as a string.
	EName string `json:"ename"`

	// EValue is the exception value, as a string.
	EValue string `json:"evalue"`

	// Traceback is a list of traceback frames as strings.
	Traceback []string `json:"traceback"`
}

// KernelState represents possible execution states for the kernel.
// https://jupyter-protocol.readthedocs.io/en/latest/messaging.html#kernel-status
type KernelState string

const (
	// StateBusy indicates that the kernel is currently busy processing a request.
	StateBusy KernelState = "busy"

	// StateIdle indicates that the kernel is idle and not processing any requests.
	StateIdle KernelState = "idle"

	// StateStarting indicates that the kernel is in the starting state (published exactly once at process startup).
	StateStarting KernelState = "starting"
)

// StatusMessage represents the content of a status message in the Jupyter protocol.
type StatusMessage struct {
	// ExecutionState represents the state of the kernel: 'busy', 'idle', 'starting'.
	ExecutionState KernelState `json:"execution_state"`
}

func parseContent(msgType string, content json.RawMessage) (interface{}, error) {
	target, err := createTarget(msgType)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(content, target); err != nil {
		return nil, err
	}

	return target, nil
}

func createTarget(msgType string) (interface{}, error) {
	switch msgType {
	case "stream":
		return new(StreamMessage), nil
	case "display_data":
		return new(DisplayDataMessage), nil
	case "update_display_data":
		return new(UpdateDisplayDataMessage), nil
	case "clear_output":
		return new(ClearOutputMessage), nil
	case "execute_input":
		return new(ExecuteInputMessage), nil
	case "execute_result":
		return new(ExecuteResultMessage), nil
	case "error":
		return new(ErrorMessage), nil
	case "status":
		return new(StatusMessage), nil
	default:
		return nil, fmt.Errorf("Unknown message type: %s", msgType)
	}
}
