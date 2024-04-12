package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/wzshiming/jitdi/pkg/apis/v1alpha1"
	"github.com/wzshiming/jitdi/pkg/client/clientset/versioned"
)

type Handler struct {
	mut   sync.Mutex
	image *imageBuilder

	rules []*imageRule

	crMut     sync.Mutex
	cr        []*imageRule
	store     cache.Store
	clientset *versioned.Clientset
}

func NewHandler(cache string, config []*v1alpha1.ImageSpec, clientset *versioned.Clientset) (*Handler, error) {
	rules := make([]*imageRule, 0, len(config))
	for _, c := range config {
		r, err := newImageRule(c)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	builder, err := newImageBuilder(cache)
	if err != nil {
		return nil, err
	}

	h := &Handler{
		image:     builder,
		rules:     rules,
		clientset: clientset,
	}

	if clientset != nil {
		go h.start(context.Background())
	}

	return h, nil
}

func (h *Handler) start(ctx context.Context) {
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
				h.resetCR()
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				h.resetCR()
			},
			DeleteFunc: func(obj interface{}) {
				h.resetCR()
			},
		},
	)
	h.store = store
	controller.Run(ctx.Done())
}

func (h *Handler) resetCR() {
	h.crMut.Lock()
	defer h.crMut.Unlock()
	h.cr = nil
}

func (h *Handler) getRules() []*imageRule {
	if h.store == nil {
		return h.rules
	}

	h.crMut.Lock()
	defer h.crMut.Unlock()
	if h.cr == nil {
		h.cr = h.rules
		for _, item := range h.store.List() {
			image := item.(*v1alpha1.Image)
			r, err := newImageRule(&image.Spec)
			if err != nil {
				slog.Error("newImageRule", "err", err)
				continue
			}
			h.cr = append(h.cr, r)
		}
	}

	return h.cr
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/v2/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if r.URL.Path == "/v2/" {
		w.Write([]byte("ok"))
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	image := strings.Join(parts[2:len(parts)-2], "/")

	typ := parts[len(parts)-2]
	switch typ {
	case "blobs":
		h.blobs(w, r, image, parts[len(parts)-1])
	case "manifests":
		h.manifests(w, r, image, parts[len(parts)-1])
	}
}

func (h *Handler) blobs(w http.ResponseWriter, r *http.Request, image, hash string) {
	http.ServeFile(w, r, h.image.blobsPathWithPrefix(hash))
}

func (h *Handler) manifests(w http.ResponseWriter, r *http.Request, image, tag string) {
	if strings.HasPrefix(tag, "sha256:") {
		serveManifest(w, r, h.image.blobsPathWithPrefix(tag))
		return
	}

	manifestPath := h.image.manifestPath(image, tag)
	_, err := os.Stat(manifestPath)
	if err != nil {
		err := h.build(image, tag)
		if err != nil {
			slog.Error("image.Build", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	serveManifest(w, r, h.image.manifestPath(image, tag))
}

func (h *Handler) build(image, tag string) error {
	h.mut.Lock()
	defer h.mut.Unlock()
	ref := image + ":" + tag
	for _, rule := range h.getRules() {
		mutates, ok := rule.match(ref)
		if ok {
			err := h.image.Build(context.Background(), ref, mutates)
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func serveManifest(w http.ResponseWriter, r *http.Request, manifestPath string) {
	f, err := os.Open(manifestPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	mediaType := struct {
		MediaType types.MediaType `json:"mediaType,omitempty"`
	}{}

	err = json.NewDecoder(f).Decode(&mediaType)
	if err != nil {
		slog.Error("json.Decode", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stat, err := f.Stat()
	if err != nil {
		slog.Error("Stat", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", string(mediaType.MediaType))
	_, _ = f.Seek(0, 0)
	http.ServeContent(w, r, path.Base(r.URL.Path), stat.ModTime(), f)
}

type imageRule struct {
	Match     *pattern
	BaseImage string
	Mutates   []v1alpha1.Mutate
}

func newImageRule(conf *v1alpha1.ImageSpec) (*imageRule, error) {
	pat, err := parsePattern(conf.Match)
	if err != nil {
		return nil, err
	}
	return &imageRule{
		Match:     pat,
		BaseImage: conf.BaseImage,
		Mutates:   conf.Mutates,
	}, nil
}

func (i *imageRule) match(image string) (*MutateMeta, bool) {

	params, ok := i.Match.Match(image)
	if !ok {
		return nil, false
	}

	return &MutateMeta{
		params:    params,
		BaseImage: i.BaseImage,
		Mutates:   i.Mutates,
	}, true
}

type MutateMeta struct {
	params    map[string]string
	BaseImage string
	Mutates   []v1alpha1.Mutate
}

func (m *MutateMeta) GetBaseImage() string {
	baseImage := m.BaseImage
	baseImage = replaceWithParams(baseImage, m.params)

	return baseImage
}

func (m *MutateMeta) GetMutates(p *v1.Platform) []v1alpha1.Mutate {
	mutates := m.Mutates
	mutates = replaceMutateWithParams(mutates, m.params)
	return mutates
}

func replaceWithParams(s string, params map[string]string) string {
	for k, v := range params {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

func replaceMutateWithParams(m []v1alpha1.Mutate, params map[string]string) []v1alpha1.Mutate {
	ms := make([]v1alpha1.Mutate, 0, len(m))
	for _, v := range m {
		if v.File != nil {
			ms = append(ms, v1alpha1.Mutate{
				File: &v1alpha1.File{
					Source:      replaceWithParams(v.File.Source, params),
					Destination: replaceWithParams(v.File.Destination, params),
				},
			})
		} else if v.Ollama != nil {
			ms = append(ms, v1alpha1.Mutate{
				Ollama: &v1alpha1.Ollama{
					Model:   replaceWithParams(v.Ollama.Model, params),
					WorkDir: replaceWithParams(v.Ollama.WorkDir, params),
				},
			})
		}
	}
	return ms
}
