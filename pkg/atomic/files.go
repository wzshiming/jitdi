package atomic

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
)

func WriteFile(file string, data []byte, perm os.FileMode) error {
	return WriteFileWithReader(file, bytes.NewReader(data), perm)
}

func WriteFileWithReader(file string, r io.Reader, perm os.FileMode) error {
	f, err := OpenFileWithWriter(file, perm)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	if err != nil {
		return err
	}

	return nil
}

func OpenFileWithWriter(file string, perm os.FileMode) (*WriteCloser, error) {
	dir := path.Dir(file)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}

	f, err := os.CreateTemp(dir, "tmp-"+path.Base(file)+"-")
	if err != nil {
		return nil, err
	}
	err = f.Chmod(perm)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &WriteCloser{
		f:    f,
		path: file,
	}, nil
}

type WriteCloser struct {
	f    *os.File
	path string
}

func (a *WriteCloser) Write(p []byte) (n int, err error) {
	return a.f.Write(p)
}

func (a *WriteCloser) Close() error {
	_ = a.f.Close()
	return os.Rename(a.f.Name(), a.path)
}

func (a *WriteCloser) Abort() error {
	_ = a.f.Close()
	return os.Remove(a.f.Name())
}

func SumSha256(blob []byte) string {
	sum256 := sha256.Sum256(blob)
	return hex.EncodeToString(sum256[:])
}
