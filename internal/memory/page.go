package memory

import "context"

// ReadPage returns the whole canonical page under a shared lock, without writing.
func (repository *Repository) ReadPage(ctx context.Context, requestedPath string) (Page, error) {
	var page Page
	err := repository.withLock(ctx, func() error {
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
