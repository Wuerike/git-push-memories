package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyLeavesBaselineWhenAtomicWriterFails(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "page.md")
	source := []byte("# Page\n<!-- @id: page -->\n## Leaf\n<!-- @id: leaf -->\nold\n")
	if err := os.WriteFile(filePath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []writeStage{writeCreated, writeWritten, writeSynced} {
		t.Run(stage.String(), func(t *testing.T) {
			if err := os.WriteFile(filePath, source, 0o644); err != nil {
				t.Fatal(err)
			}
			writeFailure := errors.New("injected write failure")
			repository, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			defer repository.Close()
			repository.fault = func(current writeStage) error {
				if current == stage {
					return writeFailure
				}
				return nil
			}

			_, err = repository.Apply(context.Background(), ChangeSet{
				Target:       Target{Path: "page.md", SectionID: "leaf"},
				BaselineHash: hash(source),
				Replacement:  "new\n",
			})
			if !errors.Is(err, writeFailure) {
				t.Fatalf("Apply() error = %v, want injected failure", err)
			}
			got, readErr := os.ReadFile(filePath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != string(source) {
				t.Fatal("Apply() changed the baseline before rename")
			}
			entries, readErr := os.ReadDir(root)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 1 || entries[0].Name() != "page.md" {
				t.Fatalf("temporary remains after failure: %v", entries)
			}
		})
	}
}

func TestNewRemovesControlledInterruptedTemporaries(t *testing.T) {
	root := t.TempDir()
	temporary := filepath.Join(root, ".git-push-memories-0123456789abcdef.tmp")
	if err := os.WriteFile(temporary, []byte("incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if _, err := os.Stat(temporary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("controlled temporary still exists: %v", err)
	}
}

func TestApplyTreatsRenameAsCommitPointWhenLaterStepFails(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "page.md")
	source := []byte("# Page\n<!-- @id: page -->\n## Leaf\n<!-- @id: leaf -->\nold\n")
	if err := os.WriteFile(filePath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	writeFailure := errors.New("injected post-rename failure")
	repository, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	repository.fault = func(stage writeStage) error {
		if stage == writeRenamed {
			return writeFailure
		}
		return nil
	}
	_, err = repository.Apply(context.Background(), ChangeSet{
		Target:       Target{Path: "page.md", SectionID: "leaf"},
		BaselineHash: hash(source),
		Replacement:  "new\n",
	})
	if !errors.Is(err, writeFailure) {
		t.Fatalf("Apply() error = %v, want injected post-rename failure", err)
	}
	got, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	want := "# Page\n<!-- @id: page -->\n## Leaf\n<!-- @id: leaf -->\nnew\n"
	if string(got) != want {
		t.Fatalf("file after rename = %q, want committed post-state", got)
	}
}
