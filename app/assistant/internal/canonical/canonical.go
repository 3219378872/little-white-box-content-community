package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// JSON returns a deterministic encoding: nested objects use sorted keys.
func JSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical json: %w", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("canonical json decode: %w", err)
	}
	out, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("canonical json encode: %w", err)
	}
	return out, nil
}

// DigestSHA256 hex-encodes SHA-256 of CanonicalJSON(v).
func DigestSHA256(v any) (string, error) {
	raw, err := JSON(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// UnwrapArgsJSON peels a JSON-string-wrapped object (Responses `arguments` is a
// string) so executors receive a JSON object. At most two layers are unwrapped.
func UnwrapArgsJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	for i := 0; i < 2; i++ {
		if len(raw) < 2 || raw[0] != '"' {
			return raw
		}
		var inner string
		if err := json.Unmarshal([]byte(raw), &inner); err != nil {
			return raw
		}
		inner = strings.TrimSpace(inner)
		if inner == "" {
			return "{}"
		}
		raw = inner
	}
	return raw
}

// DigestArgs parses tool argument JSON (object or empty) then digests it.
func DigestArgs(raw string) (string, error) {
	raw = UnwrapArgsJSON(raw)
	if raw == "" {
		raw = "{}"
	}
	var generic any
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		return "", fmt.Errorf("canonical args: %w", err)
	}
	return DigestSHA256(generic)
}
