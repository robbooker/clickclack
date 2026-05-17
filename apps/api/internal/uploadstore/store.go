package uploadstore

import (
	"context"
	"errors"
	"io"
	"net/http"
)

var ErrNotFound = errors.New("upload object not found")

type Object struct {
	Path        string
	Filename    string
	ContentType string
	ByteSize    int64
}

type SaveOptions struct {
	ContentType string
}

type SavedObject struct {
	Path     string
	ByteSize int64
}

type Store interface {
	Save(ctx context.Context, body io.Reader, options SaveOptions) (SavedObject, error)
	Delete(ctx context.Context, path string) error
	ServeHTTP(w http.ResponseWriter, r *http.Request, object Object) error
}
