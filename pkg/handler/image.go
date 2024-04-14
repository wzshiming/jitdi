package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"golang.org/x/sync/errgroup"

	"github.com/wzshiming/jitdi/pkg/apis/v1alpha1"
	"github.com/wzshiming/jitdi/pkg/atomic"
	"github.com/wzshiming/jitdi/pkg/pattern"
)

type imageBuilder struct {
	cacheOllamaBlobs string
	cacheTmp         string
	cacheBlobs       string
	cacheManifests   string
}

func newImageBuilder(cache string) (*imageBuilder, error) {
	cacheBlobs := path.Join(cache, "blobs")
	cacheManifests := path.Join(cache, "manifests")
	cacheTmp := path.Join(cache, "tmp")
	cacheOllamaBlobs := path.Join(cacheTmp, "ollama", "blobs")

	for _, p := range []string{cacheBlobs, cacheManifests, cacheOllamaBlobs} {
		err := os.MkdirAll(p, 0755)
		if err != nil {
			return nil, err
		}
	}
	return &imageBuilder{
		cacheOllamaBlobs: cacheOllamaBlobs,
		cacheBlobs:       cacheBlobs,
		cacheManifests:   cacheManifests,
		cacheTmp:         cacheTmp,
	}, nil
}

func (b *imageBuilder) Build(newImage string, meta *pattern.Action) error {
	o := crane.GetOptions()

	src := meta.GetBaseImage()
	ref, err := name.ParseReference(src, o.Name...)
	if err != nil {
		return fmt.Errorf("parsing reference %q: %w", src, err)
	}

	rmt, err := remote.Get(ref, o.Remote...)
	if err != nil {
		return fmt.Errorf("getting remote %q: %w", src, err)
	}

	var (
		image string
		tag   string
	)
	s := strings.Split(newImage, ":")
	if len(s) == 1 {
		image = s[0]
		tag = "latest"
	} else {
		image = s[0]
		tag = s[1]
	}

	switch rmt.MediaType {
	default:
		return fmt.Errorf("unknown media type %q", rmt.MediaType)
	case types.DockerManifestList, types.OCIImageIndex:
		imageIndex, err := rmt.ImageIndex()
		if err != nil {
			return fmt.Errorf("getting image index: %w", err)
		}
		indexManifest, err := imageIndex.IndexManifest()
		if err != nil {
			return fmt.Errorf("getting index manifest: %w", err)
		}
		g := errgroup.Group{}
		g.SetLimit(2)

		images := make([]v1.Image, len(indexManifest.Manifests))
		for i, manifest := range indexManifest.Manifests {
			img, err := imageIndex.Image(manifest.Digest)
			if err != nil {
				return fmt.Errorf("getting image %q: %w", manifest.Digest, err)
			}
			// img = cache.Image(img, NewFilesystemCache(b.cacheBlobs))

			err = b.saveManifest(img, "", "")
			if err != nil {
				return fmt.Errorf("save manifest: %w", err)
			}

			index := i
			doMutate := func() error {
				img, err := b.mutateManifest(img, meta, manifest.Platform, manifest.MediaType)
				if err != nil {
					return fmt.Errorf("mutate manifest: %w", err)
				}

				err = b.saveManifest(img, "", "")
				if err != nil {
					return fmt.Errorf("save manifest: %w", err)
				}
				images[index] = img
				return nil
			}
			err = doMutate()
			if err != nil {
				return err
			}
			//if index == 0 {
			//	err = doMutate()
			//	if err != nil {
			//		return err
			//	}
			//} else {
			//	g.Go(doMutate)
			//}
		}
		err = g.Wait()
		if err != nil {
			return err
		}

		manifests := make([]v1.Descriptor, 0, len(indexManifest.Manifests))
		for i, img := range images {
			if img == nil {
				continue
			}
			digest, err := img.Digest()
			if err != nil {
				return fmt.Errorf("getting digest: %w", err)
			}

			//mediaType, err := img.MediaType()
			//if err != nil {
			//	return fmt.Errorf("getting media type: %w", err)
			//}

			size, err := img.Size()
			if err != nil {
				return fmt.Errorf("getting size: %w", err)
			}

			manifest := indexManifest.Manifests[i]
			manifests = append(manifests, v1.Descriptor{
				Size:      size,
				Digest:    digest,
				MediaType: manifest.MediaType,
				Platform:  manifest.Platform,
			})
		}
		if len(manifests) == 0 {
			return fmt.Errorf("no valid images")
		}

		indexManifest.Manifests = manifests
		err = b.saveIndexManifest(indexManifest, image, tag)
		if err != nil {
			return fmt.Errorf("save index manifest: %w", err)
		}
	case types.OCIManifestSchema1, types.DockerManifestSchema2:
		img, err := rmt.Image()
		if err != nil {
			return fmt.Errorf("getting image: %w", err)
		}
		//img = cache.Image(img, NewFilesystemCache(b.cacheBlobs))

		img, err = b.mutateManifest(img, meta, rmt.Platform, rmt.MediaType)
		if err != nil {
			return fmt.Errorf("mutate manifest: %w", err)
		}

		err = b.saveManifest(img, image, tag)
		if err != nil {
			return fmt.Errorf("save manifest: %w", err)
		}
	case types.DockerManifestSchema1:
		img, err := rmt.Schema1()
		if err != nil {
			return fmt.Errorf("getting image: %w", err)
		}
		//img = cache.Image(img, NewFilesystemCache(b.cacheBlobs))

		img, err = b.mutateManifest(img, meta, rmt.Platform, rmt.MediaType)
		if err != nil {
			return fmt.Errorf("mutate manifest: %w", err)
		}

		err = b.saveManifest(img, image, tag)
		if err != nil {
			return fmt.Errorf("save manifest: %w", err)
		}
	}
	return nil
}

