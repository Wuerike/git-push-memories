package retrieval

import (
	"context"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/Wuerike/git-push-memories/internal/memory"
)

type Retriever struct {
	repository *memory.Repository
	categories map[string]struct{}
}

func New(memoryRoot string, memoryCategories ...string) (*Retriever, error) {
	if len(memoryCategories) == 0 {
		return nil, ErrInvalidScope
	}
	repository, err := memory.Open(memoryRoot)
	if err != nil {
		return nil, err
	}
	categories := make(map[string]struct{}, len(memoryCategories))
	for _, category := range memoryCategories {
		if category == "" || strings.Contains(category, "/") {
			_ = repository.Close()
			return nil, ErrInvalidScope
		}
		categories[category] = struct{}{}
	}
	return &Retriever{repository: repository, categories: categories}, nil
}

func (retriever *Retriever) Close() error {
	return retriever.repository.Close()
}

type rankedItem struct {
	item         Item
	sectionOrder int
}

func (retriever *Retriever) Retrieve(ctx context.Context, query string, scope Scope, budget int) (ContextBundle, error) {
	terms := tokenize(query)
	if len(terms) == 0 {
		return ContextBundle{}, ErrInvalidQuery
	}
	if budget <= 0 {
		return ContextBundle{}, ErrInvalidBudget
	}
	prefix, err := canonicalScope(scope.PathPrefix)
	if err != nil {
		return ContextBundle{}, err
	}
	paths, err := retriever.markdownPaths(ctx, prefix)
	if err != nil {
		return ContextBundle{}, err
	}

	var ranked []rankedItem
	for _, filePath := range paths {
		snapshot, err := retriever.repository.Inspect(ctx, memory.Scope{Path: filePath})
		if err != nil {
			return ContextBundle{}, err
		}
		for sectionOrder, section := range snapshot.Sections {
			if !section.Leaf {
				continue
			}
			score := matchScore(terms, snapshot.Path+" "+section.Heading+" "+section.Body)
			if score == 0 {
				continue
			}
			ranked = append(ranked, rankedItem{item: Item{
				Handle:  Handle{Path: snapshot.Path, SectionID: section.ID, ContentHash: section.ContentHash},
				Heading: section.Heading,
				Content: strings.TrimSpace(section.Body),
				Score:   score,
			}, sectionOrder: sectionOrder})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		left, right := ranked[i], ranked[j]
		if left.item.Score != right.item.Score {
			return left.item.Score > right.item.Score
		}
		if left.item.Handle.Path != right.item.Handle.Path {
			return left.item.Handle.Path < right.item.Handle.Path
		}
		if left.sectionOrder != right.sectionOrder {
			return left.sectionOrder < right.sectionOrder
		}
		return left.item.Handle.SectionID < right.item.Handle.SectionID
	})
	items := make([]Item, len(ranked))
	for i := range ranked {
		items[i] = ranked[i].item
	}
	return fitContextBundleToBudget(items, budget)
}

func (retriever *Retriever) Expand(ctx context.Context, handle Handle, budget int) (ContextBundle, error) {
	if budget <= 0 {
		return ContextBundle{}, ErrInvalidBudget
	}
	if handle.Path == "" || handle.SectionID == "" || handle.ContentHash == "" || !retriever.authoritativePath(handle.Path) {
		return ContextBundle{}, ErrStaleHandle
	}
	snapshot, err := retriever.repository.Inspect(ctx, memory.Scope{Path: handle.Path})
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return ContextBundle{}, contextErr
		}
		return ContextBundle{}, ErrStaleHandle
	}
	for _, section := range snapshot.Sections {
		if section.ID != handle.SectionID {
			continue
		}
		if section.ContentHash != handle.ContentHash {
			return ContextBundle{}, ErrStaleHandle
		}
		return fitContextBundleToBudget([]Item{{
			Handle:  handle,
			Heading: section.Heading,
			Content: section.Body,
		}}, budget)
	}
	return ContextBundle{}, ErrStaleHandle
}

func tokenize(value string) []string {
	words := strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsNumber(character)
	})
	seen := make(map[string]struct{}, len(words))
	terms := make([]string, 0, len(words))
	for _, word := range words {
		if _, exists := seen[word]; exists {
			continue
		}
		seen[word] = struct{}{}
		terms = append(terms, word)
	}
	return terms
}

func matchScore(queryTerms []string, content string) int {
	contentTerms := make(map[string]struct{})
	for _, term := range tokenize(content) {
		contentTerms[term] = struct{}{}
	}
	score := 0
	for _, term := range queryTerms {
		if _, exists := contentTerms[term]; exists {
			score++
		}
	}
	return score
}

func canonicalScope(requested string) (string, error) {
	if requested == "" {
		return "", nil
	}
	normalized := strings.ReplaceAll(requested, "\\", "/")
	if path.IsAbs(normalized) {
		return "", ErrInvalidScope
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return "", ErrInvalidScope
		}
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrInvalidScope
	}
	return clean, nil
}

func (retriever *Retriever) markdownPaths(ctx context.Context, prefix string) ([]string, error) {
	categories := make([]string, 0, len(retriever.categories))
	for category := range retriever.categories {
		categories = append(categories, category)
	}
	allPaths, err := retriever.repository.MarkdownPaths(ctx, categories)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(allPaths))
	for _, filePath := range allPaths {
		if prefix == "" || filePath == prefix || strings.HasPrefix(filePath, prefix+"/") {
			paths = append(paths, filePath)
		}
	}
	return paths, nil
}

func (retriever *Retriever) authoritativePath(filePath string) bool {
	if !strings.HasPrefix(filePath, "memories/") || path.Ext(filePath) != ".md" {
		return false
	}
	relative := strings.TrimPrefix(filePath, "memories/")
	category, remainder, found := strings.Cut(relative, "/")
	if !found || remainder == "" {
		return false
	}
	_, configured := retriever.categories[category]
	return configured
}

func fitContextBundleToBudget(items []Item, budget int) (ContextBundle, error) {
	for count := len(items); count >= 0; count-- {
		bundle := ContextBundle{Items: items[:count], Truncated: count < len(items)}
		encoded, err := json.Marshal(bundle)
		if err != nil {
			return ContextBundle{}, err
		}
		if len(encoded) <= budget {
			if count == 0 && len(items) > 0 {
				return ContextBundle{}, ErrBudgetTooSmall
			}
			return bundle, nil
		}
	}
	return ContextBundle{}, ErrBudgetTooSmall
}
