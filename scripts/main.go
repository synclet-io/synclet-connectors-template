// Command registry validates connector metadata and builds registry.json.
//
//	go run ./scripts validate
//	go run ./scripts registry [--image-prefix ghcr.io/org/repo] [--out dist/registry.json] [--released-only]
//	go run ./scripts pending                    # JSON list of connectors whose version has no release tag yet
//	go run ./scripts changed --base <git-ref>   # JSON list of connectors touched since <git-ref>
//	go run ./scripts check-versions --base <git-ref>
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: registry <validate|registry|pending|changed|check-versions> [flags]")
		os.Exit(2)
	}

	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	base := fs.String("base", "", "git ref to diff against")
	imagePrefix := fs.String("image-prefix", "", "image repository prefix (default ghcr.io/$GITHUB_REPOSITORY)")
	out := fs.String("out", "dist/registry.json", "output file")
	releasedOnly := fs.Bool("released-only", false, "describe each connector as of its newest <dir>-v<version> release tag")

	if err := fs.Parse(args); err != nil {
		return err
	}

	switch cmd {
	case "validate":
		return validate(*root)
	case "registry":
		return writeRegistry(*root, *imagePrefix, *out, *releasedOnly)
	case "pending":
		return pending(*root)
	case "changed":
		return changed(*root, *base)
	case "check-versions":
		return checkVersions(*root, *base)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func validate(root string) error {
	connectors, err := loadConnectors(root)
	if err != nil {
		return err
	}

	for _, c := range connectors {
		fmt.Printf("ok  %s %s\n", c.Dir, c.Meta.Version)
	}

	return nil
}

func writeRegistry(root, imagePrefix, out string, releasedOnly bool) error {
	if imagePrefix == "" {
		var err error
		if imagePrefix, err = defaultImagePrefix(os.Getenv); err != nil {
			return err
		}
	}

	connectors, err := loadConnectors(root)
	if err != nil {
		return err
	}

	if releasedOnly {
		if connectors, err = releasedConnectors(root, connectors); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(buildRegistry(connectors, imagePrefix), "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf("wrote %s (%d connectors)\n", out, len(connectors))

	return nil
}

// releasedConnectors maps every connector to the state of its newest release:
// the working tree when its current version is tagged, otherwise the files at
// the latest <dir>-v* tag. Never-released connectors are dropped, so the
// registry never points at an image tag that was not pushed.
func releasedConnectors(root string, connectors []Connector) ([]Connector, error) {
	tags, err := gitTags(root)
	if err != nil {
		return nil, err
	}

	schema, err := compileSchema(root)
	if err != nil {
		return nil, err
	}

	var released []Connector

	for _, c := range connectors {
		if tags[c.ReleaseTag()] {
			released = append(released, c)

			continue
		}

		tag := latestReleaseTag(tags, c.Dir)
		if tag == "" {
			fmt.Fprintf(os.Stderr, "skip %s: never released\n", c.Dir)

			continue
		}

		prev, err := parseConnector(c.Dir, schema, gitShowReader(root, tag, c.Dir))
		if err != nil {
			return nil, fmt.Errorf("%s at %s: %w", c.Dir, tag, err)
		}

		fmt.Fprintf(os.Stderr, "%s: %s not released yet, publishing %s\n", c.Dir, c.Meta.Version, prev.Meta.Version)
		released = append(released, prev)
	}

	return released, nil
}

func pending(root string) error {
	connectors, err := loadConnectors(root)
	if err != nil {
		return err
	}

	tags, err := gitTags(root)
	if err != nil {
		return err
	}

	type release struct {
		Connector string `json:"connector"`
		Version   string `json:"version"`
		Tag       string `json:"tag"`
	}

	list := []release{}

	for _, c := range connectors {
		if !c.Meta.Tombstone && !tags[c.ReleaseTag()] {
			list = append(list, release{Connector: c.Dir, Version: c.Meta.Version, Tag: c.ReleaseTag()})
		}
	}

	return printJSON(list)
}

func changed(root, base string) error {
	if base == "" {
		return errors.New("--base is required")
	}

	byDir, err := changedFiles(root, base)
	if err != nil {
		return err
	}

	dirs := []string{}

	for _, dir := range sortedKeys(byDir) {
		if _, err := os.Stat(filepath.Join(root, connectorsDir, dir)); err == nil {
			dirs = append(dirs, dir)
		}
	}

	return printJSON(dirs)
}

// checkVersions fails when a connector's image inputs changed but its version
// is already released, since merging would silently skip the release.
func checkVersions(root, base string) error {
	if base == "" {
		return errors.New("--base is required")
	}

	connectors, err := loadConnectors(root)
	if err != nil {
		return err
	}

	byDir, err := changedFiles(root, base)
	if err != nil {
		return err
	}

	tags, err := gitTags(root)
	if err != nil {
		return err
	}

	var errs []error

	for _, c := range connectors {
		files, ok := byDir[c.Dir]
		if !ok || !tags[c.ReleaseTag()] {
			continue
		}

		for _, f := range files {
			if affectsImage(f) {
				fmt.Printf("::error file=%s/%s/meta.json::%s changed but version %s is already released; bump \"version\"\n",
					connectorsDir, c.Dir, f, c.Meta.Version)
				errs = append(errs, fmt.Errorf("%s: bump version (already released as %s)", c.Dir, c.ReleaseTag()))

				break
			}
		}
	}

	return errors.Join(errs...)
}

func printJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}
