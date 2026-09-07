package intake

const (
	OperationReplaceSection = "replace_section_content"
	OperationCreatePage     = "create_page"
	OperationCreateSection  = "create_section"
)

type Candidate struct {
	raw []byte
}

func NewCandidate(raw []byte) Candidate {
	return Candidate{raw: append([]byte(nil), raw...)}
}

type AllowedCandidate struct {
	operation   string
	targetPath  string
	sectionID   string
	replacement string
	title       string
	heading     string
	body        string
	sourcePaths []string
}

func (candidate AllowedCandidate) Operation() string   { return candidate.operation }
func (candidate AllowedCandidate) TargetPath() string  { return candidate.targetPath }
func (candidate AllowedCandidate) SectionID() string   { return candidate.sectionID }
func (candidate AllowedCandidate) Replacement() string { return candidate.replacement }
func (candidate AllowedCandidate) Title() string       { return candidate.title }
func (candidate AllowedCandidate) Heading() string     { return candidate.heading }
func (candidate AllowedCandidate) Body() string        { return candidate.body }
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
