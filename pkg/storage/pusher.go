package storage

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type Pusher interface {
	PushImage(ctx context.Context, ref name.Reference, image v1.Image) error
	PushImageWithIndex(ctx context.Context, repo name.Repository, image v1.Image) error
	PushImageIndex(ctx context.Context, ref name.Reference, imageIndex v1.ImageIndex) error
}

type pusher struct {
	options options

	pusher *remote.Pusher
}

func NewPusher(opts ...option) (Pusher, error) {
	p := &pusher{}
	for _, opt := range opts {
		err := opt(&p.options)
		if err != nil {
			return nil, err
		}
	}

	r, err := remote.NewPusher(p.options.opts...)
	if err != nil {
		return nil, err
	}
	p.pusher = r

	return p, nil
}

func (p *pusher) PushImage(ctx context.Context, ref name.Reference, image v1.Image) error {
	layers, err := image.Layers()
	if err != nil {
		return err
	}
	for _, layer := range layers {
		err = p.pusher.Upload(ctx, ref.Context(), layer)
		if err != nil {
			return err
		}
	}

	err = p.pusher.Push(ctx, ref, image)
	if err != nil {
		return err
	}

	return nil
}

func (p *pusher) PushImageWithIndex(ctx context.Context, repo name.Repository, image v1.Image) error {
	layers, err := image.Layers()
	if err != nil {
		return err
	}
	for _, layer := range layers {
		err = p.pusher.Upload(ctx, repo, layer)
		if err != nil {
			return err
		}
	}

	digest, err := image.Digest()
	if err != nil {
		return err
	}
	err = p.pusher.Push(ctx, repo.Digest(digest.String()), image)
	if err != nil {
		return err
	}

	return nil
}

func (p *pusher) PushImageIndex(ctx context.Context, ref name.Reference, imageIndex v1.ImageIndex) error {
	err := p.pusher.Push(ctx, ref, imageIndex)
	if err != nil {
		return err
	}
	return nil
}
