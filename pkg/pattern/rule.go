package pattern

import (
	"github.com/wzshiming/jitdi/pkg/apis/v1alpha1"
)

type Rule struct {
	match        *pattern
	baseImage    string
	mutates      []v1alpha1.Mutate
	storageImage string
	platforms    []v1alpha1.Platform
}

func NewRule(conf *v1alpha1.ImageSpec) (*Rule, error) {
	pat, err := parsePattern(conf.Match)
	if err != nil {
		return nil, err
	}
	return &Rule{
		match:        pat,
		baseImage:    conf.BaseImage,
		mutates:      conf.Mutates,
		storageImage: conf.StorageImage,
		platforms:    conf.Platforms,
	}, nil
}

func (r *Rule) Match(image string) (*Action, bool) {
	params, ok := r.match.Match(image)
	if !ok {
		return nil, false
	}

	return &Action{
		params: params,
		match:  image,
		rule:   r,
	}, true
}

func (r *Rule) LessThan(o *Rule) bool {
	return patternLess(r.match, o.match)
}
