package jupyter

var (
	RequestExecute = "execute_request"
	RequestInspect = "inspect_request"
	RequestHistory = "history_request"
)

// ExecutionRequest represents a request to execute source code by the kernel.
// https://jupyter-protocol.readthedocs.io/en/latest/messaging.html#execute
type ExecutionRequest struct {
	// Code to be executed by the kernel, one or more lines.
	Code string `json:"code"`

	// Silent, if true, signals the kernel to execute quietly without broadcasting output.
	// Defaults to false.
	Silent bool `json:"silent"`

	// StoreHistory, if true, signals the kernel to populate history.
	// Defaults to true if Silent is false.
	StoreHistory bool `json:"store_history"`

	// UserExpressions is a map of names to expressions to be evaluated in the user's dict.
	// The rich display-data representation of each will be evaluated after execution.
	// See the display_data content for the structure of the representation data.
	UserExpressions map[string]string `json:"user_expressions"`

	// AllowStdin, if true, indicates that the code running in the kernel can prompt the user for input.
	AllowStdin bool `json:"allow_stdin"`

	// StopOnError, if true, does not abort the execution queue if an exception is encountered.
	// Allows queued execution of multiple execute_requests, even if they generate exceptions.
	StopOnError bool `json:"stop_on_error"`
}

// IntrospectionRequest represents a request for code introspection.
// https://jupyter-protocol.readthedocs.io/en/latest/messaging.html#introspection
type IntrospectionRequest struct {
	// Code is the code context in which introspection is requested.
	// This may be up to an entire multiline cell.
	Code string `json:"code"`

	// CursorPos is the cursor position within 'Code' (in Unicode characters) where inspection is requested.
	CursorPos int `json:"cursor_pos"`

	// DetailLevel is the level of detail desired.
	// In IPython, 0 is equivalent to typing 'x?' at the prompt, 1 is equivalent to 'x??'.
	// The difference is up to kernels, but in IPython, level 1 includes the source code if available.
	DetailLevel int `json:"detail_level"`
}

// CompleteRequest represents the content of a complete_request message in the Jupyter protocol.
type CompleteRequest struct {
	// Code is the code context in which completion is requested.
	// This may be up to an entire multiline cell.
	// Example: 'foo = a.isal'
	Code string `json:"code"`

	// CursorPos is the cursor position within 'Code' (in Unicode characters) where completion is requested.
	CursorPos int `json:"cursor_pos"`
}

// HistoryRequest represents the content of a history_request message in the Jupyter protocol.
type HistoryRequest struct {
	// Output indicates whether to return output history in the resulting dictionary.
	Output bool `json:"output"`

	// Raw indicates whether to return the raw input history (true) or the transformed input (false).
	Raw bool `json:"raw"`

	// HistAccessType can be 'range', 'tail', or 'search'.
	HistAccessType string `json:"hist_access_type"`

	// Session is a number counting up each time the kernel starts.
	// Positive session number or negative number to count back from the current session.
	Session int `json:"session"`

	// If HistAccessType is 'range', get a range of input cells.
	// Start and Stop are line (cell) numbers within that session.
	Start int `json:"start"`
	Stop  int `json:"stop"`

	// If HistAccessType is 'tail' or 'search', get the last n cells.
	N int `json:"n"`

	// If HistAccessType is 'search', get cells matching the specified glob pattern (with * and ? as wildcards).
	Pattern string `json:"pattern"`

	// If HistAccessType is 'search' and Unique is true, do not include duplicated history. Default is false.
	Unique bool `json:"unique"`
}
