package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// TestRunWritesLoadableArchive guards the exact tar layout QNAP Container
// Station expects: manifest.json + repositories + <id>/layer.tar, all in the
// classic docker-save V1 format. The archive is read back through
// go-containerregistry, which validates the manifest/config/layer references.
func TestRunWritesLoadableArchive(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "net-bloop")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho bloop\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, []byte("-----BEGIN CERTIFICATE-----\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "image.tar")
	if err := run(bin, ca, "net-bloop:test", out, "test"); err != nil {
		t.Fatalf("run: %v", err)
	}

	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatalf("archive is not readable as a docker image tar: %v", err)
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Architecture != "amd64" || cfg.OS != "linux" {
		t.Fatalf("unexpected platform %s/%s", cfg.OS, cfg.Architecture)
	}
	if got := cfg.Config.Entrypoint; len(got) != 1 || got[0] != "/net-bloop" {
		t.Fatalf("unexpected entrypoint %v", got)
	}
	layers, err := img.Layers()
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 1 {
		t.Fatalf("want exactly one layer, got %d", len(layers))
	}
	// The legacy writer must emit an uncompressed layer.tar; a gzip blob is
	// what Container Station refused in the previous build.
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".gz" {
			t.Fatalf("unexpected compressed blob %s", f.Name())
		}
	}
}
