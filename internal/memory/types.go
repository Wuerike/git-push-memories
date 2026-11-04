package memory

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidPath        = errors.New("invalid memory path")
	ErrInvalidSchema      = errors.New("invalid Markdown schema")
	ErrInvalidID          = errors.New("invalid generated memory ID")
	ErrGeneratedIDTooLong = fmt.Errorf("%w: generated section ID exceeds 64 characters", ErrInvalidID)
	ErrSectionExists      = errors.New("memory section already exists")
)

type Scope struct {
	Path string
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

// Page is one whole canonical page; ContentHash is the file hash used by Snapshot.FileHash.
type Page struct {
	Path        string
	Markdown    string
	ContentHash string
}
