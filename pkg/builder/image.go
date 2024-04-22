package builder

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

type Image struct {
	baseImage v1.Image
	image     v1.Image
	mediaType types.MediaType
}

func NewImage(baseImage v1.Image) (*Image, error) {
	mediaType, err := baseImage.MediaType()
	if err != nil {
		return nil, err
	}
	return &Image{
		baseImage: baseImage,
		image:     baseImage,
		mediaType: mediaType,
	}, nil
}

func (i *Image) layerMediaType() types.MediaType {
	switch i.mediaType {
	default:
		return types.DockerLayer
	case types.OCIManifestSchema1:
		return types.OCILayer
	case types.DockerManifestSchema2:
		return types.DockerLayer
	}
}

func (i *Image) AppendFileAsNewLayer(file *File) error {
	rc := Tar(file)

	layer := stream.NewLayer(rc,
		stream.WithMediaType(i.layerMediaType()),
		stream.WithCompressionLevel(0),
	)

	img, err := mutate.Append(i.image, mutate.Addendum{
		Layer: layer,
		History: v1.History{
			Author:    "jitdi",
			CreatedBy: fmt.Sprintf("Add %s", file.Path),
		},
	})
	if err != nil {
		return err
	}
	i.image = img

	return nil
}

func (i *Image) AppendFileAsNewLayerWithLink(file *File, link string) error {
	rc := Tar(file)

	var layer v1.Layer
	layer = stream.NewLayer(rc,
		stream.WithMediaType(i.layerMediaType()),
		stream.WithCompressionLevel(0),
	)

	layer = NewCacheFileLayer(link, layer)

	img, err := mutate.Append(i.image, mutate.Addendum{
		Layer: layer,
		History: v1.History{
			Author:    "jitdi",
			CreatedBy: fmt.Sprintf("Add %s", file.Path),
		},
	})
	if err != nil {
		return err
	}
	i.image = img

	return nil
}

func (i *Image) Image() v1.Image {
	return i.image
}
