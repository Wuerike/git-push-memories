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
