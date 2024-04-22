package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"

	"github.com/wzshiming/jitdi/pkg/atomic"
)

func LocalBlobPath(cacheBlobs, hash string) string {
	if hash == "" {
		return path.Join(cacheBlobs)
	}
	if strings.HasPrefix(hash, "sha256:") {
		return path.Join(cacheBlobs, hash)
	}

	return path.Join(cacheBlobs, "sha256:"+hash)
}

func LocalManifestPath(cacheManifest, name, tag string) string {
	return path.Join(cacheManifest, name, tag, "manifest.json")
}

type localPusher struct {
	cacheBlobs    string
	cacheManifest string
}

func NewLocalPusher(cacheBlobs, cacheManifest string) Pusher {
	return &localPusher{
		cacheBlobs:    cacheBlobs,
		cacheManifest: cacheManifest,
	}
}

func (l *localPusher) PushImageIndex(ctx context.Context, ref name.Reference, imageIndex v1.ImageIndex) error {
	manifestBlob, err := imageIndex.RawManifest()
	if err != nil {
		return fmt.Errorf("getting raw manifest: %w", err)
	}

	return saveManifest(manifestBlob, l.cacheBlobs, l.cacheManifest, ref.Context().RepositoryStr(), ref.Identifier())
}

func (l *localPusher) PushImage(ctx context.Context, ref name.Reference, image v1.Image) error {
	layers, err := image.Layers()
	if err != nil {
		return err
	}

	for _, layer := range layers {
		err = saveLayer(layer, l.cacheBlobs)
		if err != nil {
			return err
		}
	}

	// Write the config.
	configBlob, err := image.RawConfigFile()
	if err != nil {
		return fmt.Errorf("getting raw config file: %w", err)
	}
	configName, err := image.ConfigName()
	if err != nil {
		return fmt.Errorf("getting config name: %w", err)
	}

	err = atomic.WriteFile(LocalBlobPath(l.cacheBlobs, configName.String()), configBlob, 0644)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	manifestBlob, err := image.RawManifest()
	if err != nil {
		return fmt.Errorf("getting raw manifest: %w", err)
	}

	return saveManifest(manifestBlob, l.cacheBlobs, l.cacheManifest, ref.Context().RepositoryStr(), ref.Identifier())
}

func (l *localPusher) PushImageWithIndex(ctx context.Context, repo name.Repository, image v1.Image) error {
	layers, err := image.Layers()
	if err != nil {
		return err
	}

	for _, layer := range layers {
		err = saveLayer(layer, l.cacheBlobs)
		if err != nil {
			return err
		}
	}

	// Write the config.
	configBlob, err := image.RawConfigFile()
	if err != nil {
		return fmt.Errorf("getting raw config file: %w", err)
	}
	configName, err := image.ConfigName()
	if err != nil {
		return fmt.Errorf("getting config name: %w", err)
	}

	err = atomic.WriteFile(LocalBlobPath(l.cacheBlobs, configName.String()), configBlob, 0644)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	manifestBlob, err := image.RawManifest()
	if err != nil {
		return fmt.Errorf("getting raw manifest: %w", err)
	}

	return saveManifest(manifestBlob, l.cacheBlobs, l.cacheManifest, repo.RepositoryStr(), "")
}

func saveLayer(layer v1.Layer, cacheBlobs string) (retErr error) {
	digest, err := layer.Digest()
	if err == nil {
		cachePath := LocalBlobPath(cacheBlobs, digest.Hex)
		fi, err := os.Stat(cachePath)
		if err == nil {
			size, err := layer.Size()
			if err == nil {
				n := fi.Size()
				if n == size {
					slog.Info("hit layer", "path", cachePath, "size", size)
					return nil
				}
			}
		}
	}

	dir := LocalBlobPath(cacheBlobs, "")

	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	r, err := layer.Compressed()
	if err != nil {
		return fmt.Errorf("getting compressed: %w", err)
	}

	file, err := os.CreateTemp(dir, "tmp-")
	if err != nil {
		_ = r.Close()
		return err
	}
	defer func() {
		err := file.Close()
		if err != nil {
			if retErr == nil {
				retErr = err
			} else {
				retErr = errors.Join(retErr, err)
			}
		}
		if retErr != nil {
			_ = os.Remove(file.Name())
		}
	}()

	_, err = io.Copy(file, r)
	if err != nil {
		_ = r.Close()
		return err
	}
	_ = r.Close()

	digest, err = layer.Digest()
	if err != nil {
		return fmt.Errorf("getting digest: %w", err)
	}

	size, err := layer.Size()
	if err != nil {
		return fmt.Errorf("getting size: %w", err)
	}

	if size <= 0 {
		return fmt.Errorf("size is zero")
	}

	cachePath := LocalBlobPath(cacheBlobs, digest.Hex)
	err = os.Rename(file.Name(), cachePath)
	if err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	slog.Info("save layer", "path", cachePath, "size", size)

	return nil
}

func saveManifest(manifestBlob []byte, cacheBlobs, cacheManifest, name, tag string) error {
	err := atomic.WriteFile(LocalBlobPath(cacheBlobs, atomic.SumSha256(manifestBlob)), manifestBlob, 0644)
	if err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	if name != "" && tag != "" && !strings.HasPrefix(tag, "sha256:") {
		err = atomic.WriteFile(LocalManifestPath(cacheManifest, name, tag), manifestBlob, 0644)
		if err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}
	}
	return nil
}
