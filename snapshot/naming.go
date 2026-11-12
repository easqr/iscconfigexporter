package snapshot

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// FilenameMode selects the value used before .json, under the type directory.
// The empty value preserves the legacy hex convention.
type FilenameMode string

const (
	HexFilename  FilenameMode = "hex"
	IDFilename   FilenameMode = "id"
	NameFilename FilenameMode = "name"
)

func (m FilenameMode) Valid() bool {
	return m == "" || m == HexFilename || m == IDFilename || m == NameFilename
}

// Effective resolves an omitted mode to hex.
func (m FilenameMode) Effective() FilenameMode {
	if m == "" {
		return HexFilename
	}
	return m
}

// Name returns self.name, the export envelope's display name. Missing or non-string
// names become empty; name mode reports these rather than inventing a fallback.
func Name(env map[string]any) string {
	self, _ := env["self"].(map[string]any)
	name, _ := self["name"].(string)
	return name
}

// FilenameIssue identifies one portability problem without including object payloads.
type FilenameIssue struct {
	Code      string   `json:"code"`
	Type      string   `json:"type"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Platforms []string `json:"platforms"`
	Message   string   `json:"message"`
}

// FilenameCandidate shows the actual proposed path, including any escapes.
type FilenameCandidate struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Escaped    bool   `json:"escaped"`
	IDAppended bool   `json:"id_appended"`
}

// FilenameOptions controls filenames. AppendID forces IDs on all name-mode files;
// otherwise inventory planning adds IDs only to collision groups.
type FilenameOptions struct {
	Mode     FilenameMode
	Escape   bool
	AppendID bool
	// IdentityProfileName supplies the parent name for low-level lifecycle-state
	// path construction. Inventory callers resolve it from identityProfileRef.
	IdentityProfileName string
}

// FilenameReport describes filename components and inventory collisions across
// Linux, macOS and Windows; it does not certify destination full-path limits.
type FilenameReport struct {
	Escape   bool                `json:"escape"`
	AppendID bool                `json:"append_id"`
	Files    []FilenameCandidate `json:"files"`
	Mode     FilenameMode        `json:"mode"`
	Safe     bool                `json:"safe"`
	Issues   []FilenameIssue     `json:"issues"`
}

// FilenameError is returned before publication when a naming mode has issues.
type FilenameError struct{ Report FilenameReport }

func (e *FilenameError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s filename mode has %d compatibility issue(s)", e.Report.Mode, len(e.Report.Issues))
	for _, issue := range e.Report.Issues {
		fmt.Fprintf(&b, "\n  %s id=%q name=%q path=%q [%s]: %s", issue.Type, issue.ID, issue.Name, issue.Path, strings.Join(issue.Platforms, ", "), issue.Message)
	}
	return b.String()
}

// CollisionKey conservatively compares paths without case or canonical Unicode
// normalization distinctions. Full Unicode folding can reject more pairs than a
// particular filesystem; this avoids relying on host-specific comparison rules.
func CollisionKey(p string) string { return norm.NFD.String(cases.Fold().String(norm.NFD.String(p))) }

func filenameProblems(stem string) []FilenameIssue {
	var out []FilenameIssue
	add := func(code, message string, platforms ...string) {
		out = append(out, FilenameIssue{Code: code, Message: message, Platforms: platforms})
	}
	if stem == "" {
		add("missing_value", "filename value is missing or empty", "linux", "macos", "windows")
	}
	if !utf8.ValidString(stem) {
		add("invalid_unicode", "filename value is not valid UTF-8", "linux", "macos", "windows")
	}
	if strings.ContainsRune(stem, '/') {
		add("path_separator", "forward slash is a path separator", "linux", "macos", "windows")
	}
	if strings.ContainsRune(stem, '\\') {
		add("path_separator", "backslash is a Windows path separator", "windows")
	}
	if strings.ContainsRune(stem, 0) {
		add("nul", "NUL cannot be filename content", "linux", "macos", "windows")
	}
	if strings.IndexFunc(stem, func(r rune) bool { return r != 0 && unicode.IsControl(r) }) >= 0 {
		add("control_character", "control characters are rejected for interoperability", "linux", "macos", "windows")
	}
	if strings.ContainsAny(stem, `<>:"|?*`) {
		add("reserved_character", "filename contains Windows-reserved punctuation (colon also affects Finder)", "windows")
	}
	if reservedDevice(stem) {
		add("reserved_device", "Windows device name remains reserved with a .json extension", "windows")
	}
	if strings.HasPrefix(stem, " ") {
		add("leading_space", "leading ASCII spaces are not reliably preserved by Windows tools", "windows")
	}
	// The suffix means a trailing space or period in the value is internal to the
	// completed filename and is not a trailing Windows filename character.
	file := stem + ".json"
	if len(file) > 255 {
		add("filename_length", "filename including .json exceeds the conservative 255 UTF-8 byte component limit", "linux", "macos")
	}
	if len(utf16.Encode([]rune(file))) > 255 {
		add("filename_length", "filename including .json exceeds 255 UTF-16 code units", "windows")
	}
	return out
}

