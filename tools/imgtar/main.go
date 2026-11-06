// Command imgtar turns the statically linked net-bloop binary into a
// docker-loadable image tar without needing a Docker daemon.
//
// Usage:
//
//	go run . -binary ../../dist/linux-amd64/net-bloop \
//	           -ca ./ca-certificates.crt \
//	           -tag net-bloop:0.2.6 \
//	           -out ../../dist/net-bloop-0.2.6-linux-amd64.tar
package main

import (
	"archive/tar"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	legacytarball "github.com/google/go-containerregistry/pkg/legacy/tarball"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func main() {
	binary := flag.String("binary", "", "linux/amd64 net-bloop binary")
	ca := flag.String("ca", "", "PEM CA bundle copied to /etc/ssl/certs/ca-certificates.crt")
	tag := flag.String("tag", "net-bloop:0.2.6", "image tag")
	out := flag.String("out", "net-bloop.tar", "output docker image tar")
	version := flag.String("version", "0.2.6", "version label")
	flag.Parse()
	if *binary == "" || *ca == "" {
		fmt.Fprintln(os.Stderr, "-binary and -ca are required")
		os.Exit(2)
	}
	if err := run(*binary, *ca, *tag, *out, *version); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%s)\n", *out, *tag)
}

func run(binary, ca, tag, out, version string) error {
	tmp, err := os.MkdirTemp("", "net-bloop-img-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	layerPath := filepath.Join(tmp, "layer.tar")
	if err := writeLayer(layerPath, binary, ca); err != nil {
		return err
	}
	layer, err := tarball.LayerFromFile(layerPath)
	if err != nil {
		return fmt.Errorf("layer: %w", err)
	}

	created := v1.Time{Time: time.Now().UTC()}
	cfg := &v1.ConfigFile{
		Architecture: "amd64",
		OS:           "linux",
		Created:      created,
		RootFS:       v1.RootFS{Type: "layers"},
		Config: v1.Config{
			Entrypoint: []string{"/net-bloop"},
			Cmd:        []string{"-config", "/data/net-bloop.toml"},
			WorkingDir: "/data",
			Env: []string{
				"SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
				"TZ=Asia/Shanghai",
				"NETBLOOP_CONFIG=/data/net-bloop.toml",
			},
			ExposedPorts: map[string]struct{}{"16601/tcp": {}, "33320/tcp": {}},
			Volumes:      map[string]struct{}{"/data": {}},
			Labels: map[string]string{
				"org.opencontainers.image.title":       "net-bloop",
				"org.opencontainers.image.version":     version,
				"org.opencontainers.image.description": "DDNS + reverse proxy + STUN tunnel",
			},
		},
	}
	img, err := mutate.ConfigFile(empty.Image, cfg)
	if err != nil {
		return err
	}
	img, err = mutate.AppendLayers(img, layer)
	if err != nil {
		return err
	}
	ref, err := name.ParseReference(tag)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// Write the classic V1 "docker save" layout on purpose:
	//   <config-sha>.json, <layer-id>/layer.tar, <layer-id>/json,
	//   <layer-id>/VERSION, manifest.json, repositories
	// Newer OCI-style tarballs load fine with `docker load` but QNAP
	// Container Station rejects them as an unsupported format.
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := legacytarball.Write(ref, img, f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func writeLayer(path, binary, ca string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	defer tw.Close()

	if err := addFile(tw, binary, "net-bloop", 0o755); err != nil {
		return err
	}
	if err := addFile(tw, ca, "etc/ssl/certs/ca-certificates.crt", 0o644); err != nil {
		return err
	}
	mod := time.Now().UTC()
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     "data/",
		Mode:     0o755,
		ModTime:  mod,
	}); err != nil {
		return err
	}
	return tw.Close()
}

func addFile(tw *tar.Writer, src, dst string, mode int64) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     dst,
		Mode:     mode,
		Size:     st.Size(),
		ModTime:  st.ModTime().UTC(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, in)
	return err
}
