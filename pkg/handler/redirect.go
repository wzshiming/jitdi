package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/wzshiming/jitdi/pkg/pattern"
)

func (h *Handler) redirectManifest(w http.ResponseWriter, r *http.Request, image, tag string, action *pattern.Action) {
	storageImage := action.GetStorageImage()

	storageRef, err := name.ParseReference(storageImage)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	puller, err := h.getPuller(storageRef)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	desc, err := puller.Get(r.Context(), storageRef)
	if err != nil {
		var t *transport.Error
		if errors.As(err, &t) {
			if t.StatusCode == http.StatusNotFound {
				err = h.buildAndPush(context.Background(), image, tag, action)
				if err != nil {
					_ = regErrInternal(err).Write(w)
					return
				}
			}
		}

		desc, err = puller.Get(r.Context(), storageRef)
		if err != nil {
			_ = regErrInternal(err).Write(w)
			return
		}
	}
	_ = desc

	manifest, err := desc.RawManifest()
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	w.Header().Set("Content-Type", string(desc.MediaType))
	_, err = w.Write(manifest)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	newURL := ReferenceToURL(storageRef)

	slog.Info("redirect", "from", r.URL, "to", newURL, "image", storageRef)
	http.Redirect(w, r, newURL, http.StatusTemporaryRedirect)
}
