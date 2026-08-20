package alerter

import (
	"bytes"
	"strings"
	"testing"
)

// The alerter payloads are encoded with encoding/json/v2, which changes two
// things the wire format depends on: omitzero drops empty optional fields, and
// Deterministic pins map ordering so two identical alerts serialise
// identically.
func TestEncodeJSONAppliesV2Semantics(t *testing.T) {
	payload := mattermostPayload{
		// Channel, Username and IconURL are left empty: a webhook that was
		// not given overrides must fall back to its own defaults, which it
		// only does when the keys are absent rather than present-and-empty.
		Text: "down",
		Attachments: []mattermostAttachment{{
			Fallback: "🔴 FAILURE — api",
			Color:    mattermostColorFailure,
			Title:    "🔴 FAILURE — api",
			Fields: []mattermostField{
				{Title: "URL", Value: "`https://api.example.com/health`"},
				{Title: "Expected", Value: "200", Short: true},
			},
		}},
	}

	var buf bytes.Buffer
	if err := encodeJSON(&buf, payload); err != nil {
		t.Fatalf("encodeJSON: %v", err)
	}

	got := buf.String()

	for _, absent := range []string{`"channel"`, `"username"`, `"icon_url"`} {
		if strings.Contains(got, absent) {
			t.Errorf("payload contains %s; omitzero should have dropped it:\n%s", absent, got)
		}
	}

	for _, present := range []string{`"text":"down"`, `"attachments"`, `"color":"` + mattermostColorFailure + `"`} {
		if !strings.Contains(got, present) {
			t.Errorf("payload is missing %s:\n%s", present, got)
		}
	}

	// Deterministic(true) means re-encoding the same value is byte-identical.
	var again bytes.Buffer
	if err := encodeJSON(&again, payload); err != nil {
		t.Fatalf("encodeJSON (second pass): %v", err)
	}
	if again.String() != got {
		t.Errorf("re-encoding changed the body:\n%s\n%s", got, again.String())
	}
}

// An empty attachment list has to serialise as [] rather than null, which is
// what FormatNilSliceAsNull(false) buys.
func TestEncodeJSONWritesEmptySlicesAsArrays(t *testing.T) {
	var buf bytes.Buffer
	if err := encodeJSON(&buf, mattermostPayload{}); err != nil {
		t.Fatalf("encodeJSON: %v", err)
	}

	if got := buf.String(); !strings.Contains(got, `"attachments":[]`) {
		t.Errorf("got %s, want attachments encoded as an empty array", got)
	}
}

// Provider APIs grow fields over time; decoding must not start failing when
// they do.
func TestDecodeJSONIgnoresUnknownMembers(t *testing.T) {
	const body = `{"ok":false,"error_code":403,"description":"blocked","retry_after":30}`

	var got telegramResponse
	if err := decodeJSON(strings.NewReader(body), &got); err != nil {
		t.Fatalf("decodeJSON: %v", err)
	}

	if got.OK {
		t.Error("OK = true, want false")
	}
	if got.ErrorCode != 403 {
		t.Errorf("ErrorCode = %d, want 403", got.ErrorCode)
	}
	if got.Description != "blocked" {
		t.Errorf("Description = %q, want %q", got.Description, "blocked")
	}
}
