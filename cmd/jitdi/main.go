package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/wzshiming/jitdi/pkg/apis/v1alpha1"
	"github.com/wzshiming/jitdi/pkg/client/clientset/versioned"
	"github.com/wzshiming/jitdi/pkg/handler"
)

var (
	address           string
	cache             string
	storageImageProxy string

	config     []string
	kubeconfig string
	master     string
)

func init() {
	pflag.StringVar(&address, "address", ":8888", "listen on the address")

	pflag.StringVar(&cache, "cache", "./cache", "cache directory")
	pflag.StringVar(&storageImageProxy, "storage-image-proxy", "", "storage image proxy")

	pflag.StringSliceVarP(&config, "config", "c", nil, "config file")
	pflag.StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig file")
	pflag.StringVar(&master, "master", "", "master url")
	pflag.Parse()
}

func main() {
	ctx := context.Background()

	logger := slog.Default()

	staticImageConfig, staticRegistryConfig, err := loadConfigFile(config...)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	var clientset *versioned.Clientset
	if kubeconfig != "" {
		clientConfig, err := clientcmd.BuildConfigFromFlags(master, kubeconfig)
		if err != nil {
			logger.Error("failed to BuildConfigFromFlags", "err", err)
			os.Exit(1)
		}
		clientset, err = versioned.NewForConfig(clientConfig)
		if err != nil {
			logger.Error("failed to NewForConfig", "err", err)
			os.Exit(1)
		}

	} else {
		if master == "" {
			logger.Info("Neither --kubeconfig nor --master was specified")
			logger.Info("Using the inClusterConfig")
		}
		clientConfig, err := rest.InClusterConfig()
		if err != nil {
			logger.Warn("failed to InClusterConfig", "err", err)
		} else {
			clientset, err = versioned.NewForConfig(clientConfig)
			if err != nil {
				logger.Error("failed to NewForConfig", "err", err)
			}
		}
	}

	mux := http.NewServeMux()
	h, err := handler.NewHandler(
		handler.WithCache(cache),
		handler.WithStorageImageProxy(storageImageProxy),
		handler.WithClientset(clientset),
		handler.WithImageConfig(staticImageConfig),
		handler.WithRegistryConfig(staticRegistryConfig),
	)
	if err != nil {
		logger.Error("failed to NewHandler", "err", err)
		os.Exit(1)
	}

	mux.Handle("/v2/", h)

	server := http.Server{
		BaseContext: func(listener net.Listener) context.Context {
			return ctx
		},
		Handler: handlers.LoggingHandler(os.Stderr, mux),
		Addr:    address,
	}

	err = server.ListenAndServe()
	if err != nil {
		logger.Error("failed to ListenAndServe", "err", err)
		os.Exit(1)
	}
}

func loadConfigFile(path ...string) ([]*v1alpha1.Image, []*v1alpha1.Registry, error) {
	var images []*v1alpha1.Image
	var registries []*v1alpha1.Registry
	for _, p := range path {
		file, err := os.Open(p)
		if err != nil {
			return nil, nil, err
		}
		imgs, regs, err := loadConfig(file)
		if err != nil {
			return nil, nil, err
		}
		images = append(images, imgs...)
		registries = append(registries, regs...)
	}
	return images, registries, nil
}

func loadConfig(r io.Reader) ([]*v1alpha1.Image, []*v1alpha1.Registry, error) {
	var images []*v1alpha1.Image
	var registries []*v1alpha1.Registry
	decoder := yaml.NewYAMLToJSONDecoder(r)
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, nil, fmt.Errorf("failed to decode %q: %w", raw, err)
		}
		if len(raw) == 0 {
			// Ignore empty documents
			continue
		}

		meta := metav1.TypeMeta{}
		err = json.Unmarshal(raw, &meta)
		if err != nil {
			return nil, nil, err
		}
		if meta.APIVersion != v1alpha1.GroupVersion.String() {
			return nil, nil, fmt.Errorf("unexpected APIVersion %q", meta.APIVersion)
		}
		switch meta.Kind {
		default:
			return nil, nil, fmt.Errorf("unexpected Kind %q", meta.Kind)
		case v1alpha1.ImageKind:
			img := v1alpha1.Image{}
			err = json.Unmarshal(raw, &img)
			if err != nil {
				return nil, nil, err
			}
			images = append(images, &img)
		case v1alpha1.RegistryKind:
			registry := v1alpha1.Registry{}
			err = json.Unmarshal(raw, &registry)
			if err != nil {
				return nil, nil, err
			}
			registries = append(registries, &registry)
		}

	}
	return images, registries, nil
}
