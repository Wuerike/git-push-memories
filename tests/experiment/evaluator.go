package experiment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type candidateEvaluationSpec struct {
	RunID         string
	Scenario      string
	HiddenTest    string
	Workspace     string
	EvaluatorPath string
	ExpectedAPI   []string
}

type BlindEvaluationRecord struct {
	RunID          string   `json:"run_id"`
	Scenario       string   `json:"scenario"`
	EvaluatorValid bool     `json:"evaluator_valid"`
	PublicTests    bool     `json:"public_tests"`
	HiddenTest     bool     `json:"hidden_test"`
	APIPreserved   bool     `json:"api_preserved"`
	ScopeValid     bool     `json:"scope_valid"`
	Correctness    bool     `json:"correctness"`
	Score          int      `json:"score"`
	ChangedPaths   []string `json:"changed_paths"`
	DiffHash       string   `json:"diff_hash"`
	CandidateHash  string   `json:"candidate_hash"`
	PublicTestMS   int64    `json:"public_test_ms"`
	HiddenTestMS   int64    `json:"hidden_test_ms"`
	APICheckMS     int64    `json:"api_check_ms"`
}

func evaluateCandidate(ctx context.Context, spec candidateEvaluationSpec) BlindEvaluationRecord {
	result := BlindEvaluationRecord{RunID: spec.RunID, Scenario: spec.Scenario, EvaluatorValid: true}
	publicStarted := time.Now()
	result.PublicTests = runQuiet(ctx, spec.Workspace, "go", "test", "./...", "-count=1") == nil
	result.PublicTestMS = time.Since(publicStarted).Milliseconds()

	apiStarted := time.Now()
	actualAPI, apiErr := exportedAPISignatures(filepath.Join(spec.Workspace, "policy"))
	result.APICheckMS = time.Since(apiStarted).Milliseconds()
	result.APIPreserved = apiErr == nil && slicesEqual(actualAPI, spec.ExpectedAPI)

	paths, diffHash, candidateHash, diffErr := candidateDiffEvidence(ctx, spec.Workspace)
	result.ChangedPaths, result.DiffHash, result.CandidateHash = sanitizeCandidatePaths(paths), diffHash, candidateHash
	result.ScopeValid = diffErr == nil && len(paths) > 0
	for _, path := range paths {
		matched, _ := filepath.Match("policy/*.go", filepath.ToSlash(path))
		if !matched {
			result.ScopeValid = false
		}
	}

	hiddenPath := filepath.Join(spec.Workspace, "policy", "policy_hidden_test.go")
	if _, err := os.Lstat(hiddenPath); !errors.Is(err, os.ErrNotExist) {
		result.EvaluatorValid = false
	} else {
		contents, err := os.ReadFile(spec.EvaluatorPath)
		if err != nil || os.WriteFile(hiddenPath, contents, 0o600) != nil {
			result.EvaluatorValid = false
		} else {
			hiddenStarted := time.Now()
			result.HiddenTest = runQuiet(ctx, spec.Workspace, "go", "test", "./policy", "-run", "^"+spec.HiddenTest+"$", "-count=1") == nil
			result.HiddenTestMS = time.Since(hiddenStarted).Milliseconds()
			if err := os.Remove(hiddenPath); err != nil {
				result.EvaluatorValid = false
				result.HiddenTest = false
			}
		}
	}
	if result.HiddenTest {
		result.Score += 4
	}
	if result.PublicTests {
		result.Score += 2
	}
	if result.APIPreserved {
		result.Score += 2
	}
	if result.ScopeValid {
		result.Score += 2
	}
	result.Correctness = result.EvaluatorValid && result.HiddenTest && result.PublicTests && result.APIPreserved
	return result
}

func sanitizeCandidatePaths(paths []string) []string {
	safe := regexp.MustCompile(`^policy/[A-Za-z0-9._-]+\.go$`)
	result := make([]string, len(paths))
	for index, path := range paths {
		if safe.MatchString(path) {
			result[index] = path
		} else {
			result[index] = "out_of_scope"
		}
	}
	return result
}

