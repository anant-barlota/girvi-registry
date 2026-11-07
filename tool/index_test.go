package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func good() *Index {
	art := func(os, arch string) Artifact {
		return Artifact{OS: os, Arch: arch, URL: "https://github.com/acme/hook/releases/download/v1.0.0/girvi-plugin-hook_1.0.0_" + os + "_" + arch,
			SHA256: strings.Repeat("a", 64), Size: 10}
	}
	return &Index{Schema: 1, Name: "Test", Plugins: []Entry{{
		ID: "acme/hook", Name: "Hook", Author: "Acme", Description: "does a thing",
		Versions: []Version{{Version: "1.0.0", Protocol: 3, Artifacts: []Artifact{art("linux", "amd64"), art("windows", "amd64")}}},
	}}}
}

func has(problems []string, part string) bool {
	for _, p := range problems {
		if strings.Contains(p, part) {
			return true
		}
	}
	return false
}

func TestAGoodIndexHasNoProblems(t *testing.T) {
	if p := Validate(good(), Policy{GitHubOwnerMustMatchID: true}); len(p) != 0 {
		t.Errorf("problems: %v", p)
	}
}

func TestEveryProblemIsReportedAtOnce(t *testing.T) {
	idx := good()
	idx.Schema = 2
	idx.Plugins[0].ID = "Acme/Hook"
	idx.Plugins = append(idx.Plugins, Entry{ID: "acme/two", Versions: []Version{{Version: "v1.0", Protocol: 0, Artifacts: []Artifact{
		{OS: "plan9", Arch: "amd64", URL: "http://x/y", SHA256: "short"},
	}}}})
	p := Validate(idx, Policy{})
	for _, want := range []string{"schema must be 1", "not a valid plugin id", "needs a name", "needs a author", "needs a description", "no leading v", "protocol must be 1", "not a supported platform", "64 lowercase hex", "size is missing", "https"} {
		if !has(p, want) {
			t.Errorf("missing a problem mentioning %q in %v", want, p)
		}
	}
}

func TestAnAuthorCannotListAProgramUnderAnotherAuthorsId(t *testing.T) {
	idx := good()
	idx.Plugins[0].ID = "someone-else/hook"
	if p := Validate(idx, Policy{GitHubOwnerMustMatchID: true}); !has(p, "account allowed for the plugin id") {
		t.Errorf("a download from another account must be refused, got %v", p)
	}
	if p := Validate(idx, Policy{}); has(p, "account allowed for the plugin id") {
		t.Error("the check must be switchable off by the maintainers")
	}
	idx = good()
	idx.Plugins[0].Versions[0].Artifacts[0].URL = "https://evil.example/acme/hook/releases/download/v1/x"
	if p := Validate(idx, Policy{GitHubOwnerMustMatchID: true}); !has(p, "GitHub release asset") {
		t.Errorf("a download from another host must be refused, got %v", p)
	}
}

func TestDuplicatesAreRefused(t *testing.T) {
	idx := good()
	idx.Plugins = append(idx.Plugins, idx.Plugins[0])
	idx.Plugins[0].Versions = append(idx.Plugins[0].Versions, idx.Plugins[0].Versions[0])
	idx.Plugins[0].Versions[0].Artifacts = append(idx.Plugins[0].Versions[0].Artifacts, idx.Plugins[0].Versions[0].Artifacts[0])
	p := Validate(idx, Policy{})
	for _, want := range []string{"listed twice"} {
		if !has(p, want) {
			t.Errorf("want %q in %v", want, p)
		}
	}
}

func TestAPublishedVersionCannotBeChangedButCanBeAddedOrRemoved(t *testing.T) {
	old := good()
	cur := good()
	if p := CheckImmutable(old, cur); len(p) != 0 {
		t.Errorf("unchanged: %v", p)
	}
	cur.Plugins[0].Versions[0].Artifacts[0].SHA256 = strings.Repeat("b", 64)
	if p := CheckImmutable(old, cur); !has(p, "cannot be changed") {
		t.Errorf("a swapped program must be refused, got %v", p)
	}
	added := good()
	added.Plugins[0].Versions = append(added.Plugins[0].Versions, Version{Version: "1.1.0", Protocol: 3})
	if p := CheckImmutable(old, added); len(p) != 0 {
		t.Errorf("adding a version is allowed: %v", p)
	}
	removed := good()
	removed.Plugins[0].Versions = nil
	if p := CheckImmutable(old, removed); len(p) != 0 {
		t.Errorf("removing a bad version is allowed: %v", p)
	}
}

func TestAMisspelledKeyIsAnErrorNotAMissingChecksum(t *testing.T) {
	_, err := Parse([]byte(`{"schema":1,"name":"x","plugins":[{"id":"a/b","name":"B","versions":[{"version":"1.0.0","artifacts":[{"os":"linux","arch":"amd64","url":"u","sha265":"x"}]}]}]}`))
	if err == nil || !strings.Contains(err.Error(), "sha265") {
		t.Errorf("got %v", err)
	}
}

