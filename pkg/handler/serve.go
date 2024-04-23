package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/wzshiming/jitdi/pkg/pattern"
	"github.com/wzshiming/jitdi/pkg/storage"
)

func (h *Handler) blobs(w http.ResponseWriter, r *http.Request, image, hash string) {
	if h.storageRegistry != "" {
		h.forwardBlob(w, r, image, hash)
		return
	}

	blobPath := storage.LocalBlobPath(h.blobPath, hash)
	f, err := os.Open(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			_ = regErrBlobUnknown.Write(w)
			return
		}
		_ = regErrInternal(err).Write(w)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}
	http.ServeContent(w, r, path.Base(r.URL.Path), stat.ModTime(), f)
}

func (h *Handler) localManifests(w http.ResponseWriter, r *http.Request, image, tag string, action *pattern.Action) {
	if h.storageRegistry != "" {
		h.forwardManifest(w, r, image, tag, action)
		return
	}

	manifestPath := storage.LocalManifestPath(h.manifestPath, image, tag)
	_, err := os.Stat(manifestPath)
	if err != nil {
		err := h.buildAndSave(context.Background(), image, tag, action)
		if err != nil {
			slog.Error("image.Build", "err", err)
			_ = regErrInternal(err).Write(w)
			return
		}
	}

	serveManifest(w, r, manifestPath)
}

func (h *Handler) forwardBlob(w http.ResponseWriter, r *http.Request, image, hash string) {
	refDestination, err := name.NewDigest(h.storageRegistry + "/" + image + "@" + hash)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	puller, err := h.getPuller(refDestination)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	layer, err := puller.Layer(r.Context(), refDestination)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	mediaType, err := layer.MediaType()
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}
	w.Header().Set("Content-Type", string(mediaType))
	if strings.HasSuffix(string(mediaType), "gzip") {
		r, err := layer.Compressed()
		if err != nil {
			_ = regErrInternal(err).Write(w)
			return
		}

		_, err = io.Copy(w, r)
		if err != nil {
			_ = regErrInternal(err).Write(w)
			return
		}
	} else {
		r, err := layer.Uncompressed()
		if err != nil {
			_ = regErrInternal(err).Write(w)
			return
		}

		_, err = io.Copy(w, r)
		if err != nil {
			_ = regErrInternal(err).Write(w)
			return
		}
	}
}

func (h *Handler) forwardManifestBlob(w http.ResponseWriter, r *http.Request, image, hash string) {
	refDestination, err := name.NewDigest(h.storageRegistry + "/" + image + "@" + hash)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	puller, err := h.getPuller(refDestination)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	desc, err := puller.Get(r.Context(), refDestination)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	manifest, err := desc.RawManifest()
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	w.Header().Set("Content-Type", string(desc.MediaType))
	w.Write(manifest)
}

func (h *Handler) forwardManifest(w http.ResponseWriter, r *http.Request, image, tag string, action *pattern.Action) {
	refDestination, err := name.ParseReference(h.storageRegistry + "/" + action.GetMatchImage())
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	puller, err := h.getPuller(refDestination)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	desc, err := puller.Get(r.Context(), refDestination)
	if err != nil {
		err := h.buildAndSave(context.Background(), image, tag, action)
		if err != nil {
			slog.Error("image.Build", "err", err)
			_ = regErrInternal(err).Write(w)
			return
		}

		desc, err = puller.Get(r.Context(), refDestination)
	}

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
}

func serveManifest(w http.ResponseWriter, r *http.Request, manifestPath string) {
	f, err := os.Open(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			_ = regErrNotFound.Write(w)
			return
		}
		_ = regErrInternal(err).Write(w)
		return
	}
	defer f.Close()

	mediaType := struct {
		MediaType types.MediaType `json:"mediaType,omitempty"`
	}{}

	err = json.NewDecoder(f).Decode(&mediaType)
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}
	stat, err := f.Stat()
	if err != nil {
		_ = regErrInternal(err).Write(w)
		return
	}

	w.Header().Set("Content-Type", string(mediaType.MediaType))
	_, _ = f.Seek(0, 0)
	http.ServeContent(w, r, path.Base(r.URL.Path), stat.ModTime(), f)
}