func exportedAPISignatures(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	fileSet := token.NewFileSet()
	var signatures []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fileSet, filepath.Join(directory, entry.Name()), nil, 0)
		if err != nil {
			return nil, err
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !function.Name.IsExported() {
				continue
			}
			parameters, err := fieldTypes(fileSet, function.Type.Params)
			if err != nil {
				return nil, err
			}
			results, err := fieldTypes(fileSet, function.Type.Results)
			if err != nil {
				return nil, err
			}
			signatures = append(signatures, function.Name.Name+"("+strings.Join(parameters, ",")+")->("+strings.Join(results, ",")+")")
		}
	}
	sort.Strings(signatures)
	return signatures, nil
}

func fieldTypes(fileSet *token.FileSet, fields *ast.FieldList) ([]string, error) {
	if fields == nil {
		return nil, nil
	}
	var result []string
	for _, field := range fields.List {
		var rendered bytes.Buffer
		if err := format.Node(&rendered, fileSet, field.Type); err != nil {
			return nil, err
		}
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			result = append(result, rendered.String())
		}
	}
	return result, nil
}

func candidateDiffEvidence(ctx context.Context, workspace string) ([]string, string, string, error) {
	diff, err := commandBytes(ctx, workspace, "git", "diff", "--binary", "--no-ext-diff")
	if err != nil {
		return nil, "", "", err
	}
	tracked, err := commandBytes(ctx, workspace, "git", "diff", "--name-only")
	if err != nil {
		return nil, "", "", err
	}
	untracked, err := commandBytes(ctx, workspace, "git", "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, "", "", err
	}
	paths := append(strings.Fields(string(tracked)), strings.Fields(string(untracked))...)
	sort.Strings(paths)
	combined := append([]byte(nil), diff...)
	candidateHasher := sha256.New()
	for _, path := range paths {
		contents, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
		if err != nil {
			return nil, "", "", err
		}
		_, _ = candidateHasher.Write([]byte(path + "\x00"))
		_, _ = candidateHasher.Write(contents)
		if strings.Contains(string(untracked), path) {
			combined = append(combined, []byte("\x00"+path+"\x00")...)
			combined = append(combined, contents...)
		}
	}
	diffDigest := sha256.Sum256(combined)
	return paths, hex.EncodeToString(diffDigest[:]), hex.EncodeToString(candidateHasher.Sum(nil)), nil
}

func runQuiet(ctx context.Context, directory, name string, arguments ...string) error {
	goRoot, err := commandBytes(ctx, directory, "go", "env", "GOROOT")
	if err != nil {
		return err
	}
	goRootPath := strings.TrimSpace(string(goRoot))
	bwrap := sandboxBaseArguments()
	bwrap = append(bwrap,
		"--unshare-net", "--dir", "/home", "--dir", "/home/evaluator", "--dir", "/workspace", "--dir", "/goroot", "--ro-bind", directory, "/workspace", "--ro-bind", goRootPath, "/goroot",
		"--chdir", "/workspace", "--setenv", "PATH", "/goroot/bin:/usr/local/bin:/usr/bin:/bin", "--setenv", "GOROOT", "/goroot", "--setenv", "HOME", "/home/evaluator",
		"--setenv", "GOCACHE", "/tmp/go-cache", "--setenv", "GOTOOLCHAIN", "go1.25.12", "--setenv", "GOPROXY", "off",
		"--setenv", "GIT_CONFIG_NOSYSTEM", "1", "--setenv", "GIT_CONFIG_GLOBAL", "/dev/null", "--setenv", "TZ", "UTC", "--setenv", "LC_ALL", "C.UTF-8",
		"--", name,
	)
	bwrap = append(bwrap, arguments...)
	command := exec.CommandContext(ctx, "bwrap", bwrap...)
	return command.Run()
}

func commandBytes(ctx context.Context, directory, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = directory
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "TZ=UTC", "LC_ALL=C.UTF-8"}
	return command.Output()
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
