package builder

import (
	"archive/tar"
	"fmt"
	"io"
	"time"
)

func Tar(fs ...*File) io.ReadCloser {
	return &lazyTar{
		fs: fs,
	}
}

type lazyTar struct {
	rc io.ReadCloser
	fs []*File
}

func (l *lazyTar) Read(p []byte) (n int, err error) {
	if l.rc == nil {
		l.rc = startTar(l.fs)
	}

	return l.rc.Read(p)
}

func (l *lazyTar) Close() error {
	if l.rc == nil {
		return nil
	}

	return l.rc.Close()
}

func startTar(fs []*File) io.ReadCloser {
	r, w := io.Pipe()
	tw := tar.NewWriter(w)
	go func() {
		var err error

		defer func() {
			if err != nil {
				_ = w.CloseWithError(err)

				// Creating tar is wrong, so don't care about the error of closing the tar.
				_ = tw.Close()
			} else {
				err = tw.Close()
				_ = w.CloseWithError(err)
			}
		}()

		for _, f := range fs {
			err = tarFile(tw, f)
			if err != nil {
				return
			}
		}
	}()
	return r
}

type File struct {
	Path       string
	Mode       int64
	ModTime    time.Time
	OpenReader func() (io.ReadCloser, int64, error)
}

func tarFile(tw *tar.Writer, f *File) (err error) {
	r, size, err := f.OpenReader()
	if err != nil {
		return fmt.Errorf("f.OpenReader(): %w", err)
	}
	defer func() {
		if err != nil {
			_ = r.Close()
		} else {
			err = r.Close()
		}
	}()

	header := &tar.Header{
		Name:     f.Path,
		Size:     size,
		Typeflag: tar.TypeReg,
		Mode:     f.Mode,
		ModTime:  f.ModTime,
	}
	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("tar.Writer.WriteHeader(%q): %w", f.Path, err)
	}

	n, err := io.Copy(tw, r)
	if err != nil {
		return fmt.Errorf("io.Copy(%q, %q): %w", f.Path, r, err)
	}

	if n != size {
		return fmt.Errorf("io.Copy(%q, %q): short write: %d != %d", f.Path, r, n, size)
	}

	err = tw.Flush()
	if err != nil {
		return fmt.Errorf("tar.Writer.Flush(): %w", err)
	}

	return nil
}
