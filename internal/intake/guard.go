package intake

import (
	"bytes"
	"encoding/json"
	"io"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxCandidateBytes   = 256 * 1024
	maxReplacementBytes = 128 * 1024
	maxHeadingBytes     = 256
	maxPathBytes        = 1024
	maxSourcePaths      = 16
)

var sectionIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var normativePatterns = []string{
	`-----BEGIN[ A-Z0-9_-]*PRIVATE KEY-----`,
	`(?i)\bauthorization\s*:\s*(basic|bearer)\s+\S+`,
	`(?i)\b(api[_-]?key|access[_-]?token|secret|password)\b\s*[:=]\s*\S+`,
	`\b(AKIA|ASIA)[0-9A-Z]{16}\b`,
	`\bgh[pousr]_[A-Za-z0-9]{36,}\b`,
	`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`,
	`\bsk-[A-Za-z0-9_-]{20,}\b`,
	"\\b[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+\\b",
	`\+[1-9][0-9]{7,14}\b`,
	`(?i)\b(session[_-]?id|transcript)\s*[:=]\s*\S+`,
	`(?i)(^|[\\/])\.claude[\\/].*\.jsonl(?:$|[?#])`,
}

type Guard struct {
	detectors []*regexp.Regexp
}

func New() (*Guard, error) {
	return newGuard(nil)
}

func newGuard(extraPatterns []string) (*Guard, error) {
	patterns := append(append([]string(nil), normativePatterns...), extraPatterns...)
	detectors := make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, Rejection{Class: "policy", Reason: "detector_initialization_failed"}
		}
		detectors[i] = compiled
	}
	return &Guard{detectors: detectors}, nil
}

func (guard *Guard) Inspect(candidate Candidate) (AllowedCandidate, error) {
	if len(candidate.raw) > maxCandidateBytes {
		return AllowedCandidate{}, rejection("limits", "limit_exceeded")
	}
	if !utf8.Valid(candidate.raw) {
		return AllowedCandidate{}, rejection("schema", "invalid_encoding")
	}
	fields, err := decodeClosedObject(candidate.raw)
	if err != nil {
		return AllowedCandidate{}, err
	}
	var decoded struct {
		Operation   string
		TargetPath  string
		SectionID   string
		Replacement string
		Title       string
		Heading     string
		Body        string
		SourcePaths []string
	}
	if raw, exists := fields["operation"]; exists {
		if err := decodeRawField(raw, &decoded.Operation); err != nil {
			return AllowedCandidate{}, err
		}
	} else {
		decoded.Operation = OperationReplaceSection
	}
	if err := decodeField(fields, "target_path", &decoded.TargetPath); err != nil {
		return AllowedCandidate{}, err
	}
	switch decoded.Operation {
	case OperationReplaceSection:
		if !hasExactly(fields, "target_path", "section_id", "replacement", "source_paths") && !hasExactly(fields, "operation", "target_path", "section_id", "replacement", "source_paths") {
			return AllowedCandidate{}, rejection("schema", "invalid_candidate")
		}
		if err := decodeField(fields, "section_id", &decoded.SectionID); err != nil {
			return AllowedCandidate{}, err
		}
		if err := decodeField(fields, "replacement", &decoded.Replacement); err != nil {
			return AllowedCandidate{}, err
		}
	case OperationCreatePage:
		if !hasExactly(fields, "operation", "target_path", "title", "body", "source_paths") {
			return AllowedCandidate{}, rejection("schema", "invalid_candidate")
		}
		if err := decodeField(fields, "title", &decoded.Title); err != nil {
			return AllowedCandidate{}, err
		}
		if err := decodeField(fields, "body", &decoded.Body); err != nil {
			return AllowedCandidate{}, err
		}
	case OperationCreateSection:
		if !hasExactly(fields, "operation", "target_path", "heading", "body", "source_paths") {
			return AllowedCandidate{}, rejection("schema", "invalid_candidate")
		}
		if err := decodeField(fields, "heading", &decoded.Heading); err != nil {
			return AllowedCandidate{}, err
		}
		if err := decodeField(fields, "body", &decoded.Body); err != nil {
			return AllowedCandidate{}, err
		}
	default:
		return AllowedCandidate{}, rejection("schema", "invalid_operation")
	}
	if err := decodeField(fields, "source_paths", &decoded.SourcePaths); err != nil {
		return AllowedCandidate{}, err
	}
	if len(decoded.TargetPath) > maxPathBytes || len(decoded.Replacement) > maxReplacementBytes || len(decoded.Body) > maxReplacementBytes || len(decoded.Title) > maxHeadingBytes || len(decoded.Heading) > maxHeadingBytes || len(decoded.SourcePaths) > maxSourcePaths {
		return AllowedCandidate{}, rejection("limits", "limit_exceeded")
	}
	for _, sourcePath := range decoded.SourcePaths {
		if len(sourcePath) > maxPathBytes {
			return AllowedCandidate{}, rejection("limits", "limit_exceeded")
		}
	}
	if invalidControls(decoded.Operation, false) || invalidControls(decoded.TargetPath, false) || invalidControls(decoded.SectionID, false) || invalidControls(decoded.Replacement, true) || invalidControls(decoded.Title, false) || invalidControls(decoded.Heading, false) || invalidControls(decoded.Body, true) {
		return AllowedCandidate{}, rejection("privacy", "control_character")
	}
	for _, sourcePath := range decoded.SourcePaths {
		if invalidControls(sourcePath, false) {
			return AllowedCandidate{}, rejection("privacy", "control_character")
		}
	}
	rawValues := []string{decoded.Operation, decoded.TargetPath, decoded.SectionID, decoded.Replacement, decoded.Title, decoded.Heading, decoded.Body}
	rawValues = append(rawValues, decoded.SourcePaths...)
	for _, value := range rawValues {
		if guard.sensitive(value) {
			return AllowedCandidate{}, rejection("privacy", "sensitive_content")
		}
	}
	canonicalTarget, err := validatePath(decoded.TargetPath)
	if err != nil {
		return AllowedCandidate{}, err
	}
	canonicalSources := make([]string, len(decoded.SourcePaths))
	for i, sourcePath := range decoded.SourcePaths {
		canonicalSources[i], err = validatePath(sourcePath)
		if err != nil {
			return AllowedCandidate{}, err
		}
	}
	for _, value := range append([]string{canonicalTarget}, canonicalSources...) {
		if guard.sensitive(value) {
			return AllowedCandidate{}, rejection("privacy", "sensitive_content")
		}
	}
	if decoded.Operation == OperationReplaceSection && (!sectionIDPattern.MatchString(decoded.SectionID) || len(decoded.SectionID) > 64) {
		return AllowedCandidate{}, rejection("schema", "invalid_section_id")
	}
	return AllowedCandidate{
		operation: decoded.Operation, targetPath: canonicalTarget, sectionID: decoded.SectionID,
		replacement: decoded.Replacement, title: decoded.Title, heading: decoded.Heading, body: decoded.Body, sourcePaths: canonicalSources,
	}, nil
}

func decodeClosedObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, rejection("schema", "invalid_candidate")
	}
	fields := make(map[string]json.RawMessage, 7)
	allowed := map[string]bool{"operation": true, "target_path": true, "section_id": true, "replacement": true, "title": true, "heading": true, "body": true, "source_paths": true}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, rejection("schema", "invalid_candidate")
		}
		name, ok := token.(string)
		if !ok || !allowed[name] {
			return nil, rejection("schema", "unknown_field")
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, rejection("schema", "duplicate_field")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, rejection("schema", "invalid_candidate")
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, rejection("schema", "invalid_candidate")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, rejection("schema", "invalid_candidate")
	}
	return fields, nil
}

func decodeField(fields map[string]json.RawMessage, name string, target any) error {
	raw, exists := fields[name]
	if !exists {
		return rejection("schema", "invalid_candidate")
	}
	return decodeRawField(raw, target)
}

func decodeRawField(raw json.RawMessage, target any) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return rejection("schema", "invalid_candidate")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return rejection("schema", "invalid_candidate")
	}
	return nil
}

func hasExactly(fields map[string]json.RawMessage, names ...string) bool {
	if len(fields) != len(names) {
		return false
	}
	for _, name := range names {
		if _, exists := fields[name]; !exists {
			return false
		}
	}
	return true
}

func validatePath(value string) (string, error) {
	normalized := strings.ReplaceAll(value, "\\", "/")
	if normalized == "" || path.IsAbs(normalized) || (len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/') {
		return "", rejection("schema", "invalid_path")
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return "", rejection("schema", "invalid_path")
		}
	}
	canonical := path.Clean(normalized)
	if canonical == "." || canonical == ".." || strings.HasPrefix(canonical, "../") {
		return "", rejection("schema", "invalid_path")
	}
	return canonical, nil
}

func invalidControls(value string, replacement bool) bool {
	for _, character := range value {
		if character <= 0x1f || (character >= 0x7f && character <= 0x9f) {
			if replacement && (character == '\t' || character == '\r' || character == '\n') {
				continue
			}
			return true
		}
	}
	return false
}

func (guard *Guard) sensitive(value string) bool {
	for _, detector := range guard.detectors {
		if detector.MatchString(value) {
			return true
		}
	}
	return validCPFIn(value)
}

func validCPFIn(value string) bool {
	for start := 0; start < len(value); start++ {
		if value[start] < '0' || value[start] > '9' || (start > 0 && value[start-1] >= '0' && value[start-1] <= '9') {
			continue
		}
		var digits strings.Builder
		end := start
		for end < len(value) {
			character := value[end]
			if character >= '0' && character <= '9' {
				digits.WriteByte(character)
				end++
				if digits.Len() > 11 {
					break
				}
				continue
			}
			if character == '.' || character == '-' || character == ' ' {
				end++
				continue
			}
			break
		}
		if digits.Len() == 11 && validCPF(digits.String()) {
			return true
		}
	}
	return false
}

func validCPF(value string) bool {
	if len(value) != 11 || strings.Count(value, value[:1]) == 11 {
		return false
	}
	for check := 9; check < 11; check++ {
		sum := 0
		for i := 0; i < check; i++ {
			sum += int(value[i]-'0') * (check + 1 - i)
		}
		digit := 11 - sum%11
		if digit >= 10 {
			digit = 0
		}
		if digit != int(value[check]-'0') {
			return false
		}
	}
	return true
}

func rejection(class, reason string) error {
	return Rejection{Class: class, Reason: reason}
}
