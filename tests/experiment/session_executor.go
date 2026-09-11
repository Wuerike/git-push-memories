package experiment

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type reviewRequest struct {
	Kind   string
	UnitID string
	Turn   int
	Text   string
}

type reviewFunc func(reviewRequest) (string, error)

type sessionRequest struct {
	UnitID        string
	Kind          string
	Arm           string
	Workspace     string
	MemoryRoot    string
	StateRoot     string
	Prompt        string
	ExpectedModel string
	RelevantTerms []string
	VisiblePaths  []string
	MaxBudgetUSD  float64
	Timeout       time.Duration
}

type executor struct {
	root          string
	authRoot      string
	productBinary string
	claudeBinary  string
	credential    string
	rememberSkill string
	curateSkill   string
	goRoot        string
}

func (runner executor) run(ctx context.Context, request sessionRequest, reviewer reviewFunc) (SessionObservation, time.Duration, error) {
	sessionContext, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	sessionRoot := filepath.Join(runner.root, "units", request.UnitID)
	configRoot := filepath.Join(runner.authRoot, request.UnitID)
	homeRoot := filepath.Join(sessionRoot, "home")
	bridgeRoot := filepath.Join(sessionRoot, "bridge")
	for _, directory := range []string{configRoot, homeRoot, bridgeRoot, request.StateRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return SessionObservation{}, 0, err
		}
	}
	defer os.RemoveAll(bridgeRoot)
	credential, err := os.ReadFile(runner.credential)
	if err != nil {
		return SessionObservation{}, 0, errors.New("read Claude credential")
	}
	if err := os.WriteFile(filepath.Join(configRoot, ".credentials.json"), credential, 0o600); err != nil {
		return SessionObservation{}, 0, err
	}
	for index := range credential {
		credential[index] = 0
	}
	if request.Kind == "capture" {
		if err := copySkill(runner.rememberSkill, filepath.Join(configRoot, "skills", "remember", "SKILL.md")); err != nil {
			return SessionObservation{}, 0, err
		}
	}
	if request.Kind == "curation" {
		if err := copySkill(runner.curateSkill, filepath.Join(configRoot, "skills", "curate", "SKILL.md")); err != nil {
			return SessionObservation{}, 0, err
		}
	}
	if err := writeRuntimeJSON(filepath.Join(configRoot, "git-push-memories", "config.json"), map[string]any{"schema_version": 1, "memory_root": "/memory"}); err != nil {
		return SessionObservation{}, 0, err
	}
	servers := map[string]any{}
	if request.Kind != "model_preflight" {
		servers["git-push-memories"] = map[string]any{"type": "stdio", "command": "/usr/bin/socat", "args": []string{"STDIO", "UNIX-CONNECT:/bridge/mcp.sock"}}
	}
	if err := writeRuntimeJSON(filepath.Join(configRoot, "mcp.json"), map[string]any{"mcpServers": servers}); err != nil {
		return SessionObservation{}, 0, err
	}
	plan, err := buildSessionPlan(sessionSpec{
		Kind: request.Kind, Arm: request.Arm, Workspace: request.Workspace, MemoryRoot: request.MemoryRoot, StateRoot: request.StateRoot,
		ConfigRoot: configRoot, HomeRoot: homeRoot, BridgeRoot: bridgeRoot, ProductBinary: runner.productBinary, ClaudeBinary: runner.claudeBinary,
		GoRoot: runner.goRoot,
	})
	if err != nil {
		return SessionObservation{}, 0, err
	}
	plan.ClaudeArguments = append(plan.ClaudeArguments, "--max-budget-usd", fmt.Sprintf("%.2f", request.MaxBudgetUSD))
	if err := runSandboxProbe(sessionContext, plan, request.VisiblePaths); err != nil {
		return SessionObservation{}, 0, fmt.Errorf("sandbox probe: %w", err)
	}
	if request.Kind != "model_preflight" {
		listener, err := startMCPBridge(sessionContext, plan, bridgeRoot)
		if err != nil {
			return SessionObservation{}, 0, err
		}
		defer os.Remove(filepath.Join(filepath.Dir(bridgeRoot), "server.sh"))
		defer func() {
			if listener.Process != nil {
				_ = listener.Process.Kill()
				_, _ = listener.Process.Wait()
			}
		}()
	}

	arguments := claudeBwrapArguments(plan)
	command := exec.CommandContext(sessionContext, "bwrap", arguments...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return SessionObservation{}, 0, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return SessionObservation{}, 0, err
	}
	command.Stderr = io.Discard
	command.WaitDelay = 5 * time.Second
	started := time.Now()
	if err := command.Start(); err != nil {
		return SessionObservation{}, 0, err
	}
	encoder := json.NewEncoder(stdin)
	if err := sendUserTurn(encoder, request.Prompt); err != nil {
		_ = stdin.Close()
		_ = command.Wait()
		return SessionObservation{}, 0, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	combined := SessionObservation{ToolCounts: make(map[string]int), ToolArguments: make(map[string][]map[string]any)}
	allowed := make(map[string]bool, len(plan.AllowedTools))
	for _, tool := range plan.AllowedTools {
		allowed[tool] = true
	}
	completed := false
	lastReviewerResponse := ""
	for turn := 1; turn <= 4 && !completed; turn++ {
		turnObserver := newStreamObserver(streamPolicy{ExpectedModel: request.ExpectedModel, RelevantTerms: request.RelevantTerms, AllowedTools: allowed})
		var assistantText strings.Builder
		terminalSeen := false
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			if err := turnObserver.consume(line); err != nil {
				zero(line)
				_ = stdin.Close()
				_ = command.Wait()
				return combined, time.Since(started), err
			}
			collectAssistantText(line, &assistantText)
			terminalSeen = eventType(line) == "result"
			zero(line)
			if terminalSeen {
				break
			}
		}
		if !terminalSeen {
			_ = stdin.Close()
			_ = command.Wait()
			return combined, time.Since(started), errors.New("Claude stream ended without a terminal result")
		}
		turnResult, err := turnObserver.finish()
		if err != nil {
			_ = stdin.Close()
			_ = command.Wait()
			return combined, time.Since(started), err
		}
		mergeObservation(&combined, turnResult)
		if turnResult.IsError {
			_ = stdin.Close()
			_ = command.Wait()
			return combined, time.Since(started), errors.New("Claude returned an error result")
		}
		targetCalled := targetToolCalled(request.Kind, combined.ToolCounts)
		if targetCalled && request.Kind == "capture" && lastReviewerResponse != "Confirm those exact fields." {
			_ = stdin.Close()
			_ = command.Wait()
			return combined, time.Since(started), errors.New("capture tool called without the exact human confirmation")
		}
		if targetCalled && request.Kind == "curation" && lastReviewerResponse != "Confirm this exact decision and diff." {
			_ = stdin.Close()
			_ = command.Wait()
			return combined, time.Since(started), errors.New("curation apply called without the exact human confirmation")
		}
		completed = request.Kind == "task" || request.Kind == "model_preflight" || targetCalled
		if completed {
			break
		}
		if reviewer == nil {
			_ = stdin.Close()
			_ = command.Wait()
			return combined, time.Since(started), errors.New("interactive review is required")
		}
		response, err := reviewer(reviewRequest{Kind: request.Kind, UnitID: request.UnitID, Turn: turn, Text: assistantText.String()})
		if err != nil {
			_ = stdin.Close()
			_ = command.Wait()
			return combined, time.Since(started), err
		}
		if response == "/cancel" {
			combined.Cancelled = true
			completed = true
			break
		}
		lastReviewerResponse = response
		if response == "" || sendUserTurn(encoder, response) != nil {
			_ = stdin.Close()
			_ = command.Wait()
			return combined, time.Since(started), errors.New("invalid reviewer response")
		}
	}
	_ = stdin.Close()
	waitErr := command.Wait()
	duration := time.Since(started)
	if scanErr := scanner.Err(); scanErr != nil {
		return combined, duration, scanErr
	}
	if waitErr != nil && !combined.Cancelled {
		return combined, duration, waitErr
	}
	if !completed {
		return combined, duration, errors.New("interaction exceeded four turns")
	}
	return combined, duration, nil
}

