package memory

import (
	"fmt"

	"github.com/Wuerike/git-push-memories/internal/markdown"
)

// PrepareDocument completes missing section IDs for a directly edited page.
// Existing markers remain authoritative, including IDs added by an earlier check.
// It returns the complete canonical document without writing or committing it.
func PrepareDocument(targetPath string, baseline, current []byte) ([]byte, error) {
	source, err := normalizePageReplacement(string(current))
	if err != nil {
		return nil, err
	}
	var frontmatter []byte
	if len(baseline) > 0 {
		sections, err := validateDocument(baseline)
		if err != nil {
			return nil, err
		}
		frontmatter = baseline[:sections[0].ByteStart]
	}
	sections, err := markdown.ParseSections(source)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}
	known := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		if section.AnchorID != "" {
			known[section.AnchorID] = struct{}{}
		}
	}
	source, err = injectSectionIDs(source, targetPath, frontmatter, known)
	if err != nil {
		return nil, err
	}
	if _, err := validateDocument(source); err != nil {
		return nil, err
	}
	return source, nil
}
