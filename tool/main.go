// Command tool checks the catalog and adds new plugin versions to it.
//
//	go run ./tool check [-base old-index.json] [-offline]
//	go run ./tool add -id owner/name -version 1.2.3 -repo owner/repo -asset girvi-plugin-name [...]
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const indexFile = "index.json"

var platforms = [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tool check | tool add")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "check":
		err = runCheck(os.Args[2:])
	case "add":
		err = runAdd(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func load(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	base := fs.String("base", "", "the index as it was before this change; published versions may not be altered")
	offline := fs.Bool("offline", false, "do not download the programs")
	anyHost := fs.Bool("any-host", false, "allow downloads that are not GitHub releases of the id's owner")
	_ = fs.Parse(args)

	idx, err := load(indexFile)
	if err != nil {
		return err
	}
	problems := Validate(idx, Policy{GitHubOwnerMustMatchID: !*anyHost})
	if *base != "" {
		if old, err := load(*base); err == nil {
			problems = append(problems, CheckImmutable(old, idx)...)
		}
	}
	if len(problems) == 0 && !*offline {
		problems = VerifyDownloads(context.Background(), http.DefaultClient, idx)
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Println("-", p)
		}
		return fmt.Errorf("%d problem(s) found", len(problems))
	}
	fmt.Printf("ok: %d plugin(s)\n", len(idx.Plugins))
	return nil
}

func runAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	id := fs.String("id", "", "plugin id, owner/name")
	version := fs.String("version", "", "version, such as 1.2.3 (no leading v)")
	protocol := fs.Int("protocol", 3, "plugin contract version it was built for")
	repo := fs.String("repo", "", "GitHub repository with the release, owner/repo")
	tag := fs.String("tag", "", "release tag (default v<version>)")
	asset := fs.String("asset", "", "program name prefix, such as girvi-plugin-webhook")
	name := fs.String("name", "", "display name (new plugins)")
	author := fs.String("author", "", "author (new plugins)")
	desc := fs.String("description", "", "one line description (new plugins)")
	license := fs.String("license", "", "license (new plugins)")
	home := fs.String("homepage", "", "homepage (new plugins)")
	caps := fs.String("capabilities", "", "comma separated capabilities, as the plugin declares them")
	requires := fs.String("requires", "", "comma separated requirements, such as network,storage.private")
	_ = fs.Parse(args)
	if *id == "" || *version == "" || *repo == "" || *asset == "" {
		return fmt.Errorf("-id, -version, -repo and -asset are required")
	}
	if *tag == "" {
		*tag = "v" + *version
	}

	idx, err := load(indexFile)
	if err != nil {
		return err
	}
	v := Version{Version: *version, Protocol: *protocol, Capabilities: split(*caps), Requires: split(*requires)}
	for _, p := range platforms {
		ext := ""
		if p[0] == "windows" {
			ext = ".exe"
		}
		u := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s_%s_%s_%s%s", *repo, *tag, *asset, *version, p[0], p[1], ext)
		fmt.Println("downloading", u)
		sum, size, err := hashURL(context.Background(), http.DefaultClient, u)
		if err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
		v.Artifacts = append(v.Artifacts, Artifact{OS: p[0], Arch: p[1], URL: u, SHA256: sum, Size: size})
	}

	var entry *Entry
	for i := range idx.Plugins {
		if idx.Plugins[i].ID == *id {
			entry = &idx.Plugins[i]
		}
	}
	if entry == nil {
		idx.Plugins = append(idx.Plugins, Entry{ID: *id, Name: *name, Author: *author, Description: *desc, License: *license, Homepage: *home})
		entry = &idx.Plugins[len(idx.Plugins)-1]
	}
	for _, old := range entry.Versions {
		if old.Version == *version {
			return fmt.Errorf("%s %s is already listed; a published version cannot be changed", *id, *version)
		}
	}
	entry.Versions = append(entry.Versions, v)

	if problems := Validate(idx, Policy{GitHubOwnerMustMatchID: true}); len(problems) > 0 {
		return fmt.Errorf("the result would not be valid:\n- %s", strings.Join(problems, "\n- "))
	}
	return os.WriteFile(indexFile, Canonical(idx), 0o644)
}

func split(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}
