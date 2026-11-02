package exporter

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

const role = `{"version":1,"self":{"type":"ROLE","id":"a:b","name":"Role"},"object":{"large":9007199254740993,"array":[2,1]},"unknown":{"signed":true},"jwsSignature":"abc"}`

func download(items string) Download {
	return Download{JobID: "job", Types: []string{"ROLE", "SOURCE"}, Raw: json.RawMessage(`{"tenant":"example","objects":[` + items + `]}`)}
}
func TestOrganize(t *testing.T) {
	a, e := Organize("example", download(role), time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	if a.Manifest.Counts["ROLE"] != 1 || checksum(a.Artifacts[0].Data) != a.Artifacts[0].SHA256 {
		t.Fatal(a)
	}
	s := string(a.Artifacts[0].Data)
	for _, sub := range []string{"9007199254740993", "unknown", "jwsSignature"} {
		if !strings.Contains(s, sub) {
			t.Fatal(s)
		}
	}
	other := strings.Replace(role, "a:b", "other", 1)
	a, e = Organize("example", download(role+","+other), time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	b, e := Organize("example", download(other+","+role), time.Time{})
	if e != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("not deterministic", e)
	}
}
func TestInvalidObjects(t *testing.T) {
	for _, s := range []string{role + "," + role, strings.Replace(role, "a:b", "../bad", 1), strings.Replace(role, "a:b", "", 1), strings.Replace(role, `"version":1`, `"version":null`, 1), strings.Replace(role, `"object":{`, `"object":null,"object":{`, 1), `{}`, strings.Replace(role, `"ROLE"`, `"UNKNOWN"`, 1)} {
		if _, e := Organize("example", download(s), time.Time{}); e == nil {
			t.Fatalf("accepted invalid input %s", s)
		}
	}
	d := download(role)
	d.Raw = []byte(`{"tenant":"other","objects":[]}`)
	if _, e := Organize("example", d, time.Time{}); e == nil {
		t.Fatal("tenant mismatch")
	}
}
func TestPortablePaths(t *testing.T) {
	for _, s := range []string{"CON", "../x", "x/y", "a:b", "a ", ""} {
		if portable(s) {
			t.Fatal(s)
		}
	}
	a, _ := objectPath("ROLE", "A")
	b, _ := objectPath("ROLE", "a")
	if strings.EqualFold(a, b) {
		t.Fatal("case collision")
	}
}
