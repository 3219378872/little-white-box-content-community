package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// JSON returns a deterministic encoding: nested objects use sorted keys.
func JSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical json: %w", err)
	}
	generic, err := decodeJSON(raw)
	if err != nil {
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
	generic, err := decodeJSON([]byte(raw))
	if err != nil {
		return "", fmt.Errorf("canonical args: %w", err)
	}
	return DigestSHA256(generic)
}

// decodeJSON keeps integer lexemes as json.Number. Decoding through the
// default interface path would round large snowflake IDs through float64 and
// make retries hash a different command than the original request.
func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value")
		}
		return nil, err
	}
	return value, nil
}
