package memory

import "errors"

var (
	ErrInvalidPath     = errors.New("invalid memory path")
	ErrInvalidSchema   = errors.New("invalid Markdown schema")
	ErrSectionNotFound = errors.New("section not found")
	ErrSectionNotLeaf  = errors.New("section is not a leaf")
	ErrDrift           = errors.New("memory file drifted")
	ErrTargetDirty     = errors.New("memory target has Git changes")
)

type Scope struct {
	Path string
}

type Target struct {
	Path      string
	SectionID string
}

type Section struct {
	ID          string
	Heading     string
	Level       int
	Body        string
	ContentHash string
	Leaf        bool
}

type Snapshot struct {
	Path     string
	FileHash string
	Head     string
	Blob     string
	Sections []Section
}

type ChangeSet struct {
	Target       Target
	BaselineHash string
	Replacement  string
}

type Receipt struct {
	Target        Target
	BaselineHash  string
	PostStateHash string
}

type PreviewRequest struct {
	Target      Target
	Replacement string
}

type Preview struct {
	Target        Target
	BaselineHash  string
	PostStateHash string
	Head          string
	Blob          string
	Baseline      []byte
	PostState     []byte
}

type ReconcileRequest struct {
	Target        Target
	Replacement   string
	BaselineHash  string
	PostStateHash string
	Head          string
	Blob          string
	Preflight     func() error
	Validate      func(baseline, postState []byte) error
	Mode          ReconcileMode
}

type ReconcileMode uint8

const (
	ReconcileOnly ReconcileMode = iota
	ApplyBaseline
)

type ReconcileState uint8

const (
	ReconcileBaseline ReconcileState = iota + 1
	ReconcilePostState
)

type ReconcileResult struct {
	Target    Target
	Baseline  []byte
	PostState []byte
	State     ReconcileState
}
