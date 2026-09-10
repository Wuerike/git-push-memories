package memory

import (
	"errors"
	"fmt"
	"os"
)

var (
	ErrInvalidPath        = errors.New("invalid memory path")
	ErrInvalidSchema      = errors.New("invalid Markdown schema")
	ErrSectionNotFound    = errors.New("section not found")
	ErrSectionNotLeaf     = errors.New("section is not a leaf")
	ErrDrift              = errors.New("memory file drifted")
	ErrTargetDirty        = errors.New("memory target has Git changes")
	ErrTargetExists       = errors.New("memory target already exists")
	ErrInvalidID          = errors.New("invalid generated memory ID")
	ErrGeneratedIDTooLong = fmt.Errorf("%w: generated section ID exceeds 64 characters", ErrInvalidID)
	ErrSectionExists      = errors.New("memory section already exists")
	ErrPageNotEligible    = errors.New("memory page is not eligible for removal")
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
	SectionHash   string
	PostStateHash string
	Head          string
	Blob          string
	Baseline      []byte
	PostState     []byte
	Diff          string
	FileMode      os.FileMode
}

type CreatePageRequest struct {
	TargetPath string
	Title      string
	Body       string
}

type CreatePagePreview struct {
	Target        Target
	PageID        string
	PostStateHash string
	Head          string
	PostState     []byte
	Diff          string
}

type ReconcileCreatePageRequest struct {
	CreatePageRequest
	PostStateHash string
	Head          string
	Preflight     func() error
	Validate      func(postState []byte) error
	Mode          ReconcileMode
}

type CreateSectionRequest struct {
	TargetPath string
	Heading    string
	Body       string
}

type ReconcileCreateSectionRequest struct {
	CreateSectionRequest
	ExpectedSectionID string
	BaselineHash      string
	PostStateHash     string
	Head              string
	Blob              string
	Preflight         func() error
	Validate          func(baseline, postState []byte) error
	Mode              ReconcileMode
}

type RemoveSectionRequest struct {
	Target      Target
	ContentHash string
}

type ReconcileRemoveSectionRequest struct {
	RemoveSectionRequest
	BaselineHash  string
	PostStateHash string
	Head          string
	Blob          string
	Preflight     func() error
	Validate      func(baseline, postState []byte) error
	Mode          ReconcileMode
}

type RemovePageRequest struct {
	Target      Target
	ContentHash string
}

type ReconcileRemovePageRequest struct {
	RemovePageRequest
	BaselineHash  string
	PostStateHash string
	Head          string
	Blob          string
	FileMode      os.FileMode
	Preflight     func() error
	Validate      func(baseline, postState []byte) error
	Mode          ReconcileMode
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

type RollbackRequest struct {
	TargetPath string
	Baseline   []byte
	PostState  []byte
	FileMode   os.FileMode
}
