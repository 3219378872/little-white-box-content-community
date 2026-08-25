package deploy

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var imageRefPattern = regexp.MustCompile(`(?m)^\s*-?\s*image:\s*(\S+)\s*$`)

// imageIsPinned reports whether ref carries an explicit tag or a digest.
// Bare names (no colon after the last path segment) and :latest are moving
// targets: an upstream push can silently change local behavior on the next
// pull. See REL-022 for the loki incident that motivates this gate.
func imageIsPinned(ref string) bool {
	if strings.Contains(ref, "@sha256:") {
		return true
	}
	tag := ref
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		tag = ref[i+1:]
	}
	if !strings.Contains(tag, ":") {
		return false
	}
	return tag[strings.LastIndex(tag, ":")+1:] != "latest"
}

func TestComposeImagesArePinned(t *testing.T) {
	for _, compose := range []string{
		"docker-compose.middleware.yml",
		"docker-compose.production.yml",
	} {
		t.Run(compose, func(t *testing.T) {
			data, err := os.ReadFile(compose)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range imageRefPattern.FindAllStringSubmatch(string(data), -1) {
				ref := m[1]
				if strings.Contains(ref, "${") {
					continue // parameterized app images are pinned by CI, not by text
				}
				if !imageIsPinned(ref) {
					t.Errorf("%s: image %q is not pinned to a version or digest", compose, ref)
				}
			}
		})
	}
}
