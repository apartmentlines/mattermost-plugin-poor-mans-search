// Package main provides the manifest helper for plugin build metadata.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

type pluginManifest struct {
	ID              string                     `json:"id"`
	Version         string                     `json:"version"`
	Name            string                     `json:"name"`
	HomepageURL     string                     `json:"homepage_url"`
	Server          json.RawMessage            `json:"server"`
	Webapp          json.RawMessage            `json:"webapp"`
	ReleaseNotesURL string                     `json:"release_notes_url"`
	raw             map[string]json.RawMessage `json:"-"`
}

// These build-time vars are read from shell commands and populated in ../setup.mk.
var (
	BuildHashShort  string
	BuildTagLatest  string
	BuildTagCurrent string
)

func main() {
	if len(os.Args) <= 1 {
		panic("no cmd specified")
	}

	mf, err := findManifest()
	if err != nil {
		panic("failed to find manifest: " + err.Error())
	}

	cmd := os.Args[1]
	switch cmd {
	case "id":
		fmt.Print(mf.ID)
	case "version":
		fmt.Print(mf.Version)
	case "has_server":
		if len(mf.Server) > 0 && string(mf.Server) != "null" {
			fmt.Print("true")
		}
	case "has_webapp":
		if len(mf.Webapp) > 0 && string(mf.Webapp) != "null" {
			fmt.Print("true")
		}
	case "apply":
		// This plugin does not use generated manifest source files.
	case "dist":
		if err := distManifest(mf); err != nil {
			panic("failed to write manifest to dist directory: " + err.Error())
		}
	case "check":
		if err := checkManifest(mf); err != nil {
			panic("failed to check manifest: " + err.Error())
		}
	default:
		panic("unrecognized command: " + cmd)
	}
}

func findManifest() (*pluginManifest, error) {
	manifestFile, err := os.Open("plugin.json")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open plugin.json")
	}
	defer func() {
		if closeErr := manifestFile.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "failed to close plugin.json: %v\n", closeErr)
		}
	}()

	var mf pluginManifest
	if err = json.NewDecoder(manifestFile).Decode(&mf.raw); err != nil {
		return nil, errors.Wrap(err, "failed to parse plugin.json")
	}

	rawBytes, err := json.Marshal(mf.raw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to re-encode plugin.json")
	}
	if err = json.Unmarshal(rawBytes, &mf); err != nil {
		return nil, errors.Wrap(err, "failed to parse plugin.json")
	}

	if mf.Version == "" {
		mf.Version = inferredVersion()
	}

	if mf.ReleaseNotesURL == "" && BuildTagLatest != "" && mf.HomepageURL != "" {
		mf.ReleaseNotesURL = strings.TrimRight(mf.HomepageURL, "/") + "/releases/tag/" + BuildTagLatest
	}

	return &mf, nil
}

func inferredVersion() string {
	tags := strings.Fields(BuildTagCurrent)
	for _, tag := range tags {
		if strings.HasPrefix(tag, "v") {
			return strings.TrimPrefix(tag, "v")
		}
	}

	if BuildTagLatest != "" {
		return strings.TrimPrefix(BuildTagLatest, "v") + "+" + BuildHashShort
	}

	if BuildHashShort != "" {
		return "0.0.0+" + BuildHashShort
	}

	return "0.0.0-dev"
}

func checkManifest(pluginManifest *pluginManifest) error {
	if pluginManifest.ID == "" {
		return errors.New("id is required")
	}
	if pluginManifest.Name == "" {
		return errors.New("name is required")
	}
	if len(pluginManifest.Server) == 0 && len(pluginManifest.Webapp) == 0 {
		return errors.New("at least one of server or webapp is required")
	}
	return nil
}

func distManifest(pluginManifest *pluginManifest) error {
	raw := make(map[string]json.RawMessage, len(pluginManifest.raw)+2)
	for key, value := range pluginManifest.raw {
		raw[key] = value
	}
	versionBytes, err := json.Marshal(pluginManifest.Version)
	if err != nil {
		return err
	}
	raw["version"] = versionBytes

	if pluginManifest.ReleaseNotesURL != "" {
		releaseNotesBytes, marshalErr := json.Marshal(pluginManifest.ReleaseNotesURL)
		if marshalErr != nil {
			return marshalErr
		}
		raw["release_notes_url"] = releaseNotesBytes
	}

	manifestBytes, err := json.MarshalIndent(raw, "", "    ")
	if err != nil {
		return err
	}

	distPath := filepath.Join("dist", pluginManifest.ID, "plugin.json")
	if err := os.WriteFile(distPath, manifestBytes, 0o600); err != nil {
		return errors.Wrap(err, "failed to write plugin.json")
	}

	return nil
}
