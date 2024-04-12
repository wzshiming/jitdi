package handler

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type FileLayerBuilder struct {
	mode    int64
	modTime time.Time
	tmpPath string
}

func NewFileLayerBuilder(tmpPath string, mode int64, modTime time.Time) *FileLayerBuilder {
	return &FileLayerBuilder{
		mode:    mode,
		modTime: modTime,
		tmpPath: tmpPath,
	}
}

func (f *FileLayerBuilder) Build(hostPath, newPath string) (string, error) {
	tmp, err := os.CreateTemp(f.tmpPath, "tmp-")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	sum := sha256.New()

	tw := tar.NewWriter(io.MultiWriter(tmp, sum))
	defer tw.Close()

	err = f.tarAny(tw, hostPath, newPath)
	if err != nil {
		return "", err
	}

	p := path.Join(f.tmpPath, fmt.Sprintf("sha256:%x", sum.Sum(nil)))
	err = os.Rename(tmp.Name(), p)
	if err != nil {
		return "", err
	}
	return p, nil
}

func (f *FileLayerBuilder) tarAny(tw *tar.Writer, hostPath, newPath string) error {
	u, err := url.Parse(hostPath)
	if err == nil {
		switch u.Scheme {
		case "http", "https":
			return f.tarRemote(tw, hostPath, newPath)
		}
	}

	return f.tarLocal(tw, hostPath, newPath)
}

func (f *FileLayerBuilder) tarRemote(tw *tar.Writer, hostPath, newPath string) error {
	if strings.HasSuffix(newPath, "/") {
		return f.tarRemoteFileInDir(tw, hostPath, newPath)
	}
	return f.tarRemoteFileToFile(tw, hostPath, newPath)
}

func (f *FileLayerBuilder) tarRemoteFileToFile(tw *tar.Writer, hostPath, newPath string) error {
	resp, err := http.Get(hostPath)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http.Get(%q): %w", hostPath, fmt.Errorf("status code %d", resp.StatusCode))
	}

	var modTime = f.modTime
	modTimeStr := resp.Header.Get("Last-Modified")
	if modTimeStr != "" {
		t, err := time.Parse(time.RFC1123, modTimeStr)
		if err == nil {
			modTime = t
		}
	}
	header := &tar.Header{
		Name:     newPath,
		Size:     resp.ContentLength,
		Typeflag: tar.TypeReg,
		Mode:     f.mode,
		ModTime:  modTime,
	}
	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("tar.Writer.WriteHeader(%q): %w", newPath, err)
	}
	_, err = io.Copy(tw, resp.Body)
	if err != nil {
		return fmt.Errorf("io.Copy(%q, %q): %w", newPath, hostPath, err)
	}
	return nil
}

func (f *FileLayerBuilder) tarRemoteFileInDir(tw *tar.Writer, hostPath, dir string) error {
	return f.tarRemoteFileToFile(tw, hostPath, path.Join(dir, path.Base(hostPath)))
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
	header := &tar.Header{
		Name:     newPath,
		Size:     info.Size(),
		Typeflag: tar.TypeReg,
		Mode:     int64(info.Mode()),
		ModTime:  info.ModTime(),
	}
	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("tar.Writer.WriteHeader(%q): %w", newPath, err)
	}
	n, err := io.Copy(tw, file)
	if err != nil {
		return fmt.Errorf("io.Copy(%q, %q): %w", newPath, hostPath, err)
	}

	if n != size {
		return fmt.Errorf("io.Copy(%q, %q): short write: %d != %d", newPath, hostPath, n, size)
	}
	return nil
}

func (f *FileLayerBuilder) tarFileInDir(tw *tar.Writer, hostPath, dir string, info os.FileInfo) error {
	return f.tarFileToFile(tw, hostPath, path.Join(dir, path.Base(hostPath)), info)
}
