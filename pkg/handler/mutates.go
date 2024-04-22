package handler

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/v1"

	"github.com/wzshiming/jitdi/pkg/apis/v1alpha1"
	"github.com/wzshiming/jitdi/pkg/atomic"
	"github.com/wzshiming/jitdi/pkg/builder"
	"github.com/wzshiming/jitdi/pkg/builder/files"
	"github.com/wzshiming/jitdi/pkg/builder/ollama"
)

func mutateImage(image v1.Image, mutates []v1alpha1.Mutate, linkPath string, now time.Time, transport http.RoundTripper) (v1.Image, error) {
	var err error
	for _, m := range mutates {
		switch {
		case m.File != nil:
			image, err = mutateImageWithFile(image, m.File, linkPath, now, transport)
		case m.Ollama != nil:
			image, err = mutateImageWithOllama(image, m.Ollama, linkPath, now, transport)
		default:
			err = fmt.Errorf("unknown mutate")
		}
		if err != nil {
			return nil, err
		}
	}
	return image, nil
}

func mutateImageWithFile(image v1.Image, f *v1alpha1.File, linkPath string, now time.Time, transport http.RoundTripper) (v1.Image, error) {
	mode := int64(0644)
	if f.Mode != "" {
		m, err := strconv.ParseInt(f.Mode, 0, 0)
		if err != nil {
			return nil, err
		}
		mode = m
	}

	file := files.NewFiles(mode, now, transport)
	fs, err := file.Build(f.Source, f.Destination)
	if err != nil {
		return nil, err
	}

	img, err := builder.NewImage(image)
	if err != nil {
		return nil, err
	}

	for _, v := range fs {
		if linkPath == "" {
			err = img.AppendFileAsNewLayer(v)
			if err != nil {
				return nil, err
			}
		} else {
			err = img.AppendFileAsNewLayerWithLink(v, sumFileInfo(linkPath, v.Path, f))
			if err != nil {
				return nil, err
			}
		}
	}

	return img.Image(), nil
}

func mutateImageWithOllama(image v1.Image, o *v1alpha1.Ollama, linkPath string, now time.Time, transport http.RoundTripper) (v1.Image, error) {
	mode := int64(0644)

	file := ollama.NewOllama(mode, now, transport)
	fs, err := file.Build(o.Model, o.WorkDir, o.ModelName)
	if err != nil {
		return nil, err
	}

	img, err := builder.NewImage(image)
	if err != nil {
		return nil, err
	}

	for _, v := range fs {
		if linkPath == "" {
			err = img.AppendFileAsNewLayer(v)
			if err != nil {
				return nil, err
			}
		} else {
			err = img.AppendFileAsNewLayerWithLink(v, sumOllamaLayerInfo(linkPath, v.Path, o))
			if err != nil {
				return nil, err
			}
		}
	}

	return img.Image(), nil
}

func sumFileInfo(linkPath, mount string, f *v1alpha1.File) string {
	return path.Join(linkPath, mount, atomic.SumSha256([]byte(strings.Join([]string{f.Source, f.Destination, f.Mode}, "\x00"))), "link")
}

func sumOllamaLayerInfo(linkPath, mount string, o *v1alpha1.Ollama) string {
	return path.Join(linkPath, mount, atomic.SumSha256([]byte(strings.Join([]string{o.Model, o.WorkDir}, "\x00"))), "link")
}