func reservedDevice(stem string) bool {
	base := strings.ToUpper(strings.TrimRight(strings.SplitN(stem, ".", 2)[0], " "))
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
		return true
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		digit := base[3:]
		return utf8.RuneCountInString(digit) == 1 && strings.ContainsAny(digit, "123456789¹²³")
	}
	return false
}

// EscapeFilename percent-encodes unsafe characters as uppercase UTF-8 byte
// escapes. Literal percent signs are always escaped, making the encoding
// reversible and distinguishing a colon from an original literal "%3A".
// It also escapes leading ASCII spaces and the first byte of device names.
// Input must be valid UTF-8. It does not shorten values or resolve case/Unicode collisions.
func EscapeFilename(stem string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	leading := true
	for _, r := range stem {
		if r == '%' || strings.ContainsRune(`<>:"/\|?*`, r) || unicode.IsControl(r) || (leading && r == ' ') {
			for _, v := range []byte(string(r)) {
				b.WriteByte('%')
				b.WriteByte(hex[v>>4])
				b.WriteByte(hex[v&15])
			}
		} else {
			b.WriteRune(r)
		}
		if r != ' ' {
			leading = false
		}
	}
	result := b.String()
	if reservedDevice(result) {
		// Device prefixes begin with ASCII, so escaping the first byte is sufficient.
		result = "%" + string([]byte{hex[result[0]>>4], hex[result[0]&15]}) + result[1:]
	}
	return result
}

func filenamePlan(typ, id, name string, opts FilenameOptions, legacyName, profilePrefix bool) (string, bool, []FilenameIssue) {
	stem := id
	if opts.Mode.Effective() == NameFilename {
		stem = name
		if profilePrefix && typ == "LIFECYCLE_STATE" {
			stem = opts.IdentityProfileName + "--" + name
		}
		if !legacyName {
			stem += "--" + id
		}
	}
	raw := stem
	if opts.Escape && opts.Mode.Effective() != HexFilename && utf8.ValidString(stem) {
		stem = EscapeFilename(stem)
	}
	p := typ + "/" + stem + ".json"
	problems := filenameProblems(stem)
	if opts.Mode.Effective() == NameFilename && name == "" && stem != "" {
		problems = append(problems, FilenameIssue{Code: "missing_value", Message: "self.name is missing, non-string or empty", Platforms: []string{"linux", "macos", "windows"}})
	}
	if opts.Mode.Effective() == NameFilename && profilePrefix && typ == "LIFECYCLE_STATE" && opts.IdentityProfileName == "" {
		problems = append(problems, FilenameIssue{Code: "missing_identity_profile", Message: "name-mode lifecycle filenames require object.identityProfileRef with type IDENTITY_PROFILE and nonempty id/name", Platforms: []string{"linux", "macos", "windows"}})
	}
	if opts.Mode.Effective() == HexFilename {
		var err error
		p, err = ObjectPath(typ, id)
		problems = nil
		if err != nil {
			problems = []FilenameIssue{{Code: "invalid_identity", Message: "unsafe object identity", Platforms: []string{"linux", "macos", "windows"}}}
		}
	} else if !opts.Mode.Valid() {
		problems = []FilenameIssue{{Code: "unknown_mode", Message: "unknown filename mode", Platforms: []string{"linux", "macos", "windows"}}}
	}
	return p, stem != raw && opts.Mode.Effective() != HexFilename, problems
}

