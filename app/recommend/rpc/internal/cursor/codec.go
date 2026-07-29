package cursor

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	cursorVersion = 1
	maxTokenSize  = 4096
)

var (
	ErrInvalid  = errors.New("invalid recommendation cursor")
	ErrExpired  = errors.New("expired recommendation cursor")
	ErrMismatch = errors.New("recommendation cursor binding mismatch")
)

type Binding struct {
	IdentityHash string
	RequestID    string
	Scene        string
	SessionID    string
	ExperimentID string
	PageSize     int
}

type Payload struct {
	Version      int    `json:"v"`
	SnapshotID   string `json:"sid"`
	Offset       int    `json:"offset"`
	ExpiresAt    int64  `json:"exp"`
	IdentityHash string `json:"identity"`
	RequestID    string `json:"request"`
	Scene        string `json:"scene"`
	SessionID    string `json:"session,omitempty"`
	ExperimentID string `json:"experiment,omitempty"`
	PageSize     int    `json:"page_size"`
}

type Codec struct {
	secret []byte
	now    func() time.Time
}

func New(secret string, now func() time.Time) (*Codec, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("recommend cursor secret must be at least 32 bytes")
	}
	if now == nil {
		now = time.Now
	}
	return &Codec{secret: []byte(secret), now: now}, nil
}

func (c *Codec) Encode(snapshotID string, offset int, expiresAt int64, binding Binding) (string, error) {
	if snapshotID == "" || offset <= 0 || expiresAt <= c.now().Unix() || binding.IdentityHash == "" ||
		binding.RequestID == "" || binding.Scene == "" || binding.PageSize <= 0 {
		return "", ErrInvalid
	}
	payload := Payload{
		Version:      cursorVersion,
		SnapshotID:   snapshotID,
		Offset:       offset,
		ExpiresAt:    expiresAt,
		IdentityHash: binding.IdentityHash,
		RequestID:    binding.RequestID,
		Scene:        binding.Scene,
		SessionID:    binding.SessionID,
		ExperimentID: binding.ExperimentID,
		PageSize:     binding.PageSize,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal recommendation cursor: %w", err)
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(encoded)
	signaturePart := base64.RawURLEncoding.EncodeToString(c.sign([]byte(payloadPart)))
	return payloadPart + "." + signaturePart, nil
}

func (c *Codec) Decode(token string, binding Binding) (Payload, error) {
	if token == "" || len(token) > maxTokenSize {
		return Payload{}, ErrInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Payload{}, ErrInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, c.sign([]byte(parts[0]))) {
		return Payload{}, ErrInvalid
	}
	encoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Payload{}, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var payload Payload
	if err := decoder.Decode(&payload); err != nil {
		return Payload{}, ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Payload{}, ErrInvalid
	}
	if payload.Version != cursorVersion || payload.SnapshotID == "" || payload.Offset <= 0 || payload.ExpiresAt <= 0 || payload.PageSize <= 0 {
		return Payload{}, ErrInvalid
	}
	if payload.ExpiresAt <= c.now().Unix() {
		return Payload{}, ErrExpired
	}
	if payload.IdentityHash != binding.IdentityHash || payload.RequestID != binding.RequestID ||
		payload.Scene != binding.Scene || payload.SessionID != binding.SessionID ||
		payload.ExperimentID != binding.ExperimentID || payload.PageSize != binding.PageSize {
		return Payload{}, ErrMismatch
	}
	return payload, nil
}

func IdentityHash(identity string) string {
	digest := sha256.Sum256([]byte(identity))
	return base64.RawURLEncoding.EncodeToString(digest[:16])
}

func (c *Codec) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}
