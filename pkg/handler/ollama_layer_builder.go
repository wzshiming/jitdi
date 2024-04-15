package handler

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/cache"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/wzshiming/jitdi/pkg/atomic"
)

type OllamaLayerBuilder struct {
	modelCachePath string

	fileBuilder *FileLayerBuilder
}

func NewOllamaLayerBuilder(modelCachePath string, fileBuilder *FileLayerBuilder) *OllamaLayerBuilder {
	return &OllamaLayerBuilder{
		modelCachePath: modelCachePath,
		fileBuilder:    fileBuilder,
	}
}

func (b *OllamaLayerBuilder) Build(modelPath, workDir string) ([]mutate.Addendum, error) {
	o := crane.GetOptions()

	ref, err := name.ParseReference(modelPath, o.Name...)
	if err != nil {
		return nil, fmt.Errorf("parsing reference %q: %w", modelPath, err)
	}

	rmt, err := remote.Get(ref, o.Remote...)
	if err != nil {
		return nil, err
	}

	img, err := rmt.Image()
	if err != nil {
		return nil, err
	}

	img = cache.Image(img, newFilesystemCache(b.modelCachePath))

	err = saveManifest(img, b.modelCachePath, "", "", "")
	if err != nil {
		return nil, err
	}

	return b.tarModel(img, modelPath, workDir)
}

func (b *OllamaLayerBuilder) tarModel(image v1.Image, modelPath, workDir string) ([]mutate.Addendum, error) {
	layers, err := image.Layers()
	if err != nil {
		return nil, err
	}

	var addendums []mutate.Addendum

	a, err := b.tarManifest(image, modelPath, workDir)
	if err != nil {
		return nil, err
	}
	addendums = append(addendums, a...)

	a, err = b.tarConfig(image, workDir)
	if err != nil {
		return nil, err
	}

	addendums = append(addendums, a...)

	for _, layer := range layers {
		a, err = b.tarLayer(layer, workDir)
		if err != nil {
			return nil, err
		}

		addendums = append(addendums, a...)
	}

	return addendums, nil
}

func (b *OllamaLayerBuilder) tarConfig(image v1.Image, workDir string) ([]mutate.Addendum, error) {
	confBlob, err := image.RawConfigFile()
	if err != nil {
		return nil, err
	}

	newPath := path.Join(workDir, "blobs", "sha256:"+atomic.SumSha256(confBlob))
	size := int64(len(confBlob))
	return b.fileBuilder.BuildFile(bytes.NewBuffer(confBlob), newPath, size)

}
func (b *OllamaLayerBuilder) tarManifest(image v1.Image, modelPath, workDir string) ([]mutate.Addendum, error) {
	m, err := image.RawManifest()
	if err != nil {
		return nil, err
	}

	newPath := path.Join(workDir, "manifests", strings.Replace(modelPath, ":", "/", 1))
	size := int64(len(m))

	return b.fileBuilder.BuildFile(bytes.NewBuffer(m), newPath, size)
}

func (b *OllamaLayerBuilder) tarLayer(layer v1.Layer, workDir string) ([]mutate.Addendum, error) {
	l, err := layer.Uncompressed()
	if err != nil {
		return nil, err
	}
	defer l.Close()

	digest, err := layer.Digest()
	if err != nil {
		return nil, err
	}

	size, err := layer.Size()
	if err != nil {
		return nil, err
	}

	newPath := path.Join(workDir, "blobs", digest.String())

	return b.fileBuilder.BuildFile(l, newPath, size)
}
