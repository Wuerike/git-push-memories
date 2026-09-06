package retrieval

import "errors"

var (
	ErrInvalidQuery   = errors.New("invalid retrieval query")
	ErrInvalidScope   = errors.New("invalid retrieval scope")
	ErrInvalidBudget  = errors.New("invalid retrieval budget")
	ErrBudgetTooSmall = errors.New("retrieval budget too small")
	ErrStaleHandle    = errors.New("stale retrieval handle")
)

func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrInvalidQuery):
		return "invalid_query"
	case errors.Is(err, ErrInvalidScope):
		return "invalid_scope"
	case errors.Is(err, ErrInvalidBudget):
		return "invalid_budget"
	case errors.Is(err, ErrBudgetTooSmall):
		return "budget_too_small"
	case errors.Is(err, ErrStaleHandle):
		return "stale_handle"
	default:
		return "retrieval_failed"
	}
}

type Scope struct {
	PathPrefix string `json:"path_prefix,omitempty"`
}

type Handle struct {
	Path        string `json:"path"`
	SectionID   string `json:"section_id"`
	ContentHash string `json:"content_hash"`
}

type Item struct {
	Handle  Handle `json:"handle"`
	Heading string `json:"heading"`
	Content string `json:"content"`
	Score   int    `json:"score,omitempty"`
}

type ContextBundle struct {
	Items     []Item `json:"items"`
	Truncated bool   `json:"truncated"`
}
