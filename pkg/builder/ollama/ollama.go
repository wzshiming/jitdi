package ollama

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/wzshiming/jitdi/pkg/atomic"
	"github.com/wzshiming/jitdi/pkg/builder"
)

type Ollama struct {
	mode    int64
	modTime time.Time

	opts crane.Options
}

func NewOllama(mode int64, modTime time.Time, transport http.RoundTripper) *Ollama {
	return &Ollama{
		mode:    mode,
		modTime: modTime,
		opts: crane.GetOptions(
			crane.WithTransport(transport),
		),
	}
}

func (o *Ollama) Build(modelPath, workDir, modelName string) ([]*builder.File, error) {
	ref, err := name.ParseReference(modelPath, o.opts.Name...)
	if err != nil {
		return nil, fmt.Errorf("parsing reference %q: %w", modelPath, err)
	}

	rmt, err := remote.Get(ref, o.opts.Remote...)
	if err != nil {
		return nil, err
	}

	img, err := rmt.Image()
	if err != nil {
		return nil, err
	}

	return o.tarModel(img, modelPath, workDir, modelName)
}

func (o *Ollama) tarModel(image v1.Image, modelPath, workDir, modelName string) ([]*builder.File, error) {
	layers, err := image.Layers()
	if err != nil {
		return nil, err
	}

	var addendums []*builder.File

	a, err := o.tarManifest(image, modelPath, workDir, modelName)
	if err != nil {
		return nil, err
	}
	addendums = append(addendums, a)

	a, err = o.tarConfig(image, workDir)
	if err != nil {
		return nil, err
	}

	addendums = append(addendums, a)

	for _, layer := range layers {
		a, err = o.tarLayer(layer, workDir)
		if err != nil {
			return nil, err
		}

		addendums = append(addendums, a)
	}

	return addendums, nil
}

func (o *Ollama) tarConfig(image v1.Image, workDir string) (*builder.File, error) {
	confBlob, err := image.RawConfigFile()
	if err != nil {
		return nil, err
	}

	newPath := path.Join(workDir, "blobs", "sha256:"+atomic.SumSha256(confBlob))
	size := int64(len(confBlob))
	return &builder.File{
		Path:    newPath,
		Mode:    o.mode,
		ModTime: o.modTime,
		OpenReader: func() (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewBuffer(confBlob)), size, nil
		},
	}, nil

}
func (o *Ollama) tarManifest(image v1.Image, modelPath, workDir, modelName string) (*builder.File, error) {
	manifestBlob, err := image.RawManifest()
	if err != nil {
		return nil, err
	}

	if modelName == "" {
		modelName = modelPath
	}

	modelName = strings.Replace(modelName, ":", "/", 1)

	newPath := path.Join(workDir, "manifests", modelName)

	size := int64(len(manifestBlob))

	return &builder.File{
		Path:    newPath,
		Mode:    o.mode,
		ModTime: o.modTime,
		OpenReader: func() (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewBuffer(manifestBlob)), size, nil
		},
	}, nil
}

func (o *Ollama) tarLayer(layer v1.Layer, workDir string) (*builder.File, error) {
	digest, err := layer.Digest()
	if err != nil {
		return nil, err
	}

	newPath := path.Join(workDir, "blobs", digest.String())

	return &builder.File{
		Path:    newPath,
		Mode:    o.mode,
		ModTime: o.modTime,
		OpenReader: func() (io.ReadCloser, int64, error) {
			l, err := layer.Uncompressed()
			if err != nil {
				return nil, 0, err
			}
			size, err := layer.Size()
			if err != nil {
				return nil, 0, err
			}

			return l, size, nil
		},
	}, nil
}
