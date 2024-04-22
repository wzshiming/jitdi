package builder

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
)

type ImageIndex struct {
	baseImageIndex v1.ImageIndex
	imageIndex     v1.ImageIndex
}

func NewImageIndex(baseImageIndex v1.ImageIndex) (*ImageIndex, error) {
	return &ImageIndex{
		baseImageIndex: baseImageIndex,
		imageIndex:     baseImageIndex,
	}, nil
}

func (i *ImageIndex) ClearImage() {
	i.imageIndex = mutate.RemoveManifests(i.imageIndex, func(desc v1.Descriptor) bool {
		return true
	})
}

func (i *ImageIndex) RemoveImage(platform *v1.Platform) {
	i.imageIndex = mutate.RemoveManifests(i.imageIndex, func(desc v1.Descriptor) bool {
		if desc.Platform != nil {
			if desc.Platform.OS == platform.OS &&
				desc.Platform.Architecture == platform.Architecture {
				return false
			}
		}
		return true
	})
}

func (i *ImageIndex) AppendImage(image v1.Image, platform *v1.Platform) error {
	manifest, err := image.Manifest()
	if err != nil {
		return fmt.Errorf("getting manifest: %w", err)
	}

	digest, err := image.Digest()
	if err != nil {
		return fmt.Errorf("getting digest: %w", err)
	}

	size, err := image.Size()
	if err != nil {
		return fmt.Errorf("getting size: %w", err)
	}

	index := mutate.AppendManifests(i.imageIndex, mutate.IndexAddendum{
		Add: image,
		Descriptor: v1.Descriptor{
			Size:      size,
			Digest:    digest,
			MediaType: manifest.MediaType,
			Platform:  platform,
		},
	})

	i.imageIndex = index
	return nil
}

func (i *ImageIndex) ImageIndex() v1.ImageIndex {
	return i.imageIndex
}
