package proposal

import (
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("proposal not found")
	ErrInvalidReason = errors.New("invalid rejection reason")
	ErrNotApplied    = errors.New("proposal was not applied")
	ErrWrongRevision = errors.New("proposal revision does not match")
	ErrTerminal      = errors.New("proposal already has a terminal decision")
	ErrIntegrity     = errors.New("proposal integrity check failed")
)

const (
	OperationReplaceSection = "replace_section_content"
	OperationCreatePage     = "create_page"
	OperationCreateSection  = "create_section"
)

type Target struct {
	Path      string `json:"path"`
	SectionID string `json:"section_id"`
}

type Operation struct {
	Type        string `json:"type"`
	Target      Target `json:"target"`
	Replacement string `json:"replacement,omitempty"`
	Title       string `json:"title,omitempty"`
	Heading     string `json:"heading,omitempty"`
	Body        string `json:"body,omitempty"`
}

type Baseline struct {
	Exists   *bool  `json:"exists,omitempty"`
	FileHash string `json:"file_hash,omitempty"`
	Head     string `json:"head"`
	Blob     string `json:"blob,omitempty"`
}

type PostState struct {
	FileHash string `json:"file_hash"`
}

type View struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Nonce         string    `json:"nonce"`
	Revision      int       `json:"revision"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	Operation     Operation `json:"operation"`
	Baseline      Baseline  `json:"baseline"`
	PostState     PostState `json:"post_state"`
	Diff          string    `json:"diff"`
	SourcePaths   []string  `json:"source_paths"`
	IntegrityHash string    `json:"integrity_hash"`
}

type DecisionReceipt struct {
	SchemaVersion int       `json:"schema_version"`
	ProposalID    string    `json:"proposal_id"`
	Revision      int       `json:"revision"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason"`
	DecidedAt     time.Time `json:"decided_at"`
}

type Review struct {
	Proposal  *View               `json:"proposal,omitempty"`
	Decision  *DecisionReceipt    `json:"decision,omitempty"`
	Applied   *ApplyReceipt       `json:"applied,omitempty"`
	Published *PublicationReceipt `json:"published,omitempty"`
}

type ApplyReceipt struct {
	SchemaVersion int       `json:"schema_version"`
	ProposalID    string    `json:"proposal_id"`
	Revision      int       `json:"revision"`
	Status        string    `json:"status"`
	AppliedAt     time.Time `json:"applied_at"`
	MemoryRoot    string    `json:"memory_root"`
	Target        Target    `json:"target"`
	Baseline      Baseline  `json:"baseline"`
	PostState     PostState `json:"post_state"`
}

type PublicationReceipt struct {
	SchemaVersion int       `json:"schema_version"`
	ProposalID    string    `json:"proposal_id"`
	Revision      int       `json:"revision"`
	Status        string    `json:"status"`
	PublishedAt   time.Time `json:"published_at"`
	MemoryRoot    string    `json:"memory_root"`
	Target        Target    `json:"target"`
	Branch        string    `json:"branch"`
	Parent        string    `json:"parent"`
	Commit        string    `json:"commit"`
	Blob          string    `json:"blob"`
}
