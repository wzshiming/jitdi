package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/wzshiming/jitdi/pkg/pattern"
)

func (h *Handler) redirectManifest(w http.ResponseWriter, r *http.Request, image, tag string, action *pattern.Action, storageImage string) {
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

	_, err = puller.Get(r.Context(), storageRef)
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
	}

	if h.storageImageProxy != "" {
		storageRef, err := name.ParseReference(strings.TrimSuffix(h.storageImageProxy, "/") + "/" + storageImage)
		if err != nil {
			_ = regErrInternal(err).Write(w)
			return
		}
		newURL := ReferenceToURL(storageRef)
		slog.Info("redirect", "from", r.URL, "to", newURL, "image", storageRef)
		http.Redirect(w, r, newURL, http.StatusTemporaryRedirect)
		return
	}
	newURL := ReferenceToURL(storageRef)

	slog.Info("redirect", "from", r.URL, "to", newURL, "image", storageRef)
	http.Redirect(w, r, newURL, http.StatusTemporaryRedirect)
}
