package mcpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const memoryPage = `# checkout-api
<!-- @id: checkout-api -->

## Operacao
<!-- @id: checkout-api-operation -->

- timeout: tres segundos
- owner: team-b
`

const removableMemoryPage = `# Checkout API
<!-- @id: checkout-api -->

## Operation
<!-- @id: checkout-api-operation -->
Timeout is three seconds.

## Ownership
<!-- @id: checkout-api-ownership -->
Owned by team-b.
`

const checkoutPath = "memories/domains/checkout-api.md"

type response struct {
	SchemaVersion int                      `json:"schema_version"`
	Data          *retrieval.ContextBundle `json:"data,omitempty"`
	Error         *struct {
		Code string `json:"code"`
	} `json:"error,omitempty"`
}

type inboxResponse struct {
	SchemaVersion int             `json:"schema_version"`
	Data          *inbox.Proposal `json:"data,omitempty"`
	Error         *struct {
		Code string `json:"code"`
	} `json:"error,omitempty"`
}

type pageReadResponse struct {
	Data *struct {
		Path        string `json:"path"`
		ContentHash string `json:"content_hash"`
		Markdown    string `json:"markdown"`
	} `json:"data"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func decodeResponse(t *testing.T, result *mcp.CallToolResult) response {
	t.Helper()
	var decoded response
	decodeStructured(t, result, &decoded)
	return decoded
}

func decodeInboxResponse(t *testing.T, result *mcp.CallToolResult) inboxResponse {
	t.Helper()
	var decoded inboxResponse
	decodeStructured(t, result, &decoded)
	return decoded
}

func decodeStructured(t *testing.T, result *mcp.CallToolResult, target any) {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatal(err)
	}
}

// errorCode returns the error code of a tool result, or "" when it carries none.
func errorCode(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var decoded struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeStructured(t, result, &decoded)
	if decoded.Error == nil {
		return ""
	}
	return decoded.Error.Code
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	return workspacetest.Git(t, root, arguments...)
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	workspacetest.Git(t, root, arguments...)
}

// startServer runs an MCP server for the managed repository over in-memory pipes.
func startServer(t *testing.T, m *workspacetest.Managed) (*mcp.ClientSession, func()) {
	t.Helper()
	server, err := mcpserver.New(m.Config, "test")
	if err != nil {
		t.Fatal(err)
	}
	serverInput, clientOutput := io.Pipe()
	clientInput, serverOutput := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- server.Run(context.Background(), serverInput, serverOutput) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.IOTransport{Reader: clientInput, Writer: clientOutput}, nil)
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	return session, func() {
		if stopped {
			return
		}
		stopped = true
		if err := session.Close(); err != nil {
			t.Error(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("MCP server did not stop")
		}
	}
}

// env is a set-up memory repository with a running MCP server.
type env struct {
	m       *workspacetest.Managed
	session *mcp.ClientSession
	stop    func()
}

// quietRefresh records a fresh fetch attempt so read tools never refresh the read worktree in the background.
func quietRefresh(t *testing.T, m *workspacetest.Managed) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(m.Config.Paths.State, "fetch-attempt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// newEnv seeds the remote main with files, runs setup and starts a server.
func newEnv(t *testing.T, files map[string]string) *env {
	t.Helper()
	m := workspacetest.Setup(t, files)
	quietRefresh(t, m)
	e := &env{m: m}
	e.session, e.stop = startServer(t, m)
	t.Cleanup(e.stop)
	return e
}

func (e *env) proposalsRoot() string { return e.m.Config.Paths.Proposals }

func (e *env) readRoot() string { return e.m.Config.Paths.Read }

// openRelease opens a release over the proposals sent so far and returns the release worktree root.
func (e *env) openRelease(t *testing.T) string {
	t.Helper()
	return e.m.OpenRelease(t).ReleaseRoot
}

func (e *env) call(t *testing.T, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := e.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (e *env) ok(t *testing.T, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result := e.call(t, name, arguments)
	if result.IsError {
		t.Fatalf("%s = %+v, code %q", name, result, errorCode(t, result))
	}
	return result
}

func (e *env) propose(t *testing.T, arguments map[string]any) inbox.Proposal {
	t.Helper()
	proposal := decodeInboxResponse(t, e.ok(t, "memory_propose", arguments)).Data
	if proposal == nil {
		t.Fatal("memory_propose returned no proposal")
	}
	return *proposal
}

func (e *env) search(t *testing.T, source, query string) *retrieval.ContextBundle {
	t.Helper()
	arguments := map[string]any{"query": query, "budget_bytes": 2048}
	if source != "" {
		arguments["source"] = source
	}
	result := e.ok(t, "memory_search", arguments)
	return decodeResponse(t, result).Data
}

// previewToken previews one decision and returns its token.
func (e *env) previewToken(t *testing.T, arguments map[string]any) string {
	t.Helper()
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
	}
	decodeStructured(t, e.ok(t, "memory_promote_preview", arguments), &preview)
	if preview.Data.PreviewToken == "" {
		t.Fatal("memory_promote_preview returned no preview_token")
	}
	return preview.Data.PreviewToken
}

func (e *env) batchToken(t *testing.T, items ...map[string]any) string {
	t.Helper()
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
	}
	decodeStructured(t, e.ok(t, "memory_promote_batch_preview", map[string]any{"items": items}), &preview)
	if preview.Data.PreviewToken == "" {
		t.Fatal("memory_promote_batch_preview returned no preview_token")
	}
	return preview.Data.PreviewToken
}

func (e *env) pageRead(t *testing.T, source, path string, budget int) (*mcp.CallToolResult, pageReadResponse) {
	t.Helper()
	arguments := map[string]any{"path": path, "budget_bytes": budget}
	if source != "" {
		arguments["source"] = source
	}
	result := e.call(t, "memory_page_read", arguments)
	var decoded pageReadResponse
	decodeStructured(t, result, &decoded)
	return result, decoded
}

func readFile(t *testing.T, root, relative string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func exists(root, relative string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative)))
	return err == nil
}

func writeFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(relative)), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// publishExecutable commits the executable bit of path on the remote main.
func publishExecutable(t *testing.T, m *workspacetest.Managed, path string) {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	workspacetest.Git(t, filepath.Dir(clone), "clone", "-q", m.Remote, clone)
	if err := os.Chmod(filepath.Join(clone, filepath.FromSlash(path)), 0o755); err != nil {
		t.Fatal(err)
	}
	workspacetest.Git(t, clone, "add", path)
	workspacetest.Git(t, clone, "commit", "-q", "-m", "make memory executable")
	workspacetest.Git(t, clone, "push", "-q", "origin", "main")
}

func commitHeads(t *testing.T, root string) string {
	t.Helper()
	return strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
}

func goEnvironment() []string {
	output, err := exec.Command("go", "env", "GOCACHE", "GOMODCACHE").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 {
		return nil
	}
	return []string{"GOCACHE=" + lines[0], "GOMODCACHE=" + lines[1]}
}

// builderEnvironment is captured before any test redirects HOME, so builds keep the real caches.
var builderEnvironment = goEnvironment()

func buildBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "gpm")
	command := exec.Command("go", "build", "-o", binary, "./cmd/gpm")
	command.Dir = repositoryRoot
	command.Env = append(os.Environ(), builderEnvironment...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build harness binary: %v: %s", err, output)
	}
	return binary
}