func TestTheCatalogIsWrittenInAStableOrder(t *testing.T) {
	idx := good()
	idx.Plugins = append([]Entry{{ID: "zed/last", Versions: []Version{{Version: "1.10.0"}, {Version: "1.2.0"}}}}, idx.Plugins...)
	out := string(Canonical(idx))
	if strings.Index(out, "acme/hook") > strings.Index(out, "zed/last") {
		t.Error("plugins must be sorted by id")
	}
	if strings.Index(out, `"1.2.0"`) > strings.Index(out, `"1.10.0"`) {
		t.Error("versions must be sorted as versions, not as text")
	}
	if string(Canonical(idx)) != out {
		t.Error("the same catalog must always produce the same bytes")
	}
}

func TestDownloadsAreCheckedAgainstTheirChecksumAndSize(t *testing.T) {
	program := []byte("a program")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(program)
	}))
	defer srv.Close()
	h := sha256.Sum256(program)
	right := hex.EncodeToString(h[:])
	mk := func(path, sum string, size int64) *Index {
		return &Index{Plugins: []Entry{{ID: "a/b", Versions: []Version{{Version: "1.0.0", Artifacts: []Artifact{{OS: "linux", Arch: "amd64", URL: srv.URL + path, SHA256: sum, Size: size}}}}}}}
	}
	if p := VerifyDownloads(context.Background(), srv.Client(), mk("/ok", right, int64(len(program)))); len(p) != 0 {
		t.Errorf("a correct entry: %v", p)
	}
	if p := VerifyDownloads(context.Background(), srv.Client(), mk("/ok", strings.Repeat("0", 64), int64(len(program)))); !has(p, "checksum") {
		t.Errorf("wrong checksum: %v", p)
	}
	if p := VerifyDownloads(context.Background(), srv.Client(), mk("/ok", right, 3)); !has(p, "size") {
		t.Errorf("wrong size: %v", p)
	}
	if p := VerifyDownloads(context.Background(), srv.Client(), mk("/missing", right, 9)); !has(p, "HTTP 404") {
		t.Errorf("missing file: %v", p)
	}
}

func TestTheOfficialNamespaceCanOnlyBeHostedByTheMaintainer(t *testing.T) {
	idx := good()
	idx.Plugins[0].ID = "girvi/whatsapp"
	for i := range idx.Plugins[0].Versions[0].Artifacts {
		idx.Plugins[0].Versions[0].Artifacts[i].URL = "https://github.com/anant-barlota/girvi-plugins/releases/download/whatsapp/v1.0.0/x"
	}
	if p := Validate(idx, Policy{GitHubOwnerMustMatchID: true}); len(p) != 0 {
		t.Errorf("the maintainer may host girvi/ plugins: %v", p)
	}
	for i := range idx.Plugins[0].Versions[0].Artifacts {
		idx.Plugins[0].Versions[0].Artifacts[i].URL = "https://github.com/girvi/whatsapp/releases/download/v1.0.0/x"
	}
	if p := Validate(idx, Policy{GitHubOwnerMustMatchID: true}); !has(p, "allowed for the plugin id") {
		t.Errorf("anyone else claiming girvi/ must be refused, got %v", p)
	}
}

// A path that starts with an allowed account must not be able to walk into another
// account's repository.
func TestDownloadsCannotEscapeTheAllowedAccount(t *testing.T) {
	const ok = "https://github.com/acme/hook/releases/download/v1.0.0/girvi-plugin-hook_1.0.0_linux_amd64"
	if msg := checkDownloadURL(ok, "acme", Policy{GitHubOwnerMustMatchID: true}); msg != "" {
		t.Fatalf("a real release asset must pass: %s", msg)
	}
	if msg := checkDownloadURL("https://github.com/acme/hook/releases/download/whatsapp/v1.0.0/asset", "acme", Policy{GitHubOwnerMustMatchID: true}); msg != "" {
		t.Errorf("a tag with a slash is allowed: %s", msg)
	}
	for name, raw := range map[string]string{
		"dot-dot segments":      "https://github.com/acme/hook/releases/download/v1/../../../../attacker/evil/releases/download/v1/bin",
		"encoded dot-dot":       "https://github.com/acme/%2e%2e/attacker/evil/releases/download/v1/bin",
		"mixed-case encoding":   "https://github.com/acme/hook/releases/download/v1/%2E%2e/bin",
		"a dot segment":         "https://github.com/acme/hook/releases/download/v1/./bin",
		"backslash":             "https://github.com/acme/hook/releases/download/v1\\..\\bin",
		"empty segment":         "https://github.com/acme/hook/releases/download//bin",
		"too short":             "https://github.com/acme/hook/releases/download/v1",
		"not a release":         "https://github.com/acme/hook/archive/refs/heads/main.zip",
		"a query string":        ok + "?x=1",
		"a fragment":            ok + "#x",
		"a port":                "https://github.com:8443/acme/hook/releases/download/v1/bin",
		"another account":       "https://github.com/attacker/hook/releases/download/v1/bin",
		"another host":          "https://evil.example/acme/hook/releases/download/v1/bin",
		"a lookalike subdomain": "https://github.com.evil.example/acme/hook/releases/download/v1/bin",
	} {
		if msg := checkDownloadURL(raw, "acme", Policy{GitHubOwnerMustMatchID: true}); msg == "" {
			t.Errorf("%s must be refused: %s", name, raw)
		}
	}
}
