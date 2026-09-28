package logging

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFrameworkDiagnosticsNeverRenderArgumentsOrFormats(t *testing.T) {
	var logs bytes.Buffer
	previous := Reset()
	SetWriter(&logs)
	defer SetWriter(previous)
	logger := &frameworkLogger{component: "kitex"}
	secret := "synthetic-private-body"
	logger.Error(secret)
	logger.Errorf(secret+" %v", secret)
	logger.CtxErrorf(WithTraceID(context.Background(), "trace-safe"), secret+" %v", secret)
	require.NotContains(t, logs.String(), secret)
	require.Equal(t, 3, strings.Count(logs.String(), "framework diagnostic"))
	require.Contains(t, logs.String(), "trace-safe")
	require.Contains(t, logs.String(), `"level":"ERROR"`)
}
func TestConfiguredLevelPreservesContext(t *testing.T) {
	var logs bytes.Buffer
	previous := Reset()
	SetWriter(&logs)
	defer SetWriter(previous)
	require.NoError(t, Configure(Config{Level: "warn"}))
	defer func() { _ = Configure(Config{Level: "info"}) }()
	logger := WithContext(WithTraceID(context.Background(), "trace-safe"))
	logger.Infow("filtered")
	logger.Errorw("kept")
	require.NotContains(t, logs.String(), "filtered")
	require.Contains(t, logs.String(), "trace-safe")
}
