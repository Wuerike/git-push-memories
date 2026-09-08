package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type command struct {
	Name       string `json:"name"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
}

type manifest struct {
	Commands        []command `json:"commands"`
	CodeCorpusFiles int       `json:"code_corpus_files"`
	CodeCorpusBytes int       `json:"code_corpus_bytes"`
}

type distribution struct {
	P50 int64 `json:"p50_ms"`
	P95 int64 `json:"p95_ms"`
}

func main() {
	root := flag.String("root", "", "directory containing measured run directories")
	output := flag.String("output", "", "summary JSON destination")
	revision := flag.String("revision", "", "evaluated product revision")
	protocolHash := flag.String("protocol-hash", "", "SHA-256 of the pre-registered protocol")
	sourceHash := flag.String("source-hash", "", "combined SHA-256 of evaluated profiling sources")
	flag.Parse()
	if *root == "" || *output == "" || *revision == "" || *protocolHash == "" || *sourceHash == "" {
		fatal("all flags are required")
	}
	paths, err := filepath.Glob(filepath.Join(*root, "run-*", "bootstrap-manifest.json"))
	if err != nil || len(paths) != 10 {
		fatal("expected exactly ten measured manifests")
	}
	stepNames := map[string]string{
		"bootstrap MCP": "mcp_ready", "bootstrap initial search": "search", "bootstrap final expand": "expand",
		"memory_propose_create_page": "prepare", "memory_propose_create_page show": "show",
		"memory_propose_create_page apply": "apply", "memory_propose_create_page publish": "publish",
	}
	values := map[string][]int64{}
	totals := make([]int64, 0, len(paths))
	corpusFiles, corpusBytes := 0, 0
	for _, path := range paths {
		encoded, err := os.ReadFile(path)
		if err != nil {
			fatal("read manifest")
		}
		var current manifest
		if json.Unmarshal(encoded, &current) != nil {
			fatal("decode manifest")
		}
		if current.CodeCorpusFiles <= 0 || current.CodeCorpusBytes <= 0 || corpusFiles != 0 && (current.CodeCorpusFiles != corpusFiles || current.CodeCorpusBytes != corpusBytes) {
			fatal("inconsistent corpus size")
		}
		corpusFiles, corpusBytes = current.CodeCorpusFiles, current.CodeCorpusBytes
		seen := map[string]bool{}
		var total int64
		for _, record := range current.Commands {
			step, selected := stepNames[record.Name]
			if !selected {
				continue
			}
			if record.ExitCode != 0 || seen[step] {
				fatal("invalid measured command")
			}
			seen[step] = true
			values[step] = append(values[step], record.DurationMS)
			total += record.DurationMS
		}
		if len(seen) != len(stepNames) {
			fatal("manifest omitted a measured step")
		}
		totals = append(totals, total)
	}
	distributions := map[string]distribution{}
	optimizationRequired := false
	for name, samples := range values {
		distributions[name] = summarize(samples)
		if distributions[name].P95 > 2000 {
			optimizationRequired = true
		}
	}
	totalDistribution := summarize(totals)
	if totalDistribution.P50 > 53735 {
		optimizationRequired = true
	}
	summary := map[string]any{
		"schema_version": 1, "generated_at": time.Now().UTC(), "product_revision": *revision, "protocol_sha256": *protocolHash, "evaluated_source_sha256": *sourceHash,
		"runs": 10, "warmup_runs": 1, "go_version": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "logical_cpus": runtime.NumCPU(),
		"t10_treatment_reference_p50_ms": 214943, "total_p50_threshold_ms": 53735, "step_p95_threshold_ms": 2000,
		"steps": distributions, "local_total": totalDistribution, "optimization_required": optimizationRequired,
		"payloads_persisted": false, "intermediate_manifests_persisted": false,
		"cpu_model": cpuModel(), "corpus": map[string]int{"initial_markdown_files": 0, "code_files": corpusFiles, "code_bytes": corpusBytes},
		"commands": map[string]string{
			"mcp_ready": "gpm mcp <isolated-roots>", "search": "MCP memory_search <structured-input>", "expand": "MCP memory_expand <handle>",
			"prepare": "MCP memory_propose_create_page <structured-input>", "show": "gpm proposal show <id> <isolated-roots>",
			"apply": "gpm proposal apply <id> --revision 1 <isolated-roots>", "publish": "gpm proposal publish <id> <isolated-roots>",
		},
	}
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil || os.WriteFile(*output, append(encoded, '\n'), 0o600) != nil {
		fatal("write summary")
	}
}

func cpuModel() string {
	contents, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unavailable"
	}
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.HasPrefix(line, "model name") {
			_, value, found := strings.Cut(line, ":")
			if found {
				return strings.TrimSpace(value)
			}
		}
	}
	return "unavailable"
}

func summarize(samples []int64) distribution {
	sorted := append([]int64(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return distribution{P50: sorted[(len(sorted)*50+99)/100-1], P95: sorted[(len(sorted)*95+99)/100-1]}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
