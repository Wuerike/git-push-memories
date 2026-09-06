package intake

type Candidate struct {
	raw []byte
}

func NewCandidate(raw []byte) Candidate {
	return Candidate{raw: append([]byte(nil), raw...)}
}

type AllowedCandidate struct {
	targetPath  string
	sectionID   string
	replacement string
	sourcePaths []string
}

func (candidate AllowedCandidate) TargetPath() string  { return candidate.targetPath }
func (candidate AllowedCandidate) SectionID() string   { return candidate.sectionID }
func (candidate AllowedCandidate) Replacement() string { return candidate.replacement }
func (candidate AllowedCandidate) SourcePaths() []string {
	result := make([]string, len(candidate.sourcePaths))
	copy(result, candidate.sourcePaths)
	return result
}

type Rejection struct {
	Class  string `json:"class"`
	Reason string `json:"reason"`
}

func (rejection Rejection) Error() string {
	return rejection.Class + ":" + rejection.Reason
}