func startMCPBridge(ctx context.Context, plan sessionPlan, bridgeRoot string) (*exec.Cmd, error) {
	wrapper := filepath.Join(filepath.Dir(bridgeRoot), "server.sh")
	arguments := serverBwrapArguments(plan)
	var script strings.Builder
	script.WriteString("#!/bin/sh\nexec")
	for _, argument := range append([]string{"bwrap"}, arguments...) {
		script.WriteByte(' ')
		script.WriteString(shellQuote(argument))
	}
	script.WriteByte('\n')
	if err := os.WriteFile(wrapper, []byte(script.String()), 0o700); err != nil {
		return nil, err
	}
	socket := filepath.Join(bridgeRoot, "mcp.sock")
	command := exec.CommandContext(ctx, "socat", "UNIX-LISTEN:"+socket+",mode=0600", "EXEC:"+wrapper)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, err
	}
	for range 100 {
		if _, err := os.Stat(socket); err == nil {
			return command, nil
		}
		if command.ProcessState != nil && command.ProcessState.Exited() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = command.Process.Kill()
	_ = command.Wait()
	return nil, errors.New("MCP bridge socket did not become ready")
}

func runSandboxProbe(ctx context.Context, plan sessionPlan, visiblePaths []string) error {
	arguments := claudeBwrapBase(plan)
	probe := "test ! -e /memory && test ! -e /state && test ! -e /protocol && test ! -e /workspace/testdata && test ! -e /workspace/policy/policy_hidden_test.go"
	if len(visiblePaths) > 0 {
		expected := append([]string(nil), visiblePaths...)
		sort.Strings(expected)
		probe += " && test \"$(git -C /workspace ls-files | LC_ALL=C sort)\" = " + shellQuote(strings.Join(expected, "\n"))
	}
	arguments = append(arguments, "--", "/bin/sh", "-c", probe)
	command := exec.CommandContext(ctx, "bwrap", arguments...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}

func claudeBwrapArguments(plan sessionPlan) []string {
	arguments := claudeBwrapBase(plan)
	return append(arguments, append([]string{"--", "/opt/claude"}, plan.ClaudeArguments...)...)
}

func claudeBwrapBase(plan sessionPlan) []string {
	arguments := sandboxBaseArguments()
	arguments = append(arguments, "--dir", "/home", "--dir", "/home/experiment", "--dir", "/opt", "--dir", "/config", "--dir", "/bridge", "--dir", "/workspace")
	arguments = appendMounts(arguments, plan.ClaudeMounts)
	return append(arguments,
		"--chdir", "/workspace", "--setenv", "PATH", "/goroot/bin:/usr/local/bin:/usr/bin:/bin", "--setenv", "GOROOT", "/goroot", "--setenv", "HOME", "/home/experiment",
		"--setenv", "CLAUDE_CONFIG_DIR", "/config", "--setenv", "XDG_CONFIG_HOME", "/config", "--setenv", "TMPDIR", "/tmp",
		"--setenv", "GOTOOLCHAIN", "go1.25.12", "--setenv", "GOPROXY", "off", "--setenv", "GIT_CONFIG_NOSYSTEM", "1",
		"--setenv", "GIT_CONFIG_GLOBAL", "/dev/null", "--setenv", "TZ", "UTC", "--setenv", "LC_ALL", "C.UTF-8",
	)
}

func serverBwrapArguments(plan sessionPlan) []string {
	arguments := sandboxBaseArguments()
	arguments = append(arguments, "--dir", "/opt", "--dir", "/config", "--dir", "/memory", "--dir", "/state", "--dir", "/workspace")
	arguments = appendMounts(arguments, plan.ServerMounts)
	arguments = append(arguments,
		"--chdir", "/workspace", "--setenv", "PATH", "/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin", "--setenv", "HOME", "/nonexistent",
		"--setenv", "XDG_CONFIG_HOME", "/config", "--setenv", "XDG_STATE_HOME", "/state", "--setenv", "GOTOOLCHAIN", "go1.25.12",
		"--setenv", "GOPROXY", "off", "--setenv", "GIT_CONFIG_NOSYSTEM", "1", "--setenv", "GIT_CONFIG_GLOBAL", "/dev/null",
		"--setenv", "TZ", "UTC", "--setenv", "LC_ALL", "C.UTF-8", "--",
	)
	return append(arguments, plan.ServerArguments...)
}

func sandboxBaseArguments() []string {
	arguments := []string{"--clearenv", "--die-with-parent", "--new-session", "--unshare-pid", "--unshare-uts", "--unshare-ipc"}
	for _, path := range []string{"/usr", "/bin", "/lib", "/lib64", "/etc/ssl", "/etc/resolv.conf", "/etc/hosts"} {
		if _, err := os.Stat(path); err == nil {
			arguments = append(arguments, "--ro-bind", path, path)
		}
	}
	return append(arguments, "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp")
}

func appendMounts(arguments []string, mounts []sandboxMount) []string {
	for _, mount := range mounts {
		option := "--bind"
		if mount.ReadOnly {
			option = "--ro-bind"
		}
		arguments = append(arguments, option, mount.Source, mount.Target)
	}
	return arguments
}

func sendUserTurn(encoder *json.Encoder, text string) error {
	return encoder.Encode(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}})
}

