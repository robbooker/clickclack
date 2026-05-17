package uploadstore

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type Local struct {
	dir string
}

func NewLocal(dir string) *Local {
	return &Local{dir: dir}
}

func (s *Local) Save(_ context.Context, body io.Reader, _ SaveOptions) (SavedObject, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return SavedObject{}, err
	}
	tmp, err := os.CreateTemp(s.dir, "upload-*")
	if err != nil {
		return SavedObject{}, err
	}
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmp.Name())
		}
	}()
	size, err := io.Copy(tmp, body)
	if err != nil {
		return SavedObject{}, err
	}
	if err := tmp.Close(); err != nil {
		return SavedObject{}, err
	}
	committed = true
	return SavedObject{Path: tmp.Name(), ByteSize: size}, nil
}

func (s *Local) Delete(_ context.Context, path string) error {
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.dir, path)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Local) ServeHTTP(w http.ResponseWriter, r *http.Request, object Object) error {
	if object.Path == "" {
		return ErrNotFound
	}
	http.ServeFile(w, r, object.Path)
	return nil
}
