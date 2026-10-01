package assets

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var pngHeader = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R'}

func TestInspectChecksTypeAndSize(t *testing.T) {
	got, err := Inspect("creative", pngHeader, false)
	require.NoError(t, err)
	require.Equal(t, "image/png", got.MimeType)
	require.Len(t, got.SHA256, 64)

	pdf := []byte("%PDF-1.7\n%...")
	_, err = Inspect("creative", pdf, false)
	require.ErrorIs(t, err, ErrTypeNotAllowed)
	doc, err := Inspect("document", pdf, true)
	require.NoError(t, err)
	require.Equal(t, "application/pdf", doc.MimeType)

	_, err = Inspect("creative", []byte("<svg onload=alert(1)>"), false)
	require.ErrorIs(t, err, ErrTypeNotAllowed)
	_, err = Inspect("creative", nil, false)
	require.ErrorIs(t, err, ErrEmpty)
	_, err = Inspect("creative", append(pngHeader, bytes.Repeat([]byte{0}, MaxAssetBytes)...), false)
	require.ErrorIs(t, err, ErrTooLarge)
}

func TestKeys(t *testing.T) {
	require.Equal(t, "assets/42", PrivateKey(42))
	require.Equal(t, "ads/abcd", PublicKey("ABCD"))
	require.Equal(t, "http://x/xbh-media/ads/ab", PublicURLFor("http://x/xbh-media/", "ab"))
}
