package storage

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type Puller struct {
	options options

	puller *remote.Puller
}

func NewPuller(opts ...option) (*Puller, error) {
	p := &Puller{}
	for _, opt := range opts {
		err := opt(&p.options)
		if err != nil {
			return nil, err
		}
	}

	r, err := remote.NewPuller(p.options.opts...)
	if err != nil {
		return nil, err
	}

	p.puller = r

	return p, nil
}

func (p *Puller) Head(ctx context.Context, ref name.Reference) (*v1.Descriptor, error) {
	return p.puller.Head(ctx, ref)
}

func (p *Puller) Get(ctx context.Context, ref name.Reference) (*remote.Descriptor, error) {
	return p.puller.Get(ctx, ref)
}
