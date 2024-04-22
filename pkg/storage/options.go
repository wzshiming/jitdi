package storage

import (
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type options struct {
	opts []remote.Option
}

type option func(p *options) error

func WithAuth(auth authn.Authenticator) func(po *options) error {
	return func(o *options) error {
		o.opts = append(o.opts, remote.WithAuth(auth))
		return nil
	}
}

func WithTransport(t http.RoundTripper) func(po *options) error {
	return func(o *options) error {
		o.opts = append(o.opts, remote.WithTransport(t))
		return nil
	}
}

func WithUserAgent(ua string) func(po *options) error {
	return func(o *options) error {
		o.opts = append(o.opts, remote.WithUserAgent(ua))
		return nil
	}
}
