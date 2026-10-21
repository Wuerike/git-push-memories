package memory

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Wuerike/git-push-memories/internal/markdown"
	"github.com/Wuerike/git-push-memories/internal/unifieddiff"
)

const maxRewriteBytes = 128 * 1024

// ReadPage returns the whole canonical page under a shared lock, without writing.
func (repository *Repository) ReadPage(ctx context.Context, requestedPath string) (Page, error) {
	var page Page
	err := repository.withLock(ctx, false, func() error {
		state, err := repository.load(ctx, requestedPath)
		if err != nil {
			return err
		}
		page = Page{Path: state.snapshot.Path, Markdown: string(state.source), ContentHash: state.snapshot.FileHash}
		return nil
	})
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

// PreviewRewritePage computes the post-state of replacing a whole page, without writing.
func (repository *Repository) PreviewRewritePage(ctx context.Context, request RewritePageRequest) (Preview, error) {
	var preview Preview
	err := repository.withLock(ctx, false, func() error {
		state, err := repository.load(ctx, request.TargetPath)
		if err != nil {
			return err
		}
		if err := repository.ensureTargetClean(ctx, state.snapshot.Path, state.snapshot.Head); err != nil {
			return err
		}
		if state.snapshot.FileHash != request.ContentHash {
			return ErrDrift
		}
		postState, err := rewritePage(state.source, state.snapshot.Path, request.Replacement)
		if err != nil {
			return err
		}
		if err := repository.confirmPreviewState(ctx, state.snapshot); err != nil {
			return err
		}
		preview = Preview{
			Target:       Target{Path: state.snapshot.Path, SectionID: PageID(state.snapshot.Path)},
			BaselineHash: state.snapshot.FileHash, SectionHash: request.ContentHash, PostStateHash: hash(postState),
			Head: state.snapshot.Head, Blob: state.snapshot.Blob,
			Baseline: append([]byte(nil), state.source...), PostState: postState,
			Diff: unifieddiff.Between(state.snapshot.Path, state.source, postState),
		}
		return nil
	})
	return preview, err
}

// ReconcileRewritePage applies or confirms a previously previewed whole-page rewrite.
func (repository *Repository) ReconcileRewritePage(ctx context.Context, request ReconcileRewritePageRequest) (ReconcileResult, error) {
	target := Target{Path: request.TargetPath, SectionID: PageID(request.TargetPath)}
	return repository.reconcileSection(ctx, target, request.BaselineHash, request.PostStateHash, request.Head, request.Blob, request.Preflight, request.Validate, request.Mode,
		func(baseline []byte, _ []markdown.Section) ([]byte, error) {
			if hash(baseline) != request.ContentHash {
				return nil, ErrDrift
			}
			return rewritePage(baseline, request.TargetPath, request.Replacement)
		})
}

// rewritePage replaces the whole page with replacement, injecting server-generated
// section IDs for headings without a marker.
func rewritePage(baseline []byte, targetPath, replacement string) ([]byte, error) {
	source, err := normalizePageReplacement(replacement)
	if err != nil {
		return nil, err
	}
	baselineSections, err := markdown.ParseSections(baseline)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}
	if len(baselineSections) == 0 {
		return nil, fmt.Errorf("%w: baseline has no heading", ErrInvalidSchema)
	}
	frontmatter := baseline[:baselineSections[0].ByteStart]
	known := make(map[string]struct{}, len(baselineSections))
	for _, section := range baselineSections {
		if section.AnchorID != "" {
			known[section.AnchorID] = struct{}{}
		}
	}
	source, err = injectSectionIDs(source, targetPath, frontmatter, known)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(source, baseline) {
		return nil, fmt.Errorf("%w: rewrite produces no change", ErrInvalidSchema)
	}
	if _, err := validateDocument(source); err != nil {
		return nil, err
	}
	return source, nil
}

// composePage builds a new page from a complete replacement under the rewrite
// rules with an empty baseline: no frontmatter and no known section IDs.
func composePage(targetPath, replacement string) ([]byte, error) {
	source, err := normalizePageReplacement(replacement)
	if err != nil {
		return nil, err
	}
	if source, err = injectSectionIDs(source, targetPath, nil, nil); err != nil {
		return nil, err
	}
	if _, err := validateDocument(source); err != nil {
		return nil, err
	}
	return source, nil
}

func normalizePageReplacement(replacement string) ([]byte, error) {
	normalized := strings.ReplaceAll(replacement, "\r\n", "\n")
	if strings.TrimSpace(normalized) == "" || len(normalized) > maxRewriteBytes || !utf8.ValidString(normalized) ||
		hasDisallowedControl(normalized, true) || strings.Contains(normalized, "\r") || hasUnresolvedMerge([]byte(normalized)) {
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
