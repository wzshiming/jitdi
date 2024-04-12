package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/cache"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/wzshiming/jitdi/pkg/apis/v1alpha1"
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
	cacheOllamaBlobs := path.Join(cache, "ollama", "blobs")

	for _, p := range []string{cacheBlobs, cacheManifests, cacheTmp, cacheOllamaBlobs} {
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

func (i *imageBuilder) Build(ctx context.Context, newImage string, meta *MutateMeta) error {
	o := crane.GetOptions(
		crane.WithContext(ctx),
	)

	src := meta.GetBaseImage()
	ref, err := name.ParseReference(src, o.Name...)
	if err != nil {
		return fmt.Errorf("parsing reference %q: %w", src, err)
	}

	index, err := remote.Index(ref, o.Remote...)
	if err != nil {
		return err
	}

	indexManifest, err := index.IndexManifest()
	if err != nil {
		return err
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

	switch indexManifest.MediaType {
	case types.DockerManifestList, types.OCIImageIndex:
		cacheMutate := map[string][]mutate.Addendum{}
		manifests := make([]v1.Descriptor, 0, len(indexManifest.Manifests))
		for _, m := range indexManifest.Manifests {
			img, err := index.Image(m.Digest)
			if err != nil {
				return err
			}
			img = cache.Image(img, NewFilesystemCache(i.cacheBlobs))

			plfm := m.Platform.String()

			mutates, ok := cacheMutate[plfm]
			if !ok {
				mu := meta.GetMutates(m.Platform)
				addendum, err := i.buildAddendum(m.MediaType, mu)
				if err != nil {
					return fmt.Errorf("build addendum: %w", err)
				}
				cacheMutate[plfm] = addendum
				mutates = addendum
			}

			if len(mutates) != 0 {
				img, err = mutate.Append(img, mutates...)
				if err != nil {
					return fmt.Errorf("mutate append: %w", err)
				}
			}

			err = i.saveIndexManifest(img)
			if err != nil {
				return err
			}

			digest, err := img.Digest()
			if err != nil {
				return err
			}
			size, err := img.Size()
			if err != nil {
				return err
			}
			manifests = append(manifests, v1.Descriptor{
				MediaType: m.MediaType,
				Size:      size,
				Digest:    digest,
				Platform:  m.Platform,
			})
		}

		indexManifest.Manifests = manifests

		err = i.saveIndex(indexManifest, image, tag)
		if err != nil {
			return err
		}
	case types.OCIManifestSchema1, types.DockerManifestSchema2:
		rmt, err := remote.Get(ref, o.Remote...)
		if err != nil {
			return err
		}

		img, err := rmt.Image()
		if err != nil {
			return err
		}

		img = cache.Image(img, NewFilesystemCache(i.cacheBlobs))

		mutates := meta.GetMutates(rmt.Platform)
		if len(mutates) != 0 {
			addendum, err := i.buildAddendum(indexManifest.MediaType, mutates)
			if err != nil {
				return err
			}
			img, err = mutate.Append(img, addendum...)
			if err != nil {
				return err
			}
		}

		err = i.saveManifest(img, image, tag)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *imageBuilder) buildAddendum(mediaType types.MediaType, mutates []v1alpha1.Mutate) ([]mutate.Addendum, error) {
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

			builder := NewFileLayerBuilder(i.cacheTmp, mode, creationTime)
			r, err := builder.Build(m.File.Source, m.File.Destination)
			if err != nil {
				return nil, fmt.Errorf("file layer builder: %w", err)
			}

			dataLayer, err := tarball.LayerFromFile(r,
				tarball.WithMediaType(layerMediaType),
			)
			if err != nil {
				return nil, fmt.Errorf("toLayer: %w", err)
			}

			layers = append(layers, mutate.Addendum{
				Layer: dataLayer,
				History: v1.History{
					Author:    "jitdi",
					CreatedBy: fmt.Sprintf("COPY %s %s", m.File.Source, m.File.Destination),
					Comment:   fmt.Sprintf("Copy %s to %s", m.File.Source, m.File.Destination),
					Created:   v1.Time{creationTime},
				},
			})
		} else if m.Ollama != nil {
			builder := NewOllamaLayerBuilder(i.cacheTmp, i.cacheOllamaBlobs, 0444, creationTime)
			r, err := builder.Build(context.Background(), m.Ollama.Model, m.Ollama.WorkDir)
			if err != nil {
				return nil, err
			}

			dataLayer, err := tarball.LayerFromFile(r,
				tarball.WithMediaType(layerMediaType),
			)
			if err != nil {
				return nil, err
			}

			layers = append(layers, mutate.Addendum{
				Layer: dataLayer,
				History: v1.History{
					Author:    "jitdi",
					CreatedBy: fmt.Sprintf("OLLAMA_PULL %s %s", m.Ollama.Model, m.Ollama.WorkDir),
					Comment:   fmt.Sprintf("Pull %s to %s", m.Ollama.Model, m.Ollama.WorkDir),
					Created:   v1.Time{creationTime},
				},
			})
		}
	}

	return layers, nil
}

func (i *imageBuilder) manifestPath(image, tag string) string {
	return path.Join(i.cacheManifests, image, tag, "manifest.json")
}

func (i *imageBuilder) blobsPath(hex string) string {
	return path.Join(i.cacheBlobs, "sha256:"+hex)
}

func (i *imageBuilder) blobsPathWithPrefix(hex string) string {
	return path.Join(i.cacheBlobs, hex)
}

func (i *imageBuilder) saveManifest(img v1.Image, name, tag string) error {
	manifest, err := img.Manifest()
	if err != nil {
		return err
	}
	manifestBlob, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	sum256 := sha256.Sum256(manifestBlob)

	cfgHex := hex.EncodeToString(sum256[:])

	err = atomicWriteFile(i.manifestPath(name, tag), manifestBlob, 0644)
	if err != nil {
		return err
	}

	err = atomicWriteFile(i.blobsPath(cfgHex), manifestBlob, 0644)
	if err != nil {
		return err
	}

	// Write the config.
	cfgName, err := img.ConfigName()
	if err != nil {
		return err
	}
	cfgBlob, err := img.RawConfigFile()
	if err != nil {
		return err
	}

	err = atomicWriteFile(i.blobsPath(cfgName.Hex), cfgBlob, 0644)
	if err != nil {
		return err
	}

	// Write the layers.
	layers, err := img.Layers()
	if err != nil {
		return err
	}

	var seenLayerDigests = map[string]struct{}{}
	for _, l := range layers {
		d, err := l.Digest()
		if err != nil {
			return err
		}

		hex := d.Hex
		if _, ok := seenLayerDigests[hex]; ok {
			continue
		}
		seenLayerDigests[hex] = struct{}{}

		r, err := l.Compressed()
		if err != nil {
			return err
		}

		f, err := atomicWriteFileStream(i.blobsPath(hex), 0644)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, r)
		f.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (i *imageBuilder) saveIndex(index *v1.IndexManifest, name, tag string) error {
	manifestBlob, err := json.Marshal(index)
	if err != nil {
		return err
	}
	sum256 := sha256.Sum256(manifestBlob)

	cfgHex := hex.EncodeToString(sum256[:])

	err = atomicWriteFile(i.blobsPath(cfgHex), manifestBlob, 0644)
	if err != nil {
		return err
	}
	err = atomicWriteFile(i.manifestPath(name, tag), manifestBlob, 0644)
	if err != nil {
		return err
	}

	return err
}

func (i *imageBuilder) saveIndexManifest(img v1.Image) error {
	manifest, err := img.Manifest()
	if err != nil {
		return err
	}
	manifestBlob, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	sum256 := sha256.Sum256(manifestBlob)

	cfgHex := hex.EncodeToString(sum256[:])

	err = atomicWriteFile(i.blobsPath(cfgHex), manifestBlob, 0644)
	if err != nil {
		return err
	}

	// Write the config.
	cfgName, err := img.ConfigName()
	if err != nil {
		return err
	}
	cfgBlob, err := img.RawConfigFile()
	if err != nil {
		return err
	}

	err = atomicWriteFile(i.blobsPath(cfgName.Hex), cfgBlob, 0644)
	if err != nil {
		return err
	}

	// Write the layers.
	layers, err := img.Layers()
	if err != nil {
		return err
	}

	var seenLayerDigests = map[string]struct{}{}
	for _, l := range layers {
		d, err := l.Digest()
		if err != nil {
			return err
		}

		hex := d.Hex
		if _, ok := seenLayerDigests[hex]; ok {
			continue
		}
		seenLayerDigests[hex] = struct{}{}

		r, err := l.Compressed()
		if err != nil {
			return err
		}

		f, err := atomicWriteFileStream(i.blobsPath(hex), 0644)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, r)
		f.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
