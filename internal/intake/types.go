package intake

import "github.com/Wuerike/git-push-memories/internal/reference"

type ProposalCandidate struct {
	raw []byte
}

func NewProposalCandidate(raw []byte) ProposalCandidate {
	return ProposalCandidate{raw: append([]byte(nil), raw...)}
}

type AllowedProposal struct {
	title            string
	suggestedAction  string
	target           string
	knowledge        string
	sourcePaths      []string
	sourceReferences []reference.Reference
}

func (proposal AllowedProposal) Title() string           { return proposal.title }
func (proposal AllowedProposal) SuggestedAction() string { return proposal.suggestedAction }
func (proposal AllowedProposal) Target() string          { return proposal.target }
func (proposal AllowedProposal) Knowledge() string       { return proposal.knowledge }
func (proposal AllowedProposal) SourcePaths() []string {
	return append([]string(nil), proposal.sourcePaths...)
}
func (proposal AllowedProposal) SourceReferences() []reference.Reference {
	return append([]reference.Reference(nil), proposal.sourceReferences...)
}

type Rejection struct {
	Class  string `json:"class"`
	Reason string `json:"reason"`
}

func (rejection Rejection) Error() string {
	return rejection.Class + ":" + rejection.Reason
}
