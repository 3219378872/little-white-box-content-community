package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// DigestArgs parses tool argument JSON (object or empty) then digests it.
func DigestArgs(raw string) (string, error) {
	if raw == "" {
		raw = "{}"
	}
	var generic any
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		return "", fmt.Errorf("canonical args: %w", err)
	}
	return DigestSHA256(generic)
}