func collectAssistantText(line []byte, target *strings.Builder) {
	var event struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &event) != nil || event.Type != "assistant" {
		return
	}
	for _, content := range event.Message.Content {
		if content.Type == "text" {
			target.WriteString(content.Text)
		}
	}
}

func eventType(line []byte) string {
	var event struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(line, &event)
	return event.Type
}

func mergeObservation(target *SessionObservation, source SessionObservation) {
	target.Authenticated = target.Authenticated || source.Authenticated
	if target.Model == "" {
		target.Model = source.Model
	}
	target.Turns = source.Turns
	target.CostAvailable = source.CostAvailable
	target.CostMicroUSD = source.CostMicroUSD
	target.IsError = target.IsError || source.IsError
	for name, count := range source.ToolCounts {
		target.ToolCounts[name] += count
	}
	for name, arguments := range source.ToolArguments {
		target.ToolArguments[name] = append(target.ToolArguments[name], arguments...)
	}
	target.ExplorationCalls += source.ExplorationCalls
	target.ExplorationResultBytes += source.ExplorationResultBytes
	target.MemoryResultBytes += source.MemoryResultBytes
	target.CurationToolBytes += source.CurationToolBytes
	target.MemoryRelevant = target.MemoryRelevant || source.MemoryRelevant
	target.ErrorCodes = appendUniqueValues(target.ErrorCodes, source.ErrorCodes...)
	target.ReferenceIntegrities = appendUniqueValues(target.ReferenceIntegrities, source.ReferenceIntegrities...)
	if source.ProposalArguments != nil {
		target.ProposalArguments = source.ProposalArguments
	}
}

func appendUniqueValues(target []string, values ...string) []string {
	for _, value := range values {
		target = appendUnique(target, value)
	}
	return target
}

func targetToolCalled(kind string, counts map[string]int) bool {
	switch kind {
	case "capture":
		return counts["mcp__git-push-memories__memory_propose"] > 0
	case "curation":
		return counts["mcp__git-push-memories__memory_curate_apply"] > 0 || counts["mcp__git-push-memories__memory_curate_batch_apply"] > 0
	default:
		return true
	}
}

func copySkill(source, target string) error {
	contents, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	return os.WriteFile(target, contents, 0o600)
}

func writeRuntimeJSON(path string, value any) error {
	contents, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(contents, '\n'), 0o600)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func zero(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
