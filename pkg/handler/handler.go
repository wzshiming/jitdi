package handler

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/wzshiming/httpseek"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/wzshiming/jitdi/pkg/apis/v1alpha1"
	"github.com/wzshiming/jitdi/pkg/atomic"
	"github.com/wzshiming/jitdi/pkg/builder"
	"github.com/wzshiming/jitdi/pkg/client/clientset/versioned"
	"github.com/wzshiming/jitdi/pkg/pattern"
	"github.com/wzshiming/jitdi/pkg/storage"
)

type Handler struct {
	manifestPath string
	blobPath     string
	linkPath     string

	storageRegistry string

	buildMutex atomic.SyncMap[string, *sync.RWMutex]

	crMut sync.Mutex

	imageRules []*pattern.Rule
	imageCR    []*pattern.Rule
	imageStore cache.Store

	registryRules map[string]*v1alpha1.RegistrySpec
	registryCR    map[string]*v1alpha1.RegistrySpec
	registryStore cache.Store

	clientset *versioned.Clientset
}

type option func(*Handler)

func WithStorageRegistry(storageRegistry string) option {
	return func(h *Handler) {
		h.storageRegistry = storageRegistry
	}
}

func WithClientset(clientset *versioned.Clientset) option {
	return func(h *Handler) {
		h.clientset = clientset
	}
}

func WithCache(cache string) option {
	return func(h *Handler) {
		h.manifestPath = path.Join(cache, "manifests")
		h.blobPath = path.Join(cache, "blobs")
		h.linkPath = path.Join(cache, "links")
	}
}

func WithImageConfig(imageConfig []*v1alpha1.Image) option {
	return func(h *Handler) {
		rules := make([]*pattern.Rule, 0, len(imageConfig))
		for _, c := range imageConfig {
			r, err := pattern.NewRule(&c.Spec)
			if err != nil {
				slog.Error("newImageRule", "err", err)
				continue
			}
			rules = append(rules, r)
		}

		sort.Slice(rules, func(i, j int) bool {
			return rules[i].LessThan(rules[j])
		})

		h.imageRules = rules
	}
}

func WithRegistryConfig(registryConfig []*v1alpha1.Registry) option {
	return func(h *Handler) {
		registries := map[string]*v1alpha1.RegistrySpec{}
		for _, c := range registryConfig {
			registries[c.Name] = &c.Spec
		}

		h.registryRules = registries
	}
}

func NewHandler(opts ...option) (*Handler, error) {
	h := &Handler{}

	for _, opt := range opts {
		opt(h)
	}

	if h.clientset != nil {
		go h.startWatchImageCR(context.Background())
	}

	return h, nil
}

func (h *Handler) startWatchImageCR(ctx context.Context) {
	api := h.clientset.ApisV1alpha1().Images()
	store, controller := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
				return api.List(ctx, opts)
			},
			WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
				return api.Watch(ctx, opts)
			},
		},
		&v1alpha1.Image{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				h.resetImageCR()
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				h.resetImageCR()
			},
			DeleteFunc: func(obj interface{}) {
				h.resetImageCR()
			},
		},
	)
	h.imageStore = store
	controller.Run(ctx.Done())
}

func (h *Handler) startWatchRegistryCR(ctx context.Context) {
	api := h.clientset.ApisV1alpha1().Registries()
	store, controller := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
				return api.List(ctx, opts)
			},
			WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
				return api.Watch(ctx, opts)
			},
		},
		&v1alpha1.Image{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				h.resetRegistryCR()
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				h.resetRegistryCR()
			},
			DeleteFunc: func(obj interface{}) {
				h.resetRegistryCR()
			},
		},
	)
	h.registryStore = store
	controller.Run(ctx.Done())
}

func (h *Handler) resetImageCR() {
	h.crMut.Lock()
	defer h.crMut.Unlock()
	h.imageCR = nil
}

func (h *Handler) resetRegistryCR() {
	h.crMut.Lock()
	defer h.crMut.Unlock()
	h.registryStore = nil
}

func (h *Handler) getImageRules() []*pattern.Rule {
	if h.imageStore == nil {
		return h.imageRules
	}

	h.crMut.Lock()
	defer h.crMut.Unlock()
	if h.imageCR == nil {
		list := h.imageStore.List()
		cr := make([]*pattern.Rule, 0, len(h.imageRules)+len(list))
		cr = append(cr, h.imageRules...)

		for _, item := range list {
			image := item.(*v1alpha1.Image)
			r, err := pattern.NewRule(&image.Spec)
			if err != nil {
				slog.Error("newImageRule", "err", err)
				continue
			}
			cr = append(cr, r)
		}
		sort.Slice(cr, func(i, j int) bool {
			return cr[i].LessThan(cr[j])
		})

		h.imageCR = cr
	}

	return h.imageCR
}

func (h *Handler) getRegistryRules() map[string]*v1alpha1.RegistrySpec {
	if h.registryStore == nil {
		return h.registryRules
	}

	h.crMut.Lock()
	defer h.crMut.Unlock()
	if h.registryCR == nil {
		list := h.registryStore.List()
		cr := map[string]*v1alpha1.RegistrySpec{}
		for k, v := range h.registryRules {
			cr[k] = v
		}

		for _, item := range list {
			registry := item.(*v1alpha1.Registry)
			cr[registry.Name] = &registry.Spec
		}

		h.registryCR = cr
	}

	return h.registryCR
}

