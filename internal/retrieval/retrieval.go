package retrieval

import (
	"bytes"
	"context"
	"path"
	"strings"

	"github.com/Wuerike/git-push-memories/internal/memory"
)

type Retriever struct {
	repository *memory.Repository
	categories []string
}

func New(root string, categories ...string) (*Retriever, error) {
	if len(categories) == 0 {
		return nil, ErrInvalidScope
	}
	repository, err := memory.Open(root)
	if err != nil {
		return nil, err
	}
	return &Retriever{repository: repository, categories: categories}, nil
}

func (r *Retriever) Close() error { return r.repository.Close() }

// Retrieve searches plain text, returning one excerpt per matching file in path order.
func (r *Retriever) Retrieve(ctx context.Context, query string, scope Scope, limit int) (ContextBundle, error) {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return ContextBundle{}, ErrInvalidQuery
	}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return ContextBundle{}, ErrInvalidRange
	}
	prefix := scope.PathPrefix
	if prefix != "" && (path.IsAbs(prefix) || path.Clean(prefix) != prefix || strings.Contains(prefix, "\\") || prefix == ".." || strings.HasPrefix(prefix, "../")) {
		return ContextBundle{}, ErrInvalidScope
	}
	revision, err := r.repository.Revision(ctx, "")
	if err != nil {
		return ContextBundle{}, err
	}
	paths, err := r.repository.MarkdownPaths(ctx, revision, r.categories)
	if err != nil {
		return ContextBundle{}, err
	}
	result := ContextBundle{Revision: revision, Items: []Item{}}
	for _, name := range paths {
		if err := ctx.Err(); err != nil {
			return ContextBundle{}, err
		}
		if prefix != "" && name != prefix && !strings.HasPrefix(name, prefix+"/") {
			continue
		}
		data, err := r.repository.Read(ctx, revision, name)
		if err != nil {
			return ContextBundle{}, err
		}
		if bytes.ContainsRune(data, 0) {
			continue
		}
		lines := splitLines(string(data))
		first := -1
		for i, line := range lines {
			if matches(terms, line) {
				first = i
				break
			}
		}
		if first < 0 && matches(terms, name) {
			first = 0
		}
		if first < 0 {
			continue
		}
		if len(result.Items) == limit {
			result.Truncated = true
			break
		}
		start := max(0, first-3)
		end := min(len(lines), start+20)
		result.Items = append(result.Items, Item{Path: name, StartLine: start + 1, EndLine: end, Content: strings.Join(lines[start:end], "")})
	}
	return result, nil
}

func matches(terms []string, text string) bool {
	text = strings.ToLower(text)
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

// Read resolves a revision once and reads a line window from that immutable file.
func (r *Retriever) Read(ctx context.Context, name, revision string, startLine, lineCount int) (FileContent, error) {
	allowed := false
	for _, category := range r.categories {
		if strings.HasPrefix(name, "memories/"+category+"/") {
			allowed = true
			break
		}
	}
	if !allowed {
		return FileContent{}, ErrInvalidScope
	}
	if startLine == 0 {
		startLine = 1
	}
	if lineCount == 0 {
		lineCount = 200
	}
	if startLine < 1 || lineCount < 1 || lineCount > 2000 {
		return FileContent{}, ErrInvalidRange
	}
	revision, err := r.repository.Revision(ctx, revision)
	if err != nil {
		return FileContent{}, err
	}
	data, err := r.repository.Read(ctx, revision, name)
	if err != nil {
		return FileContent{}, err
	}
	lines := splitLines(string(data))
	start := min(startLine-1, len(lines))
	end := start + min(lineCount, len(lines)-start)
	return FileContent{Revision: revision, Path: name, StartLine: startLine, EndLine: end, Content: strings.Join(lines[start:end], ""), Truncated: end < len(lines)}, nil
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
