package experiment

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

type sessionSpec struct {
	Kind          string
	Arm           string
	Workspace     string
	MemoryRoot    string
	StateRoot     string
	ConfigRoot    string
	HomeRoot      string
	BridgeRoot    string
	ProductBinary string
	ClaudeBinary  string
	GoRoot        string
}

type sandboxMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type sessionPlan struct {
	Shape           string
	ClaudeMounts    []sandboxMount
	ServerMounts    []sandboxMount
	ClaudeArguments []string
	ServerArguments []string
	MCPCommand      []string
	AllowedTools    []string
	DeniedTools     []string
}

func buildSessionPlan(spec sessionSpec) (sessionPlan, error) {
	if spec.Kind != "capture" && spec.Kind != "curation" && spec.Kind != "task" && spec.Kind != "model_preflight" {
		return sessionPlan{}, errors.New("unknown session kind")
	}
	if spec.Kind == "task" && spec.Arm != "control" && spec.Arm != "treatment" {
		return sessionPlan{}, errors.New("task arm is required")
	}
	for _, path := range []string{spec.Workspace, spec.MemoryRoot, spec.StateRoot, spec.ConfigRoot, spec.HomeRoot, spec.BridgeRoot, spec.ProductBinary, spec.ClaudeBinary} {
		if !filepath.IsAbs(path) {
			return sessionPlan{}, errors.New("sandbox paths must be absolute")
		}
	}
	if spec.GoRoot == "" {
		spec.GoRoot = "/usr/local/go"
	}
	if !filepath.IsAbs(spec.GoRoot) {
		return sessionPlan{}, errors.New("Go root must be absolute")
	}
	plan := sessionPlan{
		MCPCommand: []string{"/usr/bin/socat", "STDIO", "UNIX-CONNECT:/bridge/mcp.sock"},
		ClaudeMounts: []sandboxMount{
			{Source: spec.Workspace, Target: "/workspace", ReadOnly: spec.Kind != "task"},
			{Source: spec.ConfigRoot, Target: "/config", ReadOnly: false},
			{Source: spec.HomeRoot, Target: "/home/experiment", ReadOnly: false},
			{Source: spec.BridgeRoot, Target: "/bridge", ReadOnly: false},
			{Source: spec.ClaudeBinary, Target: "/opt/claude", ReadOnly: true},
			{Source: spec.GoRoot, Target: "/goroot", ReadOnly: true},
		},
		ServerMounts: []sandboxMount{
			{Source: spec.MemoryRoot, Target: "/memory", ReadOnly: spec.Kind == "task"},
			{Source: spec.StateRoot, Target: "/state", ReadOnly: false},
			{Source: spec.ConfigRoot, Target: "/config", ReadOnly: true},
			{Source: spec.ProductBinary, Target: "/opt/gpm", ReadOnly: true},
		},
		ServerArguments: []string{"/opt/gpm", "mcp"},
	}
	if spec.Kind == "capture" || spec.Kind == "curation" {
		plan.ServerMounts = append(plan.ServerMounts, sandboxMount{Source: spec.Workspace, Target: "/workspace", ReadOnly: true})
	}
	switch spec.Kind {
	case "capture":
		plan.AllowedTools = []string{
			"Read", "Grep", "Glob", "mcp__git-push-memories__memory_search", "mcp__git-push-memories__memory_expand",
			"mcp__git-push-memories__memory_reference_snapshot", "mcp__git-push-memories__memory_reference_expand", "mcp__git-push-memories__memory_propose",
		}
	case "curation":
		plan.AllowedTools = []string{
			"Read", "Grep", "Glob", "mcp__git-push-memories__memory_search", "mcp__git-push-memories__memory_expand",
			"mcp__git-push-memories__memory_page_read", "mcp__git-push-memories__memory_reference_snapshot", "mcp__git-push-memories__memory_reference_expand", "mcp__git-push-memories__memory_proposal_list",
			"mcp__git-push-memories__memory_curate_preview", "mcp__git-push-memories__memory_curate_apply",
			"mcp__git-push-memories__memory_curate_batch_preview", "mcp__git-push-memories__memory_curate_batch_apply",
		}
	case "task":
		plan.AllowedTools = []string{
			"Read", "Grep", "Glob", "Edit", "Write", "Bash(gofmt *)", "Bash(go test ./...)",
			"mcp__git-push-memories__memory_search", "mcp__git-push-memories__memory_expand",
		}
		plan.DeniedTools = []string{
			"WebFetch", "WebSearch", "Task", "mcp__git-push-memories__memory_propose", "mcp__git-push-memories__memory_proposal_list",
			"mcp__git-push-memories__memory_curate_preview", "mcp__git-push-memories__memory_curate_apply",
			"mcp__git-push-memories__memory_curate_batch_preview", "mcp__git-push-memories__memory_curate_batch_apply",
			"mcp__git-push-memories__memory_reference_snapshot", "mcp__git-push-memories__memory_reference_expand",
		}
	}
	plan.ClaudeArguments = []string{
		"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--no-chrome", "--restricted",
		"--strict-mcp-config", "--mcp-config", "/config/mcp.json", "--setting-sources", "", "--model", "sonnet", "--effort", "medium", "--autocompact", "100k",
		"--permission-mode", "acceptEdits", "--permission-prompts", "none", "--tools", strings.Join(baseTools(spec.Kind), ","),
		"--allowed-tools", strings.Join(plan.AllowedTools, ","), "--disallowed-tools", strings.Join(plan.DeniedTools, ","),
	}
	plan.Shape = planShape(spec.Kind, plan)
	return plan, nil
}

func baseTools(kind string) []string {
	if kind == "task" {
		return []string{"Read", "Grep", "Glob", "Edit", "Write", "Bash"}
	}
	if kind == "model_preflight" {
		return []string{}
	}
	return []string{"Read", "Grep", "Glob"}
}

func planShape(kind string, plan sessionPlan) string {
	parts := []string{kind}
	for _, mount := range plan.ClaudeMounts {
		parts = append(parts, "claude:"+mount.Target+":"+accessMode(mount.ReadOnly))
	}
	for _, mount := range plan.ServerMounts {
		parts = append(parts, "server:"+mount.Target+":"+accessMode(mount.ReadOnly))
	}
	allowed := append([]string(nil), plan.AllowedTools...)
	denied := append([]string(nil), plan.DeniedTools...)
	sort.Strings(allowed)
	sort.Strings(denied)
	parts = append(parts, allowed...)
	parts = append(parts, denied...)
	return strings.Join(parts, "|")
}

func accessMode(readOnly bool) string {
	if readOnly {
		return "ro"
	}
	return "rw"
}
