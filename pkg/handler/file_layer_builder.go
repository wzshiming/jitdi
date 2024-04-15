package handler

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/wzshiming/jitdi/pkg/atomic"
)

type FileLayerBuilder struct {
	mode      int64
	modTime   time.Time
	tmpPath   string
	mediaType types.MediaType
}

func NewFileLayerBuilder(tmpPath string, mode int64, modTime time.Time, mediaType types.MediaType) *FileLayerBuilder {
	return &FileLayerBuilder{
		mode:      mode,
		modTime:   modTime,
		tmpPath:   tmpPath,
		mediaType: mediaType,
	}
}

func (f *FileLayerBuilder) Build(hostPath, newPath string) ([]mutate.Addendum, error) {
	tmp, err := os.CreateTemp(f.tmpPath, "tmp-")
	if err != nil {
		return nil, err
	}

	sum := sha256.New()

	tw := tar.NewWriter(io.MultiWriter(tmp, sum))

	err = f.tarAny(tw, hostPath, newPath)
	if err != nil {
		tw.Close()
		tmp.Close()
		return nil, err
	}
	tw.Close()
	tmp.Close()

	hash := hex.EncodeToString(sum.Sum(nil))
	cachePath := path.Join(f.tmpPath, "sha256:"+hash)
	err = os.Rename(tmp.Name(), cachePath)
	if err != nil {
		return nil, err
	}

	dataLayer, err := tarball.LayerFromFile(cachePath,
		tarball.WithMediaType(f.mediaType),
		tarball.WithCompressionLevel(0),
	)
	if err != nil {
		return nil, fmt.Errorf("toLayer: %w", err)
	}

	return []mutate.Addendum{
		{
			Layer: dataLayer,
			History: v1.History{
				Author:    "jitdi",
				CreatedBy: fmt.Sprintf("COPY %s %s", hostPath, newPath),
				Comment:   fmt.Sprintf("Copy %s to %s", hostPath, newPath),
			},
		},
	}, nil
}

func (f *FileLayerBuilder) BuildFile(file io.Reader, newPath string, size int64) ([]mutate.Addendum, error) {
	tmp, err := os.CreateTemp(f.tmpPath, "tmp-")
	if err != nil {
		return nil, err
	}

	sum := sha256.New()

	tw := tar.NewWriter(io.MultiWriter(tmp, sum))

	err = f.tarFile(tw, file, newPath, size)
	if err != nil {
		tw.Close()
		tmp.Close()
		return nil, err
	}
	tw.Close()
	tmp.Close()

	cachePath := path.Join(f.tmpPath, "sha256:"+hex.EncodeToString(sum.Sum(nil)))
	err = os.Rename(tmp.Name(), cachePath)
	if err != nil {
		return nil, err
	}

	dataLayer, err := tarball.LayerFromFile(cachePath,
		tarball.WithMediaType(f.mediaType),
		tarball.WithCompressionLevel(0),
	)
	if err != nil {
		return nil, fmt.Errorf("toLayer: %w", err)
	}

	return []mutate.Addendum{
		{
			Layer: dataLayer,
			History: v1.History{
				Author:    "jitdi",
				CreatedBy: fmt.Sprintf("ADD %s", newPath),
				Comment:   fmt.Sprintf("Add %s", newPath),
			},
		},
	}, nil
}

func (f *FileLayerBuilder) tarAny(tw *tar.Writer, hostPath, newPath string) error {
	u, err := url.Parse(hostPath)
	if err == nil {
		switch u.Scheme {
		case "http", "https":
			return f.tarRemote(tw, u, newPath)
		}
	}

	return f.tarLocal(tw, hostPath, newPath)
}

func (f *FileLayerBuilder) tarRemote(tw *tar.Writer, u *url.URL, newPath string) error {
	if strings.HasSuffix(newPath, "/") {
		return f.tarRemoteFileInDir(tw, u, newPath)
	}
	return f.tarRemoteFileToFile(tw, u, newPath)
}

func (f *FileLayerBuilder) tarRemoteFileToFile(tw *tar.Writer, u *url.URL, newPath string) error {
	srcPath := path.Join(f.tmpPath, u.Scheme, u.Path)
	stat, _ := os.Stat(srcPath)
	if stat == nil {
		resp, err := http.Get(u.String())
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("http.Get(%q): %w", u.String(), fmt.Errorf("status code %d", resp.StatusCode))
		}

		err = atomic.WriteFileWithReader(srcPath, resp.Body, 0644)
		if err != nil {
			return err
		}

		stat, err = os.Stat(srcPath)
		if err != nil {
			return err
		}
	}

	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return f.tarFile(tw, file, newPath, stat.Size())
}

func (f *FileLayerBuilder) tarRemoteFileInDir(tw *tar.Writer, u *url.URL, dir string) error {
	return f.tarRemoteFileToFile(tw, u, path.Join(dir, path.Base(u.Path)))
}

func (f *FileLayerBuilder) tarLocal(tw *tar.Writer, hostPath, newPath string) error {
	info, err := os.Stat(hostPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return f.tarDirToDir(tw, hostPath, newPath)
	}

	if strings.HasSuffix(newPath, "/") {
		return f.tarFileInDir(tw, hostPath, newPath, info)
	}
	return f.tarFileToFile(tw, hostPath, newPath, info)
}

func (f *FileLayerBuilder) tarDirToDir(tw *tar.Writer, hostPath, newPath string) error {
	return filepath.Walk(hostPath, func(p string, info os.FileInfo, err error) error {
		if hostPath == p {
			return nil
		}
		if err != nil {
			return fmt.Errorf("filepath.Walk(%q): %w", hostPath, err)
		}

		if info.IsDir() {
			return f.tarDirToDir(tw, p, path.Join(newPath, path.Base(p)))
		}

		return f.tarFileToFile(tw, p, path.Join(newPath, path.Base(p)), info)
	})
}

func (f *FileLayerBuilder) tarFileToFile(tw *tar.Writer, hostPath, newPath string, info os.FileInfo) error {
	file, err := os.Open(hostPath)
	if err != nil {
		return fmt.Errorf("os.Open(%q): %w", hostPath, err)
	}
	defer file.Close()

	size := info.Size()
	return f.tarFile(tw, file, newPath, size)
}

func (f *FileLayerBuilder) tarFile(tw *tar.Writer, reader io.Reader, newPath string, size int64) error {
	header := &tar.Header{
		Name:     newPath,
		Size:     size,
		Typeflag: tar.TypeReg,
		Mode:     f.mode,
		ModTime:  f.modTime,
	}
	err := tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("tar.Writer.WriteHeader(%q): %w", newPath, err)
	}
	n, err := io.Copy(tw, reader)
	if err != nil {
		return fmt.Errorf("io.Copy(%q, %q): %w", newPath, reader, err)
	}

	if n != size {
		return fmt.Errorf("io.Copy(%q, %q): short write: %d != %d", newPath, reader, n, size)
	}
	return nil
}

func (f *FileLayerBuilder) tarFileInDir(tw *tar.Writer, hostPath, dir string, info os.FileInfo) error {
	return f.tarFileToFile(tw, hostPath, path.Join(dir, path.Base(hostPath)), info)
}
