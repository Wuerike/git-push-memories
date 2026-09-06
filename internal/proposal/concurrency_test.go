package proposal_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/proposal"
)

func TestApplyAndRejectAcrossProcessesPublishOneTerminalReceipt(t *testing.T) {
	for iteration := range 8 {
		t.Run(fmt.Sprintf("iteration-%d", iteration), testApplyAndRejectAcrossProcesses)
	}
}

func testApplyAndRejectAcrossProcesses(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	codeRoot := newCodeRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, codeRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Close(); err != nil {
		t.Fatal(err)
	}
	commands := []*exec.Cmd{
		proposalHelperCommand("apply", memoryRoot, codeRoot, stateRoot, view.ID),
		proposalHelperCommand("reject", memoryRoot, codeRoot, stateRoot, view.ID),
	}
	outputs := make([]strings.Builder, len(commands))
	for index, command := range commands {
		command.Stderr = &outputs[index]
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	successes, terminals := 0, 0
	for index, command := range commands {
		err := command.Wait()
		if err == nil {
			successes++
			continue
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 20 {
			terminals++
			continue
		}
		t.Fatalf("proposal helper failed: %v: %s", err, outputs[index].String())
	}
	if successes != 1 || terminals != 1 {
		t.Fatalf("outcomes = %d success, %d terminal; want one each", successes, terminals)
	}
	_, applyErr := os.Stat(filepath.Join(stateRoot, "applies", view.ID+".json"))
	_, rejectErr := os.Stat(filepath.Join(stateRoot, "decisions", view.ID+".json"))
	if (applyErr == nil) == (rejectErr == nil) {
		t.Fatalf("terminal receipts: apply=%v reject=%v", applyErr, rejectErr)
	}
	contents, err := os.ReadFile(filepath.Join(memoryRoot, "services", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if applyErr == nil && !strings.Contains(string(contents), "\ncurrent\n") {
		t.Fatal("apply receipt exists without post-state")
	}
	if rejectErr == nil && string(contents) != initialPage {
		t.Fatal("reject receipt exists without baseline")
	}
}

func TestProposalDecisionHelperProcess(t *testing.T) {
	if os.Getenv("GPM_PROPOSAL_HELPER") != "1" {
		return
	}
	workflow, err := proposal.New(os.Getenv("GPM_MEMORY_ROOT"), os.Getenv("GPM_CODE_ROOT"), os.Getenv("GPM_STATE_ROOT"))
	if workflow != nil {
		defer workflow.Close()
	}
	if err == nil {
		switch os.Getenv("GPM_ACTION") {
		case "apply":
			_, err = workflow.Apply(context.Background(), os.Getenv("GPM_PROPOSAL_ID"), 1)
		case "reject":
			_, err = workflow.Reject(os.Getenv("GPM_PROPOSAL_ID"), "incorrect")
		default:
			err = errors.New("invalid helper action")
		}
	}
	if errors.Is(err, proposal.ErrTerminal) {
		os.Exit(20)
	}
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func proposalHelperCommand(action, memoryRoot, codeRoot, stateRoot, id string) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^TestProposalDecisionHelperProcess$")
	command.Env = append(os.Environ(),
		"GPM_PROPOSAL_HELPER=1",
		"GPM_ACTION="+action,
		"GPM_MEMORY_ROOT="+memoryRoot,
		"GPM_CODE_ROOT="+codeRoot,
		"GPM_STATE_ROOT="+stateRoot,
		"GPM_PROPOSAL_ID="+id,
	)
	return command
}
