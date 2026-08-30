package prompt

import "strings"

type scrubFence struct {
	open  string
	close string
}

var scrubFences = []scrubFence{
	{open: MemoryOpenTag, close: MemoryCloseTag},
	{open: SummaryOpenTag, close: SummaryCloseTag},
}

var scrubNeedles = []string{UntrustedMemoryNotice, UntrustedSummaryNotice}

// SanitizeOutput removes provider-visible context envelopes from a completed
// answer. The streaming path uses the same rules through StreamingScrubber.
func SanitizeOutput(text string) string {
	for _, fence := range scrubFences {
		text = stripFenceBlocks(text, fence)
		text = replaceFold(text, fence.open, "")
		text = replaceFold(text, fence.close, "")
	}
	for _, needle := range scrubNeedles {
		text = replaceFold(text, needle, "")
	}
	return text
}

// StreamingScrubber holds partial tags and platform notes across provider
// chunks. An unterminated context span is discarded at end of stream.
type StreamingScrubber struct {
	pending string
	inSpan  bool
	close   string
}

func (s *StreamingScrubber) Feed(text string) string {
	if text == "" {
		return ""
	}
	buf := s.pending + text
	s.pending = ""
	var out strings.Builder
	for buf != "" {
		if s.inSpan {
			idx := indexFold(buf, s.close)
			if idx < 0 {
				held := longestPartialSuffix(buf, []string{s.close})
				if held > 0 {
					s.pending = buf[len(buf)-held:]
				}
				return out.String()
			}
			buf = buf[idx+len(s.close):]
			s.inSpan = false
			s.close = ""
			continue
		}

		idx, fence := earliestFence(buf)
		if idx >= 0 {
			out.WriteString(stripNeedles(buf[:idx]))
			buf = buf[idx+len(fence.open):]
			s.inSpan = true
			s.close = fence.close
			continue
		}

		buf = stripNeedles(buf)
		candidates := make([]string, 0, len(scrubFences)+len(scrubNeedles))
		for _, item := range scrubFences {
			candidates = append(candidates, item.open)
		}
		candidates = append(candidates, scrubNeedles...)
		held := longestPartialSuffix(buf, candidates)
		if held > 0 {
			out.WriteString(buf[:len(buf)-held])
			s.pending = buf[len(buf)-held:]
		} else {
			out.WriteString(buf)
		}
		return out.String()
	}
	return out.String()
}

func (s *StreamingScrubber) Flush() string {
	if s.inSpan {
		s.pending = ""
		s.inSpan = false
		s.close = ""
		return ""
	}
	out := SanitizeOutput(s.pending)
	s.pending = ""
	return out
}

func stripFenceBlocks(text string, fence scrubFence) string {
	for {
		start := indexFold(text, fence.open)
		if start < 0 {
			return text
		}
		rest := text[start+len(fence.open):]
		end := indexFold(rest, fence.close)
		if end < 0 {
			return text[:start]
		}
		text = text[:start] + rest[end+len(fence.close):]
	}
}

func stripNeedles(text string) string {
	for _, needle := range scrubNeedles {
		text = replaceFold(text, needle, "")
	}
	return text
}

func earliestFence(text string) (int, scrubFence) {
	best := -1
	var found scrubFence
	for _, fence := range scrubFences {
		idx := indexFold(text, fence.open)
		if idx >= 0 && (best < 0 || idx < best) {
			best = idx
			found = fence
		}
	}
	return best, found
}

func longestPartialSuffix(text string, candidates []string) int {
	lower := strings.ToLower(text)
	best := 0
	for _, candidate := range candidates {
		want := strings.ToLower(candidate)
		limit := minInt(len(lower), len(want)-1)
		for size := limit; size > best; size-- {
			if strings.HasPrefix(want, lower[len(lower)-size:]) {
				best = size
				break
			}
		}
	}
	return best
}

func indexFold(text, needle string) int {
	return strings.Index(strings.ToLower(text), strings.ToLower(needle))
}

func replaceFold(text, old, replacement string) string {
	for {
		idx := indexFold(text, old)
		if idx < 0 {
			return text
		}
		text = text[:idx] + replacement + text[idx+len(old):]
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
