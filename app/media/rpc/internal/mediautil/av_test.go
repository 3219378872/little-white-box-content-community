package mediautil

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func isoFixture(track string) []byte {
	box := func(name string, body []byte) []byte {
		b := make([]byte, 8+len(body))
		binary.BigEndian.PutUint32(b, uint32(len(b)))
		copy(b[4:], name)
		copy(b[8:], body)
		return b
	}
	handler := append(make([]byte, 8), []byte(track)...)
	return append(box("ftyp", []byte("isom\x00\x00\x00\x00isom")), box("moov", box("trak", box("mdia", box("hdlr", handler))))...)
}
func TestDetectAVTrackTypes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		data      []byte
		audio, ok bool
		mime      string
	}{
		{"MP4 video", isoFixture("vide"), false, true, "video/mp4"},
		{"M4A generic brand", isoFixture("soun"), true, true, "audio/mp4"},
		{"audio cannot masquerade as video", isoFixture("soun"), false, false, ""},
		{"video cannot masquerade as audio", isoFixture("vide"), true, false, ""},
		{"truncated container", isoFixture("soun")[:35], true, false, ""},
		{"header only", mp4Header, false, false, ""},
		{"unsupported", pdfHeader, true, false, ""},
		{"mp3", []byte("ID3\x04\x00\x00\x00\x00\x00\x00"), true, true, "audio/mpeg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, e := DetectAV(writeFile(t, tc.data), tc.audio)
			if !tc.ok {
				require.Error(t, e)
				return
			}
			require.NoError(t, e)
			require.Equal(t, tc.mime, d.MIME)
		})
	}
}
func TestEBMLTrackTypes(t *testing.T) {
	for _, typ := range []byte{1, 2} {
		// Segment > Tracks > TrackEntry > TrackType.
		data := []byte{0x18, 0x53, 0x80, 0x67, 0x8a, 0x16, 0x54, 0xae, 0x6b, 0x85, 0xae, 0x83, 0x83, 0x81, typ}
		f, err := os.Open(writeFile(t, data))
		require.NoError(t, err)
		a, v, err := ebmlTracks(f, 0, int64(len(data)), 0)
		require.NoError(t, err)
		require.Equal(t, typ == 1, v)
		require.Equal(t, typ == 2, a)
		f.Close()
	}
}