func (b *imageBuilder) buildAddendum(mediaType types.MediaType, mutates []v1alpha1.Mutate) ([]mutate.Addendum, error) {
	var layerMediaType types.MediaType
	switch mediaType {
	default:
		return nil, fmt.Errorf("unknown media type %q", mediaType)
	case types.OCIManifestSchema1:
		layerMediaType = types.OCILayer
	case types.DockerManifestSchema2:
		layerMediaType = types.DockerLayer
	}

	var layers []mutate.Addendum

	creationTime := time.Now()

	for _, m := range mutates {
		if m.File != nil {
			var mode int64 = 0644
			if m.File.Mode != "" {
				m, err := strconv.ParseUint(m.File.Mode, 0, 0)
				if err == nil {
					mode = int64(m)
				}
			}

			builder := NewFileLayerBuilder(b.cacheTmp, mode, creationTime, layerMediaType)
			addendums, err := builder.Build(m.File.Source, m.File.Destination)
			if err != nil {
				return nil, fmt.Errorf("file layer builder: %w", err)
			}

			return addendums, nil
		} else if m.Ollama != nil {

			builder := NewOllamaLayerBuilder(b.cacheOllamaBlobs, NewFileLayerBuilder(b.cacheTmp, 0644, creationTime, layerMediaType))
			addendums, err := builder.Build(m.Ollama.Model, m.Ollama.WorkDir)
			if err != nil {
				return nil, fmt.Errorf("ollama layer builder: %w", err)
			}

			return addendums, nil
		}
	}

	return layers, nil
}

func (b *imageBuilder) ManifestPath(image, tag string) string {
	return path.Join(b.cacheManifests, image, tag, "manifest.json")
}

func (b *imageBuilder) BlobsPath(hex string) string {
	switch len(hex) {
	case 64:
		return path.Join(b.cacheBlobs, "sha256:"+hex)
	case 71:
		return path.Join(b.cacheBlobs, hex)
	}
	return path.Join(b.cacheBlobs, "unknown:"+hex)
}

func (b *imageBuilder) mutateManifest(img v1.Image, meta *pattern.Action, p *v1.Platform, mediaType types.MediaType) (v1.Image, error) {
	mutates := meta.GetMutates(p)
	if len(mutates) == 0 {
		return img, nil
	}

	addendums, err := b.buildAddendum(mediaType, mutates)
	if err != nil {
		return nil, fmt.Errorf("build addendum: %w", err)
	}

	if len(addendums) != 0 {
		img, err = mutate.Append(img, addendums...)
		if err != nil {
			return nil, fmt.Errorf("mutate append: %w", err)
		}
	}

	return img, nil
}

