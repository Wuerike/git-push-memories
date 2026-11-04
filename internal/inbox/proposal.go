package inbox

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
)

type Snapshot struct{ Proposal Proposal }

// List returns regular Markdown files with their immutable path/blob identities.
func (inbox *Inbox) List(ctx context.Context) ([]Snapshot, error) {
	if err := inbox.checkBranch(ctx); err != nil {
		return nil, err
	}
	git := inbox.git()
	head, err := git.Output(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("read inbox HEAD: %w", err)
	}
	entries, err := git.Bytes(ctx, "ls-tree", "-r", "-z", "--full-tree", head, "--", "proposals")
	if err != nil {
		return nil, err
	}
	snapshots := []Snapshot{}
	for _, encoded := range bytes.Split(entries, []byte{0}) {
		meta, name, ok := strings.Cut(string(encoded), "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || !(fields[0] == "100644" || fields[0] == "100755") || fields[1] != "blob" || path.Ext(name) != ".md" {
			continue
		}
		contents, err := git.Bytes(ctx, "cat-file", "blob", fields[2])
		if err != nil {
			return nil, fmt.Errorf("read proposal %s: %w", name, err)
		}
		snapshots = append(snapshots, Snapshot{Proposal: Proposal{Path: name, Blob: fields[2], Content: string(contents)}})
	}
	return snapshots, nil
}
