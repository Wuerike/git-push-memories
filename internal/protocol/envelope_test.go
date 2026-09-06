package protocol

import (
	"encoding/json"
	"testing"
)

func TestEnvelopeCarriesSchemaVersion(t *testing.T) {
	type payload struct {
		ID string `json:"id"`
	}

	encoded, err := json.Marshal(NewEnvelope(payload{ID: "proposal-1"}))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"schema_version":1,"data":{"id":"proposal-1"}}`
	if string(encoded) != want {
		t.Fatalf("json.Marshal() = %s, want %s", encoded, want)
	}
}
