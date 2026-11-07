package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

// These types are the catalog format Girvi reads (schema 1). Keep them in step with
// Girvi's reader: a field added here that Girvi does not know is ignored by it, and
// one it requires that is missing here makes the entry invisible.

type Index struct {
	Schema  int     `json:"schema"`
	Name    string  `json:"name"`
	Plugins []Entry `json:"plugins"`
}

type Entry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Author      string    `json:"author"`
	Description string    `json:"description"`
	Homepage    string    `json:"homepage,omitempty"`
	License     string    `json:"license,omitempty"`
	Versions    []Version `json:"versions"`
}

type Version struct {
	Version      string     `json:"version"`
	Protocol     int        `json:"protocol"`
	Released     string     `json:"released,omitempty"`
	Capabilities []string   `json:"capabilities,omitempty"`
	Requires     []string   `json:"requires,omitempty"`
	Artifacts    []Artifact `json:"artifacts"`
}

type Artifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,48}/[a-z0-9][a-z0-9-]{0,48}$`)
	sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	oses          = map[string]bool{"linux": true, "darwin": true, "windows": true}
	arches        = map[string]bool{"amd64": true, "arm64": true}
)

// Parse reads an index and refuses fields it does not know, so a typo in a key
// (for example "sha265") is an error and not a silently missing checksum.
func Parse(data []byte) (*Index, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var idx Index
	if err := dec.Decode(&idx); err != nil {
		return nil, fmt.Errorf("index.json is not valid: %w", err)
	}
	return &idx, nil
}

// Policy decides who may list what.
type Policy struct {
	// GitHubOwnerMustMatchID requires every download to be a GitHub release asset
	// of the account named at the start of the plugin id. It stops one author
	// listing a program under another author's id.
	GitHubOwnerMustMatchID bool
}

// Validate returns every problem it finds, so an author can fix them in one go.
func Validate(idx *Index, p Policy) []string {
	var problems []string
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	if idx.Schema != 1 {
		bad("schema must be 1, not %d", idx.Schema)
	}
	if strings.TrimSpace(idx.Name) == "" {
		bad("the catalog needs a name")
	}
	seen := map[string]bool{}
	for _, e := range idx.Plugins {
		if !idPattern.MatchString(e.ID) {
			bad("%q is not a valid plugin id (owner/name, lowercase letters, digits and dashes)", e.ID)
			continue
		}
		if seen[e.ID] {
			bad("%s is listed twice", e.ID)
		}
		seen[e.ID] = true
		for field, v := range map[string]string{"name": e.Name, "author": e.Author, "description": e.Description} {
			if strings.TrimSpace(v) == "" {
				bad("%s needs a %s", e.ID, field)
			}
		}
		if len(e.Versions) == 0 {
			bad("%s has no versions", e.ID)
		}
		owner := e.ID[:strings.Index(e.ID, "/")]
		versions := map[string]bool{}
		for _, v := range e.Versions {
			where := e.ID + " " + v.Version
			if !semver.IsValid("v"+strings.TrimPrefix(v.Version, "v")) || strings.HasPrefix(v.Version, "v") {
				bad("%s: the version must look like 1.2.3 (no leading v)", where)
			}
			if versions[v.Version] {
				bad("%s is listed twice", where)
			}
			versions[v.Version] = true
			if v.Protocol < 1 {
				bad("%s: the protocol must be 1 or more", where)
			}
			if len(v.Artifacts) == 0 {
				bad("%s: no downloads", where)
			}
			pairs := map[string]bool{}
			for _, a := range v.Artifacts {
				pair := a.OS + "/" + a.Arch
				switch {
				case !oses[a.OS] || !arches[a.Arch]:
					bad("%s: %q is not a supported platform (linux, darwin, windows on amd64, arm64)", where, pair)
				case pairs[pair]:
					bad("%s: %s is listed twice", where, pair)
				}
				pairs[pair] = true
				if !sha256Pattern.MatchString(a.SHA256) {
					bad("%s %s: the sha256 must be 64 lowercase hex characters", where, pair)
				}
				if a.Size <= 0 {
					bad("%s %s: the size is missing", where, pair)
				}
				if msg := checkDownloadURL(a.URL, owner, p); msg != "" {
					bad("%s %s: %s", where, pair, msg)
				}
			}
		}
	}
	return problems
}

func checkDownloadURL(raw, owner string, p Policy) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "the download must be an https address"
	}
	if p.GitHubOwnerMustMatchID {
		allowed := []string{owner}
		if hosts, reserved := reservedOwners[owner]; reserved {
			allowed = hosts
		}
		if !isReleaseAssetOf(u, allowed) {
			return fmt.Sprintf("the download must be a GitHub release asset (github.com/<owner>/<repo>/releases/download/<tag>/<file>) of github.com/%s, the account allowed for the plugin id", strings.Join(allowed, " or github.com/"))
		}
	}
	return ""
}

// isReleaseAssetOf checks the exact shape of a GitHub release download, so the
// account in it is the account the file really comes from. A prefix check is not
// enough: dot segments ("..", "." and their percent-encoded forms) would let a path
// that starts with an allowed account walk into any other repository.
func isReleaseAssetOf(u *url.URL, owners []string) bool {
	if u.Host != "github.com" || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	// <owner>/<repo>/releases/download/<tag, which may hold a slash>/<file>
	if len(segments) < 6 || segments[2] != "releases" || segments[3] != "download" {
		return false
	}
	for _, seg := range segments {
		if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, `%\`) {
			return false
		}
	}
	for _, o := range owners {
		if segments[0] == o {
			return true
		}
	}
	return false
}

// reservedOwners are ids only the named GitHub accounts may publish. "girvi/..."
// is the official namespace, so nobody else can list a plugin as if it were one.
var reservedOwners = map[string][]string{"girvi": {"anant-barlota"}}

// CheckImmutable refuses a change to a version that was already published, which
// would swap a program people may have installed. Adding versions and removing a
// bad one are allowed.
func CheckImmutable(old, cur *Index) []string {
	var problems []string
	have := map[string]Version{}
	for _, e := range cur.Plugins {
		for _, v := range e.Versions {
			have[e.ID+"@"+v.Version] = v
		}
	}
	for _, e := range old.Plugins {
		for _, v := range e.Versions {
			now, ok := have[e.ID+"@"+v.Version]
			if !ok {
				continue
			}
			a, _ := json.Marshal(v)
			b, _ := json.Marshal(now)
			if !bytes.Equal(a, b) {
				problems = append(problems, fmt.Sprintf("%s %s was published before and cannot be changed. Release a new version instead", e.ID, v.Version))
			}
		}
	}
	return problems
}

// Canonical orders the catalog the same way every time, so a change shows up as a
// small diff.
func Canonical(idx *Index) []byte {
	sort.Slice(idx.Plugins, func(i, j int) bool { return idx.Plugins[i].ID < idx.Plugins[j].ID })
	for i := range idx.Plugins {
		vs := idx.Plugins[i].Versions
		sort.Slice(vs, func(a, b int) bool { return semver.Compare("v"+vs[a].Version, "v"+vs[b].Version) < 0 })
		for j := range vs {
			arts := vs[j].Artifacts
			sort.Slice(arts, func(a, b int) bool { return arts[a].OS+arts[a].Arch < arts[b].OS+arts[b].Arch })
		}
	}
	out, _ := json.MarshalIndent(idx, "", "  ")
	return append(out, '\n')
}
