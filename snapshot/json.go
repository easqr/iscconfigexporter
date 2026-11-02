package snapshot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DecodeJSON rejects duplicate keys instead of silently losing configuration data.
func DecodeJSON(raw []byte) (any, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("invalid UTF-8 JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	v, e := value(d)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return v, nil
}
func value(d *json.Decoder) (any, error) {
	t, e := d.Token()
	if e != nil {
		return nil, e
	}
	switch t {
	case json.Delim('{'):
		m := map[string]any{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return nil, e
			}
			s, ok := k.(string)
			if !ok {
				return nil, errors.New("invalid key")
			}
			if _, ok = m[s]; ok {
				return nil, errors.New("duplicate JSON key")
			}
			v, e := value(d)
			if e != nil {
				return nil, e
			}
			m[s] = v
		}
		_, e = d.Token()
		return m, e
	case json.Delim('['):
		a := []any{}
		for d.More() {
			v, e := value(d)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		_, e = d.Token()
		return a, e
	}
	return t, nil
}
func Checksum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func portable(s string) bool {
	if !validComponent(s) {
		return false
	}
	u := strings.ToUpper(s)
	switch u {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	}
	return true
}
func ObjectPath(typ, id string) (string, error) {
	if !portable(typ) || id == "" || len(id) > 100 || id == "." || id == ".." || strings.ContainsAny(id, "/\\") || strings.IndexFunc(id, unicode.IsControl) >= 0 {
		return "", errors.New("unsafe object identity")
	}
	return typ + "/id-" + hex.EncodeToString([]byte(id)) + ".json", nil
}

var component = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,99}$`)

func validComponent(s string) bool { return component.MatchString(s) }
