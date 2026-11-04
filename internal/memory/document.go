package memory

import (
	"bytes"
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

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

const maxRewriteBytes = 128 * 1024

func PageID(pagePath string) string {
	return strings.TrimSuffix(path.Base(pagePath), ".md")
}

func slugHeading(heading string) string {
	var slug strings.Builder
	separator := false
	for _, character := range strings.ToLower(heading) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteRune(character)
			separator = false
		} else if slug.Len() > 0 {
			separator = true
		}
	}
	return slug.String()
}

func hasDisallowedControl(value string) bool {
	for _, character := range value {
		if character == '\t' || character == '\n' {
			continue
		}
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func normalizePageReplacement(replacement string) ([]byte, error) {
	normalized := strings.ReplaceAll(replacement, "\r\n", "\n")
	if strings.TrimSpace(normalized) == "" || len(normalized) > maxRewriteBytes || !utf8.ValidString(normalized) ||
		hasDisallowedControl(normalized) || strings.Contains(normalized, "\r") || hasUnresolvedMerge([]byte(normalized)) {
		return nil, fmt.Errorf("%w: invalid page replacement", ErrInvalidSchema)
	}
	if !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	return []byte(normalized), nil
}

// injectSectionIDs checks the page structure against the baseline frontmatter and
// known section IDs, then adds a server-generated marker to every heading without one.
func injectSectionIDs(source []byte, targetPath string, frontmatter []byte, known map[string]struct{}) ([]byte, error) {
	sections, err := markdown.ParseSections(source)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}
	h1Count := 0
	for _, section := range sections {
		if section.HeadingLevel == 1 {
			h1Count++
		}
	}
	if len(sections) == 0 || h1Count != 1 || sections[0].HeadingLevel != 1 {
		return nil, fmt.Errorf("%w: page must start with exactly one H1", ErrInvalidSchema)
	}
	if lead := source[:sections[0].ByteStart]; len(lead) > 0 {
		if !bytes.Equal(lead, frontmatter) {
			return nil, fmt.Errorf("%w: frontmatter must match the baseline", ErrInvalidSchema)
		}
		source = source[len(lead):]
		if sections, err = markdown.ParseSections(source); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
		}
	}
	pageID := PageID(targetPath)
	used := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		if section.AnchorID == "" {
			continue
		}
		if section.HeadingLevel == 1 {
			if section.AnchorID != pageID {
				return nil, fmt.Errorf("%w: H1 ID must be %q", ErrInvalidID, pageID)
			}
		} else if _, ok := known[section.AnchorID]; !ok {
			return nil, fmt.Errorf("%w: unknown section ID %q", ErrInvalidID, section.AnchorID)
		}
		used[section.AnchorID] = struct{}{}
	}
	generated := make([]string, len(sections))
	for index, section := range sections {
		if section.AnchorID != "" {
			continue
		}
		next := source[section.HeadingEnd:]
		if end := bytes.IndexByte(next, '\n'); end >= 0 {
			next = next[:end]
		}
		if bytes.Contains(next, []byte("<!-- @id:")) {
			return nil, fmt.Errorf("%w: malformed section marker under %q", ErrInvalidSchema, section.HeadingText)
		}
		id := pageID
		if section.HeadingLevel != 1 {
			slug := slugHeading(section.HeadingText)
			if slug == "" {
				return nil, ErrInvalidID
			}
			id = pageID + "-" + slug
		}
		if len(id) > 64 {
			return nil, ErrGeneratedIDTooLong
		}
		if _, exists := used[id]; exists {
			return nil, ErrSectionExists
		}
		used[id] = struct{}{}
		generated[index] = id
	}
	for index := len(sections) - 1; index >= 0; index-- {
		if generated[index] == "" {
			continue
		}
		end := sections[index].HeadingEnd
		marker := "<!-- @id: " + generated[index] + " -->\n"
		source = append(source[:end:end], append([]byte(marker), source[end:]...)...)
	}
	return append(append([]byte(nil), frontmatter...), source...), nil
}