func (h *Handler) getAuthn(host string) authn.Authenticator {
	rs := h.getRegistryRules()
	r, ok := rs[host]
	if !ok {
		return nil
	}

	if r.Authentication != nil {
		if ba := r.Authentication.BaseAuth; ba != nil {
			return &authn.Basic{
				Username: ba.Username,
				Password: ba.Password,
			}
		}
	}
	return nil
}

func (h *Handler) getPusher(ref name.Reference) (storage.Pusher, error) {
	reg := ref.Context().RegistryStr()

	auth := h.getAuthn(reg)
	if auth == nil {
		return storage.NewPusher()
	}

	return storage.NewPusher(
		storage.WithAuth(
			auth,
		),
	)
}

func (h *Handler) getPuller(ref name.Reference) (*storage.Puller, error) {
	reg := ref.Context().RegistryStr()

	auth := h.getAuthn(reg)
	if auth == nil {
		return storage.NewPuller()
	}

	return storage.NewPuller(
		storage.WithAuth(
			auth,
		),
	)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/v2/") {
		_ = regErrNotFound.Write(w)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		_ = regErrUnsupported.Write(w)
		return
	}

	if r.URL.Path == "/v2/" {
		w.Write([]byte("{}"))
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		_ = regErrNotFound.Write(w)
		return
	}

	image := strings.Join(parts[2:len(parts)-2], "/")

	typ := parts[len(parts)-2]
	switch typ {
	case "blobs":
		h.blobs(w, r, image, parts[len(parts)-1])
	case "manifests":
		h.manifests(w, r, image, parts[len(parts)-1])
	default:
		_ = regErrNotFound.Write(w)
	}
}

func (h *Handler) manifests(w http.ResponseWriter, r *http.Request, image, tag string) {
	if strings.HasPrefix(tag, "sha256:") {
		if h.storageRegistry != "" {
			h.forwardManifestBlob(w, r, image, tag)
			return
		}
		blobPath := storage.LocalBlobPath(h.blobPath, tag)
		serveManifest(w, r, blobPath)
		return
	}

	ref := image + ":" + tag
	rules := h.getImageRules()

	var action *pattern.Action
	i := slices.IndexFunc(rules, func(rule *pattern.Rule) bool {
		a, ok := rule.Match(ref)
		if ok {
			action = a
		}
		return ok
	})

	if i < 0 {
		regErrNotFound.Write(w)
		return
	}

	h.localManifests(w, r, image, tag, action)
	return
}

func (h *Handler) buildAndSave(ctx context.Context, repo, tag string, action *pattern.Action) error {
	ref := repo + ":" + tag
	mut, ok := h.buildMutex.LoadOrStore(ref, &sync.RWMutex{})
	if ok {
		mut.RLock()
		defer mut.RUnlock()
		return nil
	}

	mut.Lock()
	defer func() {
		h.buildMutex.Delete(ref)
		mut.Unlock()
	}()

	source := action.GetBaseImage()

	refSource, err := name.ParseReference(source)
	if err != nil {
		return err
	}
	puller, err := h.getPuller(refSource)
	if err != nil {
		return err
	}

	desc, err := puller.Get(ctx, refSource)
	if err != nil {
		return err
	}

	var (
		pusher         storage.Pusher
		refDestination name.Reference
	)

	destination := h.storageRegistry
	if destination != "" {
		refDestination, err = name.ParseReference(destination + "/" + action.GetMatchImage())
		if err != nil {
			return err
		}

		pusher, err = h.getPusher(refDestination)
		if err != nil {
			return err
		}
	} else {
		refDestination, err = name.ParseReference(action.GetMatchImage())
		if err != nil {
			return err
		}

		pusher = storage.NewLocalPusher(h.blobPath, h.manifestPath)
	}

	// Fixed time, keep the result consistent
	now := time.Time{}

	roundTripper := httpseek.NewMustReaderTransport(http.DefaultTransport, func(request *http.Request, err error) error {
		slog.Warn("httpseek", "err", err, "request", request)
		return nil
	})

	linkPath := h.linkPath

	if desc.MediaType.IsIndex() {
		imageIndex, err := desc.ImageIndex()
		if err != nil {
			return err
		}

		index, err := builder.NewImageIndex(imageIndex)
		if err != nil {
			return err
		}

		indexManifest, err := index.ImageIndex().IndexManifest()
		if err != nil {
			return err
		}

		ps := action.GetPlatforms()

		index.ClearImage()

		manifests := indexManifest.Manifests
		for _, manifest := range manifests {
			platform := manifest.Platform
			if platform == nil {
				continue
			}

			if ps != nil {
				i := slices.IndexFunc(ps, func(p v1alpha1.Platform) bool {
					return platform.OS == p.OS && platform.Architecture == p.Architecture
				})
				if i < 0 {
					continue
				}
			}

			image, err := index.ImageIndex().Image(manifest.Digest)
			if err != nil {
				return err
			}

			newImage, err := mutateImage(image, action.GetMutates(manifest.Platform), linkPath, now, roundTripper)
			if err != nil {
				return err
			}

			err = pusher.PushImageWithIndex(ctx, refDestination.Context(), newImage)
			if err != nil {
				return err
			}

			err = index.AppendImage(newImage, platform)
			if err != nil {
				return err
			}
		}

		err = pusher.PushImageIndex(ctx, refDestination, index.ImageIndex())
		if err != nil {
			return err
		}

	} else {
		image, err := desc.Image()
		if err != nil {
			return err
		}

		image, err = mutateImage(image, action.GetMutates(desc.Platform), linkPath, now, roundTripper)
		if err != nil {
			return err
		}

		err = pusher.PushImage(ctx, refDestination, image)
		if err != nil {
			return err
		}

	}

	return nil
}
