package mcpserver_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// harness is a set-up repository plus the real gpm binary, driven over stdio like Claude does.
type harness struct {
	m      *workspacetest.Managed
	binary string
}

func newHarness(t *testing.T, files map[string]string) *harness {
	t.Helper()
	binary := buildBinary(t) // before setup redirects HOME, so the build keeps its caches
	m := workspacetest.Setup(t, files)
	quietRefresh(t, m)
	return &harness{m: m, binary: binary}
}

func (h *harness) connect(t *testing.T, name string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: exec.Command(h.binary, "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func e2eCall(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil || result.IsError {
		t.Fatalf("%s = %+v, %v", name, result, err)
	}
	return result
}

func TestClaudeAdapterCommitsProposalToMemoryRepository(t *testing.T) {
	h := newHarness(t, map[string]string{checkoutPath: memoryPage})
	session := h.connect(t, "claude-harness")
	result := e2eCall(t, session, "memory_propose", map[string]any{
		"title":   "Update checkout timeout",
		"content": "The checkout timeout is five seconds.",
	})
	response := decodeInboxResponse(t, result)
	if response.Data == nil {
		t.Fatalf("proposal result = %+v, response = %+v", result, response)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	proposals := h.m.Config.Paths.Proposals
	if remote := gitOutput(t, h.m.Remote, "show", response.Data.Commit+":"+response.Data.Path); remote != "The checkout timeout is five seconds." {
		t.Fatalf("remote proposal Markdown = %q", remote)
	}
	if gitOutput(t, h.m.Remote, "rev-parse", "refs/heads/proposals") != response.Data.Commit {
		t.Fatal("remote proposals does not point at the proposal commit")
	}
	if contents := readFile(t, proposals, checkoutPath); contents != memoryPage {
		t.Fatal("proposal changed canonical memory")
	}
	if status := gitOutput(t, proposals, "status", "--porcelain=v1"); status != "" {
		t.Fatalf("proposal left Git dirty: %s", status)
	}
	if main := gitOutput(t, h.m.Remote, "ls-tree", "-r", "--name-only", "main"); strings.Contains(main, "proposals/") {
		t.Fatalf("proposal reached main: %q", main)
	}
}

func TestMissingClaudeMCPIsVisibleWithoutPersistingPayload(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	client := mcp.NewClient(&mcp.Implementation{Name: "claude-harness", Version: "test"}, nil)
	command := exec.Command(filepath.Join(t.TempDir(), "missing-gpm"), "mcp")
	if _, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil); err == nil {
		t.Fatal("Connect() succeeded without the MCP executable")
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state root exists after missing MCP: %v", err)
	}
}
