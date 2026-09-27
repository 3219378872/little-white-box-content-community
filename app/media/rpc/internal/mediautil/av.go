package mediautil

import (
	"encoding/binary"
	"io"
	"os"

	"github.com/h2non/filetype"
)

// DetectAV checks the container's track declarations as well as its signature.
// It does not decode, transcode or claim codec/playback compatibility.
func DetectAV(path string, audio bool) (DetectedType, error) {
	f, err := os.Open(path)
	if err != nil {
		return DetectedType{}, err
	}
	defer func() { _ = f.Close() }()
	stat, err := f.Stat()
	if err != nil {
		return DetectedType{}, err
	}
	head := make([]byte, 262)
	n, err := f.ReadAt(head, 0)
	if err != nil && err != io.EOF {
		return DetectedType{}, err
	}
	head = head[:n]
	kind, err := filetype.Match(head)
	if err != nil {
		return DetectedType{}, ErrUnsupportedType
	}
	mime, ext := kind.MIME.Value, kind.Extension
	var hasAudio, hasVideo bool
	switch {
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		hasAudio, hasVideo, err = isoTracks(f, 0, stat.Size(), 0)
		if err != nil {
			return DetectedType{}, ErrUnsupportedType
		}
		if audio {
			mime, ext = "audio/mp4", "m4a"
		} else if string(head[8:12]) == "qt  " {
			mime, ext = "video/quicktime", "mov"
		} else {
			mime, ext = "video/mp4", "mp4"
		}
	case mime == "video/webm" || mime == "video/x-matroska":
		hasAudio, hasVideo, err = ebmlTracks(f, 0, stat.Size(), 0)
		if err != nil {
			return DetectedType{}, ErrUnsupportedType
		}
		// Audio-only WebM is deliberately outside the MP3/WAV/M4A audio contract.
		if audio {
			return DetectedType{}, ErrUnsupportedType
		}
	case mime == "audio/mpeg":
		hasAudio = true
	case mime == "audio/x-wav":
		hasAudio = true
		mime = "audio/wav"
	default:
		return DetectedType{}, ErrUnsupportedType
	}
	if audio && hasAudio && !hasVideo {
		return DetectedType{Kind: KindAudio, MIME: mime, Ext: ext}, nil
	}
	if !audio && hasVideo {
		return DetectedType{Kind: KindVideo, MIME: mime, Ext: ext}, nil
	}
	return DetectedType{}, ErrUnsupportedType
}

func isoTracks(r io.ReaderAt, start, end int64, depth int) (audio, video bool, err error) {
	if depth > 3 {
		return false, false, ErrUnsupportedType
	}
	for pos := start; pos < end; {
		var h [16]byte
		if end-pos < 8 {
			return false, false, ErrUnsupportedType
		}
		if _, err = r.ReadAt(h[:8], pos); err != nil {
			return
		}
		size, header := int64(binary.BigEndian.Uint32(h[:4])), int64(8)
		switch size {
		case 1:
			if _, err = r.ReadAt(h[8:], pos+8); err != nil {
				return
			}
			large := binary.BigEndian.Uint64(h[8:])
			if large > uint64(end-pos) {
				return false, false, ErrUnsupportedType
			}
			size, header = int64(large), 16
		case 0:
			size = end - pos
		}
		if size < header || size > end-pos {
			return false, false, ErrUnsupportedType
		}
		name := string(h[4:8])
		if (depth == 0 && name == "moov") || (depth == 1 && name == "trak") || (depth == 2 && name == "mdia") {
			a, v, e := isoTracks(r, pos+header, pos+size, depth+1)
			if e != nil {
				return false, false, e
			}
			audio, video = audio || a, video || v
		} else if depth == 3 && name == "hdlr" {
			if size-header < 12 {
				return false, false, ErrUnsupportedType
			}
			var handler [4]byte
			if _, err = r.ReadAt(handler[:], pos+header+8); err != nil {
				return
			}
			audio, video = audio || string(handler[:]) == "soun", video || string(handler[:]) == "vide"
		}
		pos += size
	}
	return
}

func ebmlNumber(r io.ReaderAt, pos int64, id bool) (value uint64, length int64, unknown bool, err error) {
	var b [8]byte
	if _, err = r.ReadAt(b[:1], pos); err != nil {
		return
	}
	mask := byte(0x80)
	length = 1
	for mask > 0 && b[0]&mask == 0 {
		mask >>= 1
		length++
	}
	if mask == 0 || (id && length > 4) {
		err = ErrUnsupportedType
		return
	}
	if _, err = r.ReadAt(b[:length], pos); err != nil {
		return
	}
	value = uint64(b[0] & (mask - 1))
	if id {
		value = uint64(b[0])
	}
	for _, x := range b[1:length] {
		value = value<<8 | uint64(x)
	}
	unknown = !id && value == (uint64(1)<<(7*length))-1
	return
}

func ebmlTracks(r io.ReaderAt, start, end int64, depth int) (audio, video bool, err error) {
	if depth > 3 {
		return false, false, ErrUnsupportedType
	}
	for pos := start; pos < end; {
		id, il, _, e := ebmlNumber(r, pos, true)
		if e != nil {
			return false, false, e
		}
		size, sl, unknown, e := ebmlNumber(r, pos+il, false)
		if e != nil {
			return false, false, e
		}
		payload := pos + il + sl
		if payload > end {
			return false, false, ErrUnsupportedType
		}
		if unknown {
			if id != 0x18538067 {
				return false, false, ErrUnsupportedType
			}
			size = uint64(end - payload)
		}
		if size > uint64(end-payload) {
			return false, false, ErrUnsupportedType
		}
		next := payload + int64(size)
		if (depth == 0 && id == 0x18538067) || (depth == 1 && id == 0x1654AE6B) || (depth == 2 && id == 0xAE) {
			a, v, e := ebmlTracks(r, payload, next, depth+1)
			if e != nil {
				return false, false, e
			}
			audio, video = audio || a, video || v
		} else if depth == 3 && id == 0x83 {
			if size == 0 || size > 8 {
				return false, false, ErrUnsupportedType
			}
			var b [8]byte
			if _, err = r.ReadAt(b[8-size:], payload); err != nil {
				return
			}
			t := binary.BigEndian.Uint64(b[:])
			audio, video = audio || t == 2, video || t == 1
		}
		pos = next
	}
	return
}
