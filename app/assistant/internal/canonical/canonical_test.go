package canonical

import "testing"

func TestDigestIgnoresKeyOrder(t *testing.T) {
	a, err := DigestArgs(`{"b":1,"a":{"y":2,"x":1}}`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := DigestArgs(`{"a":{"x":1,"y":2},"b":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("digests differ: %s vs %s", a, b)
	}
	c, err := DigestArgs(`{"a":{"x":1,"y":2},"b":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Fatal("different args produced the same digest")
	}
}
