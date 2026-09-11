package experiment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type runnerProtocol struct {
	SchemaVersion     int    `json:"schema_version"`
	ExperimentID      string `json:"experiment_id"`
	ProductRevision   string `json:"product_revision"`
	RandomizationSeed string `json:"randomization_seed"`
	Provider          struct {
		ClaudeVersion       string  `json:"claude_code_version"`
		Model               string  `json:"requested_model"`
		Effort              string  `json:"effort"`
		Autocompact         string  `json:"autocompact"`
		MaximumTotalUSD     float64 `json:"maximum_total_usd"`
		MaximumPreflightUSD float64 `json:"maximum_model_preflight_usd"`
		MaximumCaptureUSD   float64 `json:"maximum_capture_session_usd"`
		MaximumCurationUSD  float64 `json:"maximum_curation_session_usd"`
		MaximumTaskUSD      float64 `json:"maximum_task_session_usd"`
	} `json:"provider"`
	Environment struct {
		GoVersion         string `json:"go_version"`
		GitVersion        string `json:"git_version"`
		BubblewrapVersion string `json:"bubblewrap_version"`
		SocatVersion      string `json:"socat_version"`
	} `json:"environment"`
	SourceRepository struct {
		Revision           string `json:"revision"`
		Tree               string `json:"tree"`
		RepositoryIdentity string `json:"repository_identity"`
	} `json:"source_repository"`
	Execution struct {
		CurationOrder []string `json:"curation_order"`
	} `json:"execution"`
	PromptTemplates struct {
		Capture       string `json:"capture"`
		CaptureSecret string `json:"capture_normative_secret"`
		Curation      string `json:"curation"`
	} `json:"prompt_templates"`
	Budgets struct {
		CaptureTimeoutSeconds  int `json:"capture_timeout_seconds"`
		CurationTimeoutSeconds int `json:"curation_timeout_seconds"`
		TaskTimeoutSeconds     int `json:"task_timeout_seconds"`
	} `json:"budgets"`
	PrivacyInputs struct {
		SecretHash       string `json:"normative_secret_sha256"`
		SourceCanaryKey  string `json:"source_canary_key"`
		SourceCanaryHash string `json:"source_canary_value_sha256"`
	} `json:"privacy_inputs"`
	Pairs []struct {
		ID       string   `json:"id"`
		Scenario string   `json:"scenario"`
		Order    []string `json:"order"`
	} `json:"pairs"`
	FixtureHashes         map[string]string `json:"fixture_hashes"`
	ProductionAssetHashes map[string]string `json:"production_asset_hashes"`
}

type runnerCorpus struct {
	ValueScenarios []struct {
		ID              string          `json:"id"`
		CurationCase    string          `json:"curation_case"`
		Task            string          `json:"task"`
		HiddenTest      string          `json:"hidden_test"`
		ControlOmission controlOmission `json:"control_omission"`
		RelevantTerms   []string        `json:"relevant_terms"`
	} `json:"value_scenarios"`
	CurationCases []struct {
		ID                      string   `json:"id"`
		Class                   string   `json:"class"`
		ProposalFact            string   `json:"proposal_fact"`
		SuggestedAction         string   `json:"suggested_action"`
		SuggestedTarget         string   `json:"suggested_target"`
		ExpectedOperation       string   `json:"expected_operation"`
		ExpectedTarget          string   `json:"expected_target"`
		ExpectedHeading         string   `json:"expected_heading"`
		ExpectedSectionID       string   `json:"expected_section_id"`
		ExpectedDisposition     string   `json:"expected_disposition"`
		ExpectedReason          string   `json:"expected_reason"`
		ExpectedEdit            string   `json:"expected_edit"`
		RequiredPoststate       []string `json:"required_poststate"`
		ForbiddenPoststate      []string `json:"forbidden_poststate"`
		ExpectedIntegrity       string   `json:"expected_reference_integrity"`
		ExpectedSemanticSupport string   `json:"expected_proposal_semantic_support"`
	} `json:"curation_cases"`
}

type runnerConfig struct {
	Repository string
	Fixture    string
	Protocol   runnerProtocol
	Corpus     runnerCorpus
}

func loadRunnerConfig(repository string) (runnerConfig, error) {
	fixture := filepath.Join(repository, "testdata", "phase-3", "value-experiment")
	protocolPath := filepath.Join(fixture, "protocol.json")
	contents, err := os.ReadFile(protocolPath)
	if err != nil {
		return runnerConfig{}, err
	}
	digest := sha256.Sum256(contents)
	if hex.EncodeToString(digest[:]) != Phase3ProtocolSHA256 {
		return runnerConfig{}, errors.New("phase 3 protocol hash mismatch")
	}
	var protocol runnerProtocol
	if err := decodeStrictJSON(contents, &protocol); err != nil {
		return runnerConfig{}, err
	}
	var corpus runnerCorpus
	if err := decodeStrictFile(filepath.Join(fixture, "corpus.json"), &corpus); err != nil {
		return runnerConfig{}, err
	}
	for relative, want := range protocol.FixtureHashes {
		if hashPath(filepath.Join(repository, filepath.FromSlash(relative))) != want {
			return runnerConfig{}, errors.New("fixture hash mismatch: " + relative)
		}
	}
	for relative, want := range protocol.ProductionAssetHashes {
		if hashPath(filepath.Join(repository, filepath.FromSlash(relative))) != want {
			return runnerConfig{}, errors.New("production asset hash mismatch: " + relative)
		}
	}
	return runnerConfig{Repository: repository, Fixture: fixture, Protocol: protocol, Corpus: corpus}, nil
}

func decodeStrictFile(path string, target any) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decodeStrictJSON(contents, target)
}

func decodeStrictJSON(contents []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("JSON contains trailing content")
		}
		return err
	}
	return nil
}

func hashPath(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "unavailable"
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
