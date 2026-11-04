package memory

import (
	"bytes"
	"errors"
	"testing"
)

func TestPrepareDocumentPreservesAndCompletesIDs(t *testing.T) {
	baseline := []byte("# Checkout\n<!-- @id: checkout -->\n\n## Old title\n<!-- @id: checkout-stable -->\nOwnership stays blue.\n")
	current := []byte("# Checkout\n<!-- @id: checkout -->\n\n## Ownership\n<!-- @id: checkout-stable -->\nOwnership stays blue.\n\n## Timeout\nNine seconds.\n")
	want := []byte("# Checkout\n<!-- @id: checkout -->\n\n## Ownership\n<!-- @id: checkout-stable -->\nOwnership stays blue.\n\n## Timeout\n<!-- @id: checkout-timeout -->\nNine seconds.\n")
	got, err := PrepareDocument("memories/domains/checkout.md", baseline, current)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("prepared = %q, %v; want %q", got, err, want)
	}
}

func TestPrepareDocumentIsStableAcrossChecks(t *testing.T) {
	current := []byte("# Retry\n\n## Policy\nThree attempts.\n")
	want := []byte("# Retry\n<!-- @id: retry -->\n\n## Policy\n<!-- @id: retry-policy -->\nThree attempts.\n")
	first, err := PrepareDocument("memories/domains/retry.md", nil, current)
	if err != nil || !bytes.Equal(first, want) {
		t.Fatalf("first = %q, %v; want %q", first, err, want)
	}
	second, err := PrepareDocument("memories/domains/retry.md", nil, first)
	if err != nil || !bytes.Equal(second, want) {
		t.Fatalf("second = %q, %v; want %q", second, err, want)
	}
}

func TestPrepareDocumentPreservesFrontmatterAndHumanText(t *testing.T) {
	baseline := []byte("---\ntitle: Checkout\n---\n# Checkout\n<!-- @id: checkout -->\nOld body.\n")
	current := []byte("---\ntitle: Checkout\n---\n# Checkout\n<!-- @id: checkout -->\nHuman edit: retain **this** exact text.\n")
	got, err := PrepareDocument("memories/domains/checkout.md", baseline, current)
	if err != nil || !bytes.Equal(got, current) {
		t.Fatalf("prepared = %q, %v; want current bytes %q", got, err, current)
	}
	unchanged, err := PrepareDocument("memories/domains/checkout.md", baseline, baseline)
	if err != nil || !bytes.Equal(unchanged, baseline) {
		t.Fatalf("unchanged = %q, %v", unchanged, err)
	}
}

func TestPrepareDocumentRejectsInvalidStructure(t *testing.T) {
	for name, current := range map[string][]byte{
		"missing heading":     []byte("Body only.\n"),
		"two titles":          []byte("# One\n# Two\n"),
		"malformed marker":    []byte("# Checkout\n<!-- @id: BAD -->\nBody.\n"),
		"duplicate IDs":       []byte("# Checkout\n<!-- @id: checkout -->\n## Policy\n<!-- @id: checkout -->\nBody.\n"),
		"unresolved conflict": []byte("# Checkout\n<<<<<<< HEAD\nBody.\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := PrepareDocument("memories/domains/checkout.md", nil, current); !errors.Is(err, ErrInvalidSchema) {
				t.Fatalf("error = %v, want invalid schema", err)
			}
		})
	}
}
