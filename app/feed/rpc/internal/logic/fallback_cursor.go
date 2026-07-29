package logic

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const fallbackCursorPrefix = "feedv2."

type fallbackCursor struct {
	Version   int    `json:"v"`
	Page      int32  `json:"p"`
	RequestID string `json:"r"`
	ExpiresAt int64  `json:"e"`
}

func encodeFallbackCursor(secret, requestID string, page int32, now time.Time) (string, error) {
	if secret == "" || requestID == "" || page <= 0 {
		return "", fmt.Errorf("invalid fallback cursor input")
	}
	payload, err := json.Marshal(fallbackCursor{
		Version: 1, Page: page, RequestID: requestID, ExpiresAt: now.Add(30 * time.Minute).Unix(),
	})
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := signFallbackCursor(secret, encoded)
	return fallbackCursorPrefix + encoded + "." + signature, nil
}

func decodeFallbackCursor(secret, token, requestID string, now time.Time) (int32, bool, error) {
	if !strings.HasPrefix(token, fallbackCursorPrefix) {
		return 0, false, nil
	}
	if secret == "" || requestID == "" {
		return 0, true, fmt.Errorf("fallback cursor cannot be verified")
	}
	parts := strings.Split(strings.TrimPrefix(token, fallbackCursorPrefix), ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(signFallbackCursor(secret, parts[0]))) {
		return 0, true, fmt.Errorf("invalid fallback cursor signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, true, fmt.Errorf("decode fallback cursor: %w", err)
	}
	var cursor fallbackCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return 0, true, fmt.Errorf("parse fallback cursor: %w", err)
	}
	if cursor.Version != 1 || cursor.Page <= 0 || cursor.Page > 10_000 || cursor.RequestID != requestID {
		return 0, true, fmt.Errorf("invalid fallback cursor payload")
	}
	if cursor.ExpiresAt < now.Unix() {
		return 0, true, fmt.Errorf("fallback cursor expired")
	}
	return cursor.Page, true, nil
}

func signFallbackCursor(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
