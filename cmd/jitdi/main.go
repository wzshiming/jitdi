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
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/wzshiming/jitdi/pkg/apis/v1alpha1"
	"github.com/wzshiming/jitdi/pkg/client/clientset/versioned"
	"github.com/wzshiming/jitdi/pkg/handler"
)

var (
	address string
	cache   string

	config     string
	kubeconfig string
	master     string
)

func init() {
	pflag.StringVar(&address, "address", ":8888", "listen on the address")
	pflag.StringVar(&cache, "cache", "./cache", "cache directory")

	pflag.StringVarP(&config, "config", "c", "", "config file")
	pflag.StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig file")
	pflag.StringVar(&master, "master", "", "master url")
	pflag.Parse()
}

func main() {
	ctx := context.Background()

	logger := slog.Default()

	if config == "" && kubeconfig == "" {
		logger.Error("config or kubeconfig must be set")
		os.Exit(1)
	}

	var staticConfig []*v1alpha1.ImageSpec
	if config != "" {
		file, err := os.Open(config)
		if err != nil {
			logger.Error("failed to open config file", "err", err)
			os.Exit(1)
		}
		staticConfig, err = loadConfig(file)
		if err != nil {
			logger.Error("failed to load config", "err", err)
			os.Exit(1)
		}
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

	h, err := handler.NewHandler(cache, staticConfig, clientset)
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

func loadConfig(r io.Reader) ([]*v1alpha1.ImageSpec, error) {
	var images []*v1alpha1.ImageSpec
	decoder := yaml.NewYAMLToJSONDecoder(r)
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to decode %q: %w", raw, err)
		}
		if len(raw) == 0 {
			// Ignore empty documents
			continue
		}
		var img v1alpha1.Image
		err = json.Unmarshal(raw, &img)
		if err != nil {
			return nil, err
		}
		if img.TypeMeta.APIVersion != v1alpha1.GroupVersion.String() {
			return nil, fmt.Errorf("unexpected APIVersion %q", img.TypeMeta.APIVersion)
		}
		if img.Kind != v1alpha1.ImageKind {
			return nil, fmt.Errorf("unexpected Kind %q", img.Kind)
		}

		images = append(images, &img.Spec)
	}
	return images, nil
}
