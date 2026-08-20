package alerter

import (
	"encoding/json/v2"
	"io"

	"resty.dev/v3"
)

// jsonOptions are the encode/decode semantics every alerter payload uses.
//
// Deterministic keeps map iteration order stable, so two runs producing the
// same alert produce byte-identical bodies — which is what makes request
// bodies comparable in tests and diffable in a proxy log.
// FormatNilSliceAsNull(false) keeps an alerter that has nothing to attach
// sending `[]` rather than `null`, which webhook receivers handle better.
//
// OmitZeroStructFields is deliberately left off: which fields may disappear
// from a payload is a property of the payload, not of the transport, so it
// stays on the `omitzero` tags where a reader of the struct can see it.
var jsonOptions = json.JoinOptions(
	json.Deterministic(true),
	json.FormatNilSliceAsNull(false),
)

// encodeJSON streams v into w. json/v2 writes directly to the writer instead
// of marshaling into an intermediate []byte the way encoding/json does.
func encodeJSON(w io.Writer, v any) error {
	return json.MarshalWrite(w, v, jsonOptions)
}

// decodeJSON streams the response body into v. Unknown members are accepted:
// provider APIs add fields over time and an alerter must not start failing
// because of one.
func decodeJSON(r io.Reader, v any) error {
	return json.UnmarshalRead(r, v, jsonOptions)
}

// withJSONv2 swaps resty's default encoding/json handling for json/v2.
func withJSONv2(c *resty.Client) *resty.Client {
	return c.
		AddContentTypeEncoder("json", encodeJSON).
		AddContentTypeDecoder("json", decodeJSON)
}
