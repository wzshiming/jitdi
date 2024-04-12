package handler

import (
	"io"
	"os"
	"path"
)

func atomicWriteFile(file string, data []byte, perm os.FileMode) error {
	dir := path.Dir(file)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, "tmp-"+path.Base(file))
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return err
	}

	err = f.Chmod(perm)
	if err != nil {
		return err
	}

	return os.Rename(f.Name(), file)
}

func atomicWriteFileStream(file string, perm os.FileMode) (io.WriteCloser, error) {
	dir := path.Dir(file)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}

	f, err := os.CreateTemp(dir, "tmp-"+path.Base(file))
	if err != nil {
		return nil, err
	}
	err = f.Chmod(perm)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &atomicWriteCloser{
		f:    f,
		path: file,
	}, nil
}

type atomicWriteCloser struct {
	f    *os.File
	path string
}

func (a *atomicWriteCloser) Write(p []byte) (n int, err error) {
	return a.f.Write(p)
}

func (a *atomicWriteCloser) Close() error {
	defer a.f.Close()
	return os.Rename(a.f.Name(), a.path)
}
