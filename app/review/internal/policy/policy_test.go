package policy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadActiveVersion(t *testing.T) {
	p, err := Load("ads-2026-10-01")
	require.NoError(t, err)
	require.Equal(t, "ads-2026-10-01", p.Version)
	require.Len(t, p.ConfigHash, 64)
	require.GreaterOrEqual(t, p.QASampleRate, 0.05)
	require.Equal(t, 3, p.ProtectionSubmissions)
	require.True(t, p.ThresholdFor("CONTENT.SELF_HARM", "US").AutoReject)
	require.False(t, p.ThresholdFor("MISLEADING.CLAIM", "US").AutoReject)
	require.InDelta(t, 0.15, p.ThresholdFor("MISLEADING.CLAIM", "DE").Pass, 1e-9)
	require.NotEmpty(t, p.Keywords)
}

func TestLoadRejectsUnknownOrUnsafeVersions(t *testing.T) {
	for _, version := range []string{"", "../x", "missing"} {
		_, err := Load(version)
		require.Error(t, err, version)
	}
}

func TestParseRejectsWeakenedSafeguards(t *testing.T) {
	base, err := versions.ReadFile("versions/ads-2026-10-01.yaml")
	require.NoError(t, err)
	cases := map[string][2]string{
		"qa below 5%":        {"qaSampleRate: 0.05", "qaSampleRate: 0.01"},
		"protection below 3": {"protectionSubmissions: 3", "protectionSubmissions: 1"},
		"matrix mismatch":    {"industryMatrix: demo-matrix-2026-10-01", "industryMatrix: other"},
		"unknown issue":      {"  - CONTENT.VIOLENCE\n", "  - CONTENT.NOPE\n"},
		"pass above reject":  {"default: {route: 0.80, pass: 0.20, reject: 0.95", "default: {route: 0.80, pass: 0.96, reject: 0.95"},
		"unknown field":      {"routerTopK: 5", "routerTopK: 5\nsurprise: 1"},
		"bad action":         {"action: human", "action: allow"},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			changed := strings.Replace(string(base), edit[0], edit[1], 1)
			require.NotEqual(t, string(base), changed)
			_, err := parse([]byte(changed))
			require.Error(t, err)
		})
	}
}
