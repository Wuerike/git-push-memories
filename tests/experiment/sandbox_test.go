package experiment

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildSessionPlanSeparatesClaudeFromMemoryAndState(t *testing.T) {
	plan, err := buildSessionPlan(sessionSpec{
		Kind: "capture", Workspace: "/private/source", MemoryRoot: "/private/memory", StateRoot: "/private/state",
		ConfigRoot: "/private/config", HomeRoot: "/private/home", BridgeRoot: "/private/bridge", ProductBinary: "/private/gpm", ClaudeBinary: "/private/claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertMount(t, plan.ClaudeMounts, "/private/source", "/workspace", true)
	assertNoMountSource(t, plan.ClaudeMounts, "/private/memory", "/private/state")
	assertMount(t, plan.ServerMounts, "/private/source", "/workspace", true)
	assertMount(t, plan.ServerMounts, "/private/memory", "/memory", false)
	assertMount(t, plan.ServerMounts, "/private/state", "/state", false)
	if strings.Contains(strings.Join(plan.ClaudeArguments, "\x00"), "/private/memory") || strings.Contains(strings.Join(plan.ClaudeArguments, "\x00"), "/private/state") {
		t.Fatal("Claude arguments expose a backing root")
	}
	if !slices.Equal(plan.MCPCommand, []string{"/usr/bin/socat", "STDIO", "UNIX-CONNECT:/bridge/mcp.sock"}) {
		t.Fatalf("MCP command = %q", plan.MCPCommand)
	}
}

func TestBuildSessionPlanKeepsTaskArmTopologyIdentical(t *testing.T) {
	control, err := buildSessionPlan(sessionSpec{
		Kind: "task", Arm: "control", Workspace: "/runs/control-code", MemoryRoot: "/runs/control-memory", StateRoot: "/runs/control-state",
		ConfigRoot: "/runs/control-config", HomeRoot: "/runs/control-home", BridgeRoot: "/runs/control-bridge", ProductBinary: "/bin/gpm", ClaudeBinary: "/bin/claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := buildSessionPlan(sessionSpec{
		Kind: "task", Arm: "treatment", Workspace: "/runs/treatment-code", MemoryRoot: "/runs/treatment-memory", StateRoot: "/runs/treatment-state",
		ConfigRoot: "/runs/treatment-config", HomeRoot: "/runs/treatment-home", BridgeRoot: "/runs/treatment-bridge", ProductBinary: "/bin/gpm", ClaudeBinary: "/bin/claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	if control.Shape != treatment.Shape || !slices.Equal(control.AllowedTools, treatment.AllowedTools) || !slices.Equal(control.DeniedTools, treatment.DeniedTools) {
		t.Fatalf("arm topology differs: control=%+v treatment=%+v", control, treatment)
	}
	assertMount(t, control.ClaudeMounts, "/runs/control-code", "/workspace", false)
	assertNoMountSource(t, control.ClaudeMounts, "/runs/control-memory", "/runs/control-state")
	assertMount(t, control.ServerMounts, "/runs/control-memory", "/memory", true)
	for _, denied := range []string{"mcp__git-push-memories__memory_reference_snapshot", "mcp__git-push-memories__memory_propose"} {
		if !slices.Contains(control.DeniedTools, denied) {
			t.Fatalf("task plan permits %s", denied)
		}
	}
}

func assertMount(t *testing.T, mounts []sandboxMount, source, target string, readOnly bool) {
	t.Helper()
	for _, mount := range mounts {
		if mount.Source == source && mount.Target == target && mount.ReadOnly == readOnly {
			return
		}
	}
	t.Fatalf("mount %s -> %s readOnly=%v not found in %+v", source, target, readOnly, mounts)
}

func assertNoMountSource(t *testing.T, mounts []sandboxMount, sources ...string) {
	t.Helper()
	for _, mount := range mounts {
		if slices.Contains(sources, mount.Source) {
			t.Fatalf("private source %s is mounted", mount.Source)
		}
	}
}
