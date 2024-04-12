package handler

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/cache"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type OllamaLayerBuilder struct {
	tmpPath        string
	modelCachePath string
	mode           int64
	modTime        time.Time
}

func NewOllamaLayerBuilder(tmpPath, modelCachePath string, mode int64, modTime time.Time) *OllamaLayerBuilder {
	return &OllamaLayerBuilder{
		tmpPath:        tmpPath,
		modelCachePath: modelCachePath,
		mode:           mode,
		modTime:        modTime,
	}
}

func (b *OllamaLayerBuilder) Build(ctx context.Context, modelPath, workDir string) (string, error) {
	o := crane.GetOptions(
		crane.WithContext(ctx),
	)

	ref, err := name.ParseReference(modelPath, o.Name...)
	if err != nil {
		return "", fmt.Errorf("parsing reference %q: %w", modelPath, err)
	}

	rmt, err := remote.Get(ref, o.Remote...)
	if err != nil {
		return "", err
	}

	img, err := rmt.Image()
	if err != nil {
		return "", err
	}

	if b.modelCachePath != "" {
		img = cache.Image(img, NewFilesystemCache(b.modelCachePath))
	}

	tmp, err := os.CreateTemp(b.tmpPath, "tmp-")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	sum := sha256.New()

	tw := tar.NewWriter(io.MultiWriter(tmp, sum))
	defer tw.Close()

	err = b.tarModel(tw, img, modelPath, workDir)
	if err != nil {
		return "", err
	}

	p := path.Join(b.tmpPath, fmt.Sprintf("sha256:%x", sum.Sum(nil)))
	err = os.Rename(tmp.Name(), p)
	if err != nil {
		return "", err
	}
	return p, nil
}

func (b *OllamaLayerBuilder) tarModel(tw *tar.Writer, image v1.Image, modelPath, workDir string) error {
	layers, err := image.Layers()
	if err != nil {
		return err
	}

	for _, layer := range layers {
		err = b.tarLayer(tw, layer, workDir)
		if err != nil {
			return err
		}
	}

	err = b.tarManifest(tw, image, modelPath, workDir)
	if err != nil {
		return err
	}

	err = b.tarConfig(tw, image, modelPath, workDir)
	if err != nil {
		return err
	}

	return nil
}

func (b *OllamaLayerBuilder) tarConfig(tw *tar.Writer, image v1.Image, modelPath, workDir string) error {
	confBlob, err := image.RawConfigFile()
	if err != nil {
		return err
	}

	sum256 := sha256.Sum256(confBlob)

	confHex := hex.EncodeToString(sum256[:])

	newPath := path.Join(workDir, "blobs", confHex)
	size := int64(len(confBlob))
	header := &tar.Header{
		Name:     newPath,
		Size:     size,
		Typeflag: tar.TypeReg,
		Mode:     b.mode,
		ModTime:  b.modTime,
	}
	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("tar.Writer.WriteHeader(%q): %w", newPath, err)
	}

	n, err := tw.Write(confBlob)
	if err != nil {
		return fmt.Errorf("tar.Writer.Write(%q): %w", newPath, err)
	}

	if int64(n) != size {
		return fmt.Errorf("size(%q): short write: %d != %d", newPath, n, size)
	}

	return nil

}
func (b *OllamaLayerBuilder) tarManifest(tw *tar.Writer, image v1.Image, modelPath, workDir string) error {
	m, err := image.RawManifest()
	if err != nil {
		return err
	}

	newPath := path.Join(workDir, "manifests", strings.Replace(modelPath, ":", "/", 1))
	size := int64(len(m))
	header := &tar.Header{
		Name:     newPath,
		Size:     size,
		Typeflag: tar.TypeReg,
		Mode:     b.mode,
		ModTime:  b.modTime,
	}

	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("tar.Writer.WriteHeader(%q): %w", newPath, err)
	}

	n, err := tw.Write(m)
	if err != nil {
		return fmt.Errorf("tar.Writer.Write(%q): %w", newPath, err)
	}

	if int64(n) != size {
		return fmt.Errorf("size(%q): short write: %d != %d", newPath, n, size)
	}

	return nil
}

func (b *OllamaLayerBuilder) tarLayer(tw *tar.Writer, layer v1.Layer, workDir string) error {
	l, err := layer.Uncompressed()
	if err != nil {
		return err
	}
	defer l.Close()

	digest, err := layer.Digest()
	if err != nil {
		return err
	}

	size, err := layer.Size()
	if err != nil {
		return err
	}

	newPath := path.Join(workDir, "blobs", digest.String())

	header := &tar.Header{
		Name:     newPath,
		Size:     size,
		Typeflag: tar.TypeReg,
		Mode:     b.mode,
		ModTime:  b.modTime,
	}

	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("tar.Writer.WriteHeader(%q): %w", newPath, err)
	}
	n, err := io.Copy(tw, l)
	if err != nil {
		return fmt.Errorf("io.Copy(%q, %q): %w", newPath, l, err)
	}

	if n != size {
		return fmt.Errorf("io.Copy(%q, %q): short write: %d != %d", newPath, l, n, size)
	}
	return nil
}