// ObjectPathWithMode returns a strict portable path. Name mode uses
// <self.name>.json unless AppendID is enabled. Lifecycle states also require IdentityProfileName;
// use ObjectPathForObject to resolve it from the envelope.
func ObjectPathWithMode(typ, id, name string, mode FilenameMode) (string, error) {
	return ObjectPathWithOptions(typ, id, name, FilenameOptions{Mode: mode})
}

// ObjectPathWithOptions retains ObjectPath's identity validation in all modes.
// Escaping never changes the stored envelope or silently resolves collisions.
// Name-mode lifecycle states require opts.IdentityProfileName.
func ObjectPathWithOptions(typ, id, name string, opts FilenameOptions) (string, error) {
	return objectPathWithFormat(typ, id, name, opts, !opts.AppendID, true)
}

func objectPathWithFormat(typ, id, name string, opts FilenameOptions, legacyName, profilePrefix bool) (string, error) {
	if _, err := ObjectPath(typ, id); err != nil {
		return "", err
	}
	p, _, problems := filenamePlan(typ, id, name, opts, legacyName, profilePrefix)
	if len(problems) != 0 {
		return "", errors.New("nonportable filename value")
	}
	return p, nil
}

// IdentityProfileName resolves a lifecycle state's explicit exported parent reference.
// It returns empty for absent, malformed or wrongly typed references.
func IdentityProfileName(env map[string]any) string {
	payload, _ := env["object"].(map[string]any)
	ref, _ := payload["identityProfileRef"].(map[string]any)
	id, _ := ref["id"].(string)
	name, _ := ref["name"].(string)
	if ref["type"] != "IDENTITY_PROFILE" || id == "" {
		return ""
	}
	return name
}

func optionsForObject(obj Object, opts FilenameOptions) FilenameOptions {
	if obj.Type == "LIFECYCLE_STATE" {
		opts.IdentityProfileName = IdentityProfileName(obj.Envelope)
	}
	return opts
}

// ObjectPathForObject constructs a new path, resolving a lifecycle state's parent
// from its envelope instead of requiring separately fetched identity profiles.
func ObjectPathForObject(obj Object, opts FilenameOptions) (string, error) {
	return ObjectPathWithOptions(obj.Type, obj.ID, Name(obj.Envelope), optionsForObject(obj, opts))
}

func pathForManifest(typ, id string, env map[string]any, m Manifest) (string, error) {
	obj := Object{Type: typ, ID: id, Envelope: env}
	opts := optionsForObject(obj, FilenameOptions{Mode: m.FilenameMode, Escape: m.FilenameEscape})
	return objectPathWithFormat(typ, id, Name(env), opts, m.Schema == 2 || ((m.Schema == 0 || m.Schema >= 5) && !m.FilenameAppendID), m.Schema == 0 || m.Schema >= 4)
}

// CheckFilenames assesses strict filenames and resolves duplicate names with IDs.
func CheckFilenames(objects []Object, mode FilenameMode) FilenameReport {
	return CheckFilenamesWithOptions(objects, FilenameOptions{Mode: mode})
}

