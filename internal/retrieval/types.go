package retrieval

import "errors"

var (
	ErrInvalidQuery = errors.New("query must contain text")
	ErrInvalidScope = errors.New("invalid memory path scope")
	ErrInvalidRange = errors.New("invalid line range or result limit")
)

func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrInvalidQuery):
		return "invalid_query"
	case errors.Is(err, ErrInvalidScope):
		return "invalid_scope"
	case errors.Is(err, ErrInvalidRange):
		return "invalid_range"
	default:
		return "read_failed"
	}
}

type Scope struct {
	PathPrefix string `json:"path_prefix,omitempty"`
}

type Item struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
}

type ContextBundle struct {
	Revision  string `json:"revision"`
	Items     []Item `json:"items"`
	Truncated bool   `json:"truncated"`
}

type FileContent struct {
	Revision  string `json:"revision"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}
