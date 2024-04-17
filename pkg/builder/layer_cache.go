package builder

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/cache"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type fscache struct {
	path string
}

func newFilesystemCache(path string) cache.Cache {
	return &fscache{path}
}

func (fs *fscache) Put(l v1.Layer) (v1.Layer, error) {
	return l, nil
}

func (fs *fscache) Get(h v1.Hash) (v1.Layer, error) {
	l, err := tarball.LayerFromFile(cachepath(fs.path, h))
	if os.IsNotExist(err) || errors.Is(err, io.ErrUnexpectedEOF) {
		slog.Info("cache miss", "path", path.Join(fs.path, h.String()))
		return nil, cache.ErrNotFound
	}

	slog.Info("cache hit", "path", path.Join(fs.path, h.String()))
	return l, err
}

func (fs *fscache) Delete(h v1.Hash) error {
	return nil
}

func cachepath(path string, h v1.Hash) string {
	return filepath.Join(path, h.String())
}
