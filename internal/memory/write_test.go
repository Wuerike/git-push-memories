package memory

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

func TestApplyDoesNotOverwriteEditMadeImmediatelyBeforeRename(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "page.md")
	source := []byte("# Page\n<!-- @id: page -->\n## Leaf\n<!-- @id: leaf -->\nold\n")
	if err := os.WriteFile(filePath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	repository, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	concurrent := []byte("# Page\n<!-- @id: page -->\n## Leaf\n<!-- @id: leaf -->\nconcurrent\n")
	repository.fault = func(stage writeStage) error {
		if stage == writeSynced {
			return os.WriteFile(filePath, concurrent, 0o644)
		}
		return nil
	}
	_, err = repository.Apply(context.Background(), ChangeSet{Target: Target{Path: "page.md", SectionID: "leaf"}, BaselineHash: hash(source), Replacement: "new\n"})
	if !errors.Is(err, ErrDrift) {
		t.Fatalf("Apply() error = %v, want ErrDrift", err)
	}
	got, err := os.ReadFile(filePath)
	if err != nil || string(got) != string(concurrent) {
		t.Fatalf("concurrent edit = %q, %v", got, err)
	}
}

func TestCreatePageFaultsPreserveAbsenceOrRecoverPostState(t *testing.T) {
	for _, stage := range []writeStage{writeCreated, writeWritten, writeSynced, writeRenamed} {
		t.Run(stage.String(), func(t *testing.T) {
			root := t.TempDir()
			for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}, {"commit", "--quiet", "--allow-empty", "-m", "initial"}} {
				command := exec.Command("git", append([]string{"-C", root}, args...)...)
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v: %s", args, err, output)
				}
			}
			repository, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			defer repository.Close()
			preview, err := repository.PreviewCreatePage(context.Background(), CreatePageRequest{TargetPath: "page.md", Title: "Page", Body: "Body."})
			if err != nil {
				t.Fatal(err)
			}
			writeFailure := errors.New("injected create failure")
			repository.fault = func(current writeStage) error {
				if current == stage {
					return writeFailure
				}
				return nil
			}
			request := ReconcileCreatePageRequest{
				CreatePageRequest: CreatePageRequest{TargetPath: "page.md", Title: "Page", Body: "Body."},
				PostStateHash:     preview.PostStateHash, Head: preview.Head, Mode: ApplyBaseline,
			}
			if _, err := repository.ReconcileCreatePage(context.Background(), request); !errors.Is(err, writeFailure) {
				t.Fatalf("ReconcileCreatePage() error = %v, want injected failure", err)
			}
			_, statErr := os.Stat(filepath.Join(root, "page.md"))
			if stage < writeRenamed && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("target exists before rename: %v", statErr)
			}
			if stage == writeRenamed && statErr != nil {
				t.Fatalf("target missing after rename: %v", statErr)
			}
			repository.fault = nil
			result, err := repository.ReconcileCreatePage(context.Background(), request)
			if err != nil || result.State != ReconcilePostState {
				t.Fatalf("retry = %+v, %v", result, err)
			}
		})
	}
}
