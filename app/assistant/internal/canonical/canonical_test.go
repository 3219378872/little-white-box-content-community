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

func TestUnwrapArgsJSONPeelsStringWrappedObject(t *testing.T) {
	got := UnwrapArgsJSON(`{"keyword":"猫粮","page_size":5}`)
	if got != `{"keyword":"猫粮","page_size":5}` {
		t.Fatalf("object: %s", got)
	}
	quoted := `"{\"keyword\":\"猫粮\",\"page_size\":5}"`
	got = UnwrapArgsJSON(quoted)
	if got != `{"keyword":"猫粮","page_size":5}` {
		t.Fatalf("quoted: %s", got)
	}
	double := `"\"{\\\"keyword\\\":\\\"猫粮\\\"}\""`
	got = UnwrapArgsJSON(double)
	if got != `{"keyword":"猫粮"}` {
		t.Fatalf("double quoted: %s", got)
	}
	a, err := DigestArgs(quoted)
	if err != nil {
		t.Fatal(err)
	}
	b, err := DigestArgs(`{"page_size":5,"keyword":"猫粮"}`)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("quoted digest %s != object digest %s", a, b)
	}
}
