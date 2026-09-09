package memory_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/memory"
)

func TestConcurrentAppliesAcrossProcessesYieldOneSuccessAndOneDrift(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}

	commands := []*exec.Cmd{
		applyHelperCommand(root, snapshot.FileHash, "first\n"),
		applyHelperCommand(root, snapshot.FileHash, "second\n"),
	}
	for _, command := range commands {
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	successes, drifts := 0, 0
	for _, command := range commands {
		err := command.Wait()
		if err == nil {
			successes++
			continue
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 20 {
			drifts++
			continue
		}
		t.Fatalf("apply helper failed: %v", err)
	}
	if successes != 1 || drifts != 1 {
		t.Fatalf("outcomes = %d success, %d drift; want one of each", successes, drifts)
	}
}

func TestApplyHelperProcess(t *testing.T) {
	if os.Getenv("GPM_APPLY_HELPER") != "1" {
		return
	}
	repository, err := memory.New(os.Getenv("GPM_MEMORY_ROOT"))
	if repository != nil {
		defer repository.Close()
	}
	if err == nil {
		_, err = repository.Apply(context.Background(), memory.ChangeSet{
			Target: memory.Target{
				Path:      "memories/checkout-api.md",
				SectionID: "checkout-api-operation",
			},
			BaselineHash: os.Getenv("GPM_BASELINE_HASH"),
			Replacement:  os.Getenv("GPM_REPLACEMENT"),
		})
	}
	if errors.Is(err, memory.ErrDrift) {
		os.Exit(20)
	}
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func applyHelperCommand(root, baseline, replacement string) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^TestApplyHelperProcess$")
	command.Env = append(os.Environ(),
		"GPM_APPLY_HELPER=1",
		"GPM_MEMORY_ROOT="+root,
		"GPM_BASELINE_HASH="+baseline,
		"GPM_REPLACEMENT="+replacement,
	)
	return command
}
