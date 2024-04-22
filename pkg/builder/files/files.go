package files

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/wzshiming/jitdi/pkg/builder"
)

type Files struct {
	mode      int64
	modTime   time.Time
	client    *http.Client
	transport http.RoundTripper
}

func NewFiles(mode int64, modTime time.Time, transport http.RoundTripper) *Files {
	return &Files{
		mode:    mode,
		modTime: modTime,
		client: &http.Client{
			Transport: transport,
		},
	}
}

func (f *Files) Build(hostPath, newPath string) ([]*builder.File, error) {
	return f.tarAny(hostPath, newPath)
}

func (f *Files) tarAny(hostPath, newPath string) ([]*builder.File, error) {
	u, err := url.Parse(hostPath)
	if err == nil {
		switch u.Scheme {
		case "http", "https":
			file, err := f.tarRemote(u, newPath)
			if err != nil {
				return nil, err
			}
			return []*builder.File{file}, nil
		}
	}
	return f.tarLocal(hostPath, newPath)
}

func (f *Files) tarRemote(u *url.URL, newPath string) (*builder.File, error) {
	if strings.HasSuffix(newPath, "/") {
		return f.tarRemoteFileInDir(u, newPath)
	}
	return f.tarRemoteFileToFile(u, newPath)
}

func (f *Files) tarRemoteFileToFile(u *url.URL, newPath string) (*builder.File, error) {
	uri := u.String()
	resp, err := http.Head(uri)
	if err != nil {
		return nil, fmt.Errorf("http.Head(%q): %w", uri, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http.Head(%q): %w", uri, fmt.Errorf("status code %d", resp.StatusCode))
	}

	return &builder.File{
		Path:    newPath,
		Mode:    f.mode,
		ModTime: f.modTime,
		OpenReader: func() (io.ReadCloser, int64, error) {
			req, err := http.NewRequest(http.MethodGet, uri, nil)
			if err != nil {
				return nil, 0, fmt.Errorf("http.NewRequest(%q): %w", uri, err)
			}
			resp, err := f.client.Do(req)
			if err != nil {
				return nil, 0, fmt.Errorf("http.Get(%q): %w", u.String(), err)
			}
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				return nil, 0, fmt.Errorf("http.Get(%q): %w", u.String(), fmt.Errorf("status code %d", resp.StatusCode))
			}
			if resp.ContentLength <= 0 {
				_ = resp.Body.Close()
				return nil, 0, fmt.Errorf("http.Get(%q): %w", u.String(), fmt.Errorf("content length is unknown"))
			}
			return resp.Body, resp.ContentLength, nil
		},
	}, nil
}

func (f *Files) tarRemoteFileInDir(u *url.URL, dir string) (*builder.File, error) {
	return f.tarRemoteFileToFile(u, path.Join(dir, path.Base(u.Path)))
}

func (f *Files) tarLocal(hostPath, newPath string) ([]*builder.File, error) {
	info, err := os.Stat(hostPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return f.tarDirToDir(hostPath, newPath)
	}

	if strings.HasSuffix(newPath, "/") {
		return []*builder.File{f.tarFileInDir(hostPath, newPath)}, nil
	}
	return []*builder.File{f.tarFileToFile(hostPath, newPath)}, nil
}

func (f *Files) tarDirToDir(hostPath, newPath string) ([]*builder.File, error) {
	fs := []*builder.File{}
	err := filepath.Walk(hostPath, func(p string, info os.FileInfo, err error) error {
		if hostPath == p {
			return nil
		}
		if err != nil {
			return fmt.Errorf("filepath.Walk(%q): %w", hostPath, err)
		}

		if info.IsDir() {
			files, err := f.tarDirToDir(p, path.Join(newPath, path.Base(p)))
			if err != nil {
				return err
			}
			fs = append(fs, files...)
		} else {
			file := f.tarFileToFile(p, path.Join(newPath, path.Base(p)))
			fs = append(fs, file)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return fs, nil
}

func (f *Files) tarFileToFile(hostPath, newPath string) *builder.File {
	return &builder.File{
		Path:    newPath,
		Mode:    f.mode,
		ModTime: f.modTime,
		OpenReader: func() (io.ReadCloser, int64, error) {
			file, err := os.Open(hostPath)
			if err != nil {
				return nil, 0, fmt.Errorf("os.Open(%q): %w", hostPath, err)
			}

			info, err := file.Stat()
			if err != nil {
				return nil, 0, fmt.Errorf("file.Stat(%q): %w", hostPath, err)
			}
			size := info.Size()
			return file, size, nil
		},
	}
}

func (f *Files) tarFileInDir(hostPath, dir string) *builder.File {
	return f.tarFileToFile(hostPath, path.Join(dir, path.Base(hostPath)))
}