func (b *imageBuilder) saveIndex(index *v1.IndexManifest, name, tag string) error {
	manifestBlob, err := json.Marshal(index)
	if err != nil {
		return err
	}
	sum256 := sha256.Sum256(manifestBlob)

	cfgHex := hex.EncodeToString(sum256[:])

	err = atomic.WriteFile(b.BlobsPath(cfgHex), manifestBlob, 0644)
	if err != nil {
		return err
	}
	err = atomic.WriteFile(b.ManifestPath(name, tag), manifestBlob, 0644)
	if err != nil {
		return err
	}

	return err
}

func (b *imageBuilder) saveIndexManifest(index *v1.IndexManifest, name, tag string) error {
	manifestBlob, err := json.Marshal(index)
	if err != nil {
		return err
	}

	err = atomic.WriteFile(b.BlobsPath(atomic.SumSha256(manifestBlob)), manifestBlob, 0644)
	if err != nil {
		return err
	}
	err = atomic.WriteFile(b.ManifestPath(name, tag), manifestBlob, 0644)
	if err != nil {
		return err
	}

	return err
}

func (b *imageBuilder) saveManifest(img v1.Image, name, tag string) error {
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	for _, layer := range layers {
		err = b.saveLayer(layer)
		if err != nil {
			return fmt.Errorf("save layer: %w", err)
		}
	}

	manifestBlob, err := img.RawManifest()
	if err != nil {
		return fmt.Errorf("getting raw manifest: %w", err)
	}

	err = atomic.WriteFile(b.BlobsPath(atomic.SumSha256(manifestBlob)), manifestBlob, 0644)
	if err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	if name != "" {
		if tag == "" {
			tag = "latest"
		}
		err = atomic.WriteFile(b.ManifestPath(name, tag), manifestBlob, 0644)
		if err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}
	}

	// Write the config.
	configName, err := img.ConfigName()
	if err != nil {
		return fmt.Errorf("getting config name: %w", err)
	}
	configBlob, err := img.RawConfigFile()
	if err != nil {
		return fmt.Errorf("getting raw config file: %w", err)
	}

	err = atomic.WriteFile(b.BlobsPath(configName.Hex), configBlob, 0644)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func saveLayer(layer v1.Layer, cachePath string) (string, error) {
	f, err := os.CreateTemp(cachePath, "tmp-")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}

	r, err := layer.Compressed()
	if err != nil {
		return "", fmt.Errorf("getting compressed: %w", err)
	}

	_, err = io.Copy(f, r)
	if err != nil {
		_ = f.Close()
		return "", fmt.Errorf("copy: %w", err)
	}
	_ = f.Close()

	digest, err := layer.Digest()
	if err != nil {
		return "", fmt.Errorf("getting digest: %w", err)
	}

	p := path.Join(cachePath, digest.String())
	err = os.Rename(f.Name(), p)
	if err != nil {
		return "", fmt.Errorf("rename: %w", err)
	}

	return p, nil
}

func (b *imageBuilder) saveLayer(layer v1.Layer) (retErr error) {
	r, err := layer.Compressed()
	if err != nil {
		return fmt.Errorf("getting compressed: %w", err)
	}
	defer func() {
		err := r.Close()
		if err != nil {
			if retErr == nil {
				retErr = err
			} else {
				retErr = errors.Join(retErr, err)
			}
		}
	}()

	digest, err := layer.Digest()
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

	cachePath := path.Join(b.cacheBlobs, digest.String())
	_, err = os.Stat(cachePath)
	if err == nil {
		slog.Info("skip layer", "size", size, "path", cachePath)
		return nil
	}

	sum := sha256.New()
	wc, err := atomic.OpenFileWithWriter(cachePath, 0644)
	if err != nil {
		return fmt.Errorf("open file with writer: %w", err)
	}

	n, err := io.Copy(wc, io.TeeReader(r, sum))
	if err != nil {
		_ = wc.Abort()
		return fmt.Errorf("copy: %w", err)
	}

	hash := hex.EncodeToString(sum.Sum(nil))
	if hash != digest.Hex {
		_ = wc.Abort()
		return fmt.Errorf("hash mismatch %q != %q and size mismatch %d != %d", hash, digest.Hex, n, size)
	}

	if n != size {
		_ = wc.Abort()
		return fmt.Errorf("size mismatch %d != %d", n, size)
	}

	err = wc.Close()
	if err != nil {
		_ = wc.Abort()
		return fmt.Errorf("close: %w", err)
	}

	slog.Info("save layer", "size", size, "path", cachePath)

	return nil
}
