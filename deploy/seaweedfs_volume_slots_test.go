package deploy

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// SeaweedFS grows 7 volumes per collection with replication 000. The filer
// default collection, xbh-media and xbh-ad-private all need room on a fresh
// data volume, so the local stack must not rely on the default cap of 8.
func TestMiddlewareSeaweedFSHasVolumeSlotsForEveryBucket(t *testing.T) {
	body, err := os.ReadFile("docker-compose.middleware.yml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	start := strings.Index(content, "\n  seaweedfs:\n")
	if start < 0 {
		t.Fatal("seaweedfs service block not found")
	}
	block := content[start+1:]
	if end := regexp.MustCompile(`\n  [a-z][a-z0-9-]*:\n`).FindStringIndex(block); end != nil {
		block = block[:end[0]]
	}
	match := regexp.MustCompile(`-volume\.max=(\d+)`).FindStringSubmatch(block)
	if match == nil {
		t.Fatal("seaweedfs command must set -volume.max explicitly")
	}
	slots, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatal(err)
	}
	const collections, growth = 3, 7
	if slots < collections*growth {
		t.Fatalf("seaweedfs -volume.max=%d cannot grow %d collections of %d volumes", slots, collections, growth)
	}
}
