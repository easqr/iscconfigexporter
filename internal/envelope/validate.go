// Package envelope validates the object metadata shared by exporters and readers.
package envelope

import (
	"encoding/json"
	"errors"
)

// Validate checks a decoded envelope and returns its type and ID. It does not
// mutate the envelope or validate filesystem paths or selected type scope.
func Validate(env map[string]any) (typ, id string, err error) {
	ver, ok := env["version"].(json.Number)
	if !ok || ver.String() != "1" {
		return "", "", errors.New("unsupported envelope version; expected version 1")
	}
	self, ok := env["self"].(map[string]any)
	if !ok {
		return "", "", errors.New("missing self metadata")
	}
	typ, _ = self["type"].(string)
	id, _ = self["id"].(string)
	payload, ok := env["object"].(map[string]any)
	if !ok {
		return "", "", errors.New("missing object payload")
	}
	if pid, ok := payload["id"]; ok && pid != id {
		return "", "", errors.New("conflicting payload identity; object.id must match self.id")
	}
	return typ, id, nil
}
