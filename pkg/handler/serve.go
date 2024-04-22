package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/wzshiming/jitdi/pkg/pattern"
	"github.com/wzshiming/jitdi/pkg/storage"
)

func (h *Handler) blobs(w http.ResponseWriter, r *http.Request, image, hash string) {
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
	if strings.HasPrefix(tag, "sha256:") {
		blobPath := storage.LocalBlobPath(h.blobPath, tag)
		serveManifest(w, r, blobPath)
		return
	}

	manifestPath := storage.LocalManifestPath(h.manifestPath, image, tag)
	_, err := os.Stat(manifestPath)
	if err != nil {
		err := h.buildAndPush(context.Background(), image, tag, action)
		if err != nil {
			slog.Error("image.Build", "err", err)
			_ = regErrInternal(err).Write(w)
			return
		}
	}

	serveManifest(w, r, manifestPath)
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