// CheckFilenamesWithOptions checks a validated inventory and reports every planned
// path and unresolved issue in deterministic order, after optional escaping.
func CheckFilenamesWithOptions(objects []Object, opts FilenameOptions) FilenameReport {
	report := FilenameReport{AppendID: opts.AppendID && opts.Mode.Effective() == NameFilename, Mode: opts.Mode.Effective(), Escape: opts.Escape && opts.Mode.Effective() != HexFilename, Safe: true, Files: []FilenameCandidate{}, Issues: []FilenameIssue{}}
	// Resolve all members of each collision group, then repeat to catch a
	// generated suffix colliding with another object's original name.
	appended := make([]bool, len(objects))
	for i := range appended {
		appended[i] = opts.AppendID && opts.Mode.Effective() == NameFilename
	}
	for opts.Mode.Effective() == NameFilename {
		groups := map[string][]int{}
		for i, obj := range objects {
			p, _, _ := filenamePlan(obj.Type, obj.ID, Name(obj.Envelope), optionsForObject(obj, opts), !appended[i], true)
			groups[CollisionKey(p)] = append(groups[CollisionKey(p)], i)
		}
		changed := false
		for _, group := range groups {
			if len(group) > 1 {
				for _, i := range group {
					if !appended[i] {
						appended[i] = true
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}
	groups := map[string][]FilenameCandidate{}
	for i, obj := range objects {
		name := Name(obj.Envelope)
		p, escaped, problems := filenamePlan(obj.Type, obj.ID, name, optionsForObject(obj, opts), !appended[i], true)
		file := FilenameCandidate{Type: obj.Type, ID: obj.ID, Name: name, Path: p, Escaped: escaped, IDAppended: appended[i]}
		report.Files = append(report.Files, file)
		for _, issue := range problems {
			issue.Type, issue.ID, issue.Name, issue.Path = obj.Type, obj.ID, name, p
			report.Issues = append(report.Issues, issue)
		}
		key := CollisionKey(p)
		groups[key] = append(groups[key], file)
	}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		ids := make([]string, len(group))
		for i, file := range group {
			ids[i] = file.ID
		}
		sort.Strings(ids)
		for _, file := range group {
			report.Issues = append(report.Issues, FilenameIssue{Code: "collision", Type: file.Type, ID: file.ID, Name: file.Name, Path: file.Path, Platforms: []string{"linux", "macos", "windows"}, Message: fmt.Sprintf("same type has duplicate or case/Unicode-equivalent filenames for IDs %q (conservative comparison)", ids)})
		}
	}
	sort.Slice(report.Files, func(i, j int) bool {
		a, b := report.Files[i], report.Files[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.ID < b.ID
	})
	sort.Slice(report.Issues, func(i, j int) bool {
		a, b := report.Issues[i], report.Issues[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
	report.Safe = len(report.Issues) == 0
	return report
}

// ManagedPathWithMode checks path syntax before any filesystem access. Readers
// separately check the exact path against the envelope identity and display name.
func ManagedPathWithMode(p string, mode FilenameMode) bool {
	if mode.Effective() == HexFilename {
		return ManagedPath(p)
	}
	if !mode.Valid() {
		return false
	}
	parts := strings.Split(p, "/")
	return len(parts) == 2 && portable(parts[0]) && strings.HasSuffix(parts[1], ".json") && len(filenameProblems(strings.TrimSuffix(parts[1], ".json"))) == 0
}

func validManifestNaming(m Manifest) bool {
	if m.Schema < 5 && m.FilenameAppendID {
		return false
	}
	switch m.Schema {
	case 1:
		return m.FilenameMode == "" && !m.FilenameEscape
	case 2:
		return (m.FilenameMode == IDFilename || m.FilenameMode == NameFilename) && !m.FilenameEscape
	case 3:
		return m.FilenameMode == NameFilename || (m.FilenameMode == IDFilename && m.FilenameEscape)
	case 4, 5:
		return m.FilenameMode == NameFilename
	}
	return false
}
