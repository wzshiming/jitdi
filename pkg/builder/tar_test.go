package builder

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/wzshiming/jitdi/pkg/atomic"
)

func TestTarWithValidFiles(t *testing.T) {
	content := "Hello"

	file1 := &File{
		Path:    "file.txt",
		Mode:    0644,
		ModTime: time.Unix(1, 1),
		OpenReader: func() (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewReader([]byte(content))), int64(len(content)), nil
		},
	}

	r := Tar(file1)
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	s := atomic.SumSha256(data)
	if s != "96aecd2e52cc8d11fa04e6ede3d6de7bd39f07bb09072be974dd17543c29d341" {
		t.Fatalf("Unexpected hash: %s", s)
	}
}
