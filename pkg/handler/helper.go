package handler

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
)

func ReferenceToURL(ref name.Reference) string {
	repo := ref.Context()
	return fmt.Sprintf("%s://%s/v2/%s/manifests/%s", repo.Scheme(), repo.RegistryStr(), repo.RepositoryStr(), ref.Identifier())
}
