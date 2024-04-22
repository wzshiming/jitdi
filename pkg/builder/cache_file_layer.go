package builder

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/stream"

	"github.com/wzshiming/jitdi/pkg/atomic"
)

type cacheFileLayer struct {
	linkPath string
	v1.Layer

	diffID v1.Hash
	size   int64
}

func NewCacheFileLayer(linkPath string, layer v1.Layer) v1.Layer {
	return &cacheFileLayer{
		linkPath: linkPath,
		Layer:    layer,
	}
}

func (c *cacheFileLayer) Digest() (v1.Hash, error) {
	digest, err := c.Layer.Digest()
	if err == nil {
		_, err := os.Stat(c.linkPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				size, err := c.Layer.Size()
				if err != nil {
					return v1.Hash{}, err
				}
				diffID, err := c.Layer.DiffID()
				if err != nil {
					return v1.Hash{}, err
				}
				err = encodeLinkInfo(c.linkPath, digest, diffID, size)
				if err != nil {
					slog.Error("write file", "err", err, "path", c.linkPath)
				}
			} else {
				slog.Error("stat file", "err", err, "path", c.linkPath)
			}
		}
		return digest, nil
	}

	if errors.Is(err, stream.ErrNotComputed) {
		h, diffID, size, err := decodeLinkInfo(c.linkPath)
		if err == nil {
			c.size = size
			c.diffID = diffID
			return h, nil
		}

		if !errors.Is(err, os.ErrNotExist) {
			slog.Error("read file", "err", err, "path", c.linkPath)
		}
	}

	return v1.Hash{}, err
}

func (c *cacheFileLayer) Size() (int64, error) {
	if c.size > 0 {
		return c.size, nil
	}
	return c.Layer.Size()
}

func (c *cacheFileLayer) DiffID() (v1.Hash, error) {
	if c.diffID != (v1.Hash{}) {
		return c.diffID, nil
	}
	return c.Layer.DiffID()
}

func encodeLinkInfo(linkPath string, digest v1.Hash, diffID v1.Hash, size int64) error {
	err := atomic.WriteFile(linkPath, []byte(fmt.Sprintf("%s %s %d", digest, diffID, size)), 0644)
	return err
}

func decodeLinkInfo(linkPath string) (digest, diffID v1.Hash, size int64, err error) {
	c, err := os.ReadFile(linkPath)
	if err != nil {
		return v1.Hash{}, v1.Hash{}, 0, err
	}

	var d string
	var f string
	_, err = fmt.Sscanf(string(c), "%s %s %d", &d, &f, &size)
	if err != nil {
		return v1.Hash{}, v1.Hash{}, 0, err
	}

	digest, err = v1.NewHash(d)
	if err != nil {
		return v1.Hash{}, v1.Hash{}, 0, err
	}

	diffID, err = v1.NewHash(f)
	if err != nil {
		return v1.Hash{}, v1.Hash{}, 0, err
	}
	return digest, diffID, size, nil
}
