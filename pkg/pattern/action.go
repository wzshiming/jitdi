package pattern

import (
	"strings"

	"github.com/google/go-containerregistry/pkg/v1"

	"github.com/wzshiming/jitdi/pkg/apis/v1alpha1"
)

type Action struct {
	params map[string]string
	rule   *Rule
}

func (r *Action) GetBaseImage() string {
	return replaceWithParams(r.rule.baseImage, r.params)
}

func (r *Action) GetMutates(p *v1.Platform) []v1alpha1.Mutate {
	mutates := r.rule.mutates
	params := r.params
	if p == nil {
		params["GOOS"] = "linux"
		params["GOARCH"] = "amd64"
	} else {
		params["GOOS"] = p.OS
		params["GOARCH"] = p.Architecture
	}
	return replaceMutateWithParams(mutates, r.params)
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
					Mode:        v.File.Mode,
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
