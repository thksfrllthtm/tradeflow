package repository

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// FileStore keeps private user files (source XLSX, extracted images). Paths in
// the database are relative keys like "images/<random>.jpg" and are never
// derived from client filenames. LocalFileStore keeps them on disk (Docker
// Compose), SupabaseFileStore in a private Supabase Storage bucket.
type FileStore interface {
	Save(reader io.Reader, category, extension string) (string, int64, error)
	Open(relative string) (StoredFile, error)
	Remove(relative string) error
}

// StoredFile is an opened private file: XLSX parsing needs random access and
// the size, image responses need a plain reader.
type StoredFile interface {
	io.ReadSeekCloser
	io.ReaderAt
	Size() int64
}

type LocalFileStore struct {
	Root string
}

func NewFileStore(root string) (*LocalFileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("file storage directory is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absRoot, 0o750); err != nil {
		return nil, err
	}
	return &LocalFileStore{Root: absRoot}, nil
}

func (s *LocalFileStore) Save(reader io.Reader, category, extension string) (string, int64, error) {
	if s == nil {
		return "", 0, fmt.Errorf("file storage is not configured")
	}
	relative, err := newStoredPath(category, extension)
	if err != nil {
		return "", 0, err
	}
	fullPath, err := s.resolve(relative)
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return "", 0, err
	}
	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	size, copyErr := io.Copy(file, reader)
	if copyErr != nil {
		_ = os.Remove(fullPath)
		return "", 0, copyErr
	}
	return relative, size, nil
}

func (s *LocalFileStore) Open(relative string) (StoredFile, error) {
	fullPath, err := s.resolve(relative)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	return localFile{File: file, size: info.Size()}, nil
}

func (s *LocalFileStore) Remove(relative string) error {
	fullPath, err := s.resolve(relative)
	if err != nil {
		return err
	}
	return os.Remove(fullPath)
}

func (s *LocalFileStore) resolve(relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("absolute file path is not allowed")
	}
	fullPath := filepath.Clean(filepath.Join(s.Root, filepath.FromSlash(relative)))
	rel, err := filepath.Rel(s.Root, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file path is outside storage")
	}
	return fullPath, nil
}

type localFile struct {
	*os.File
	size int64
}

func (f localFile) Size() int64 { return f.size }

func newStoredPath(category, extension string) (string, error) {
	extension = strings.ToLower(strings.TrimSpace(extension))
	if extension != "" && !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	name, err := randomName()
	if err != nil {
		return "", err
	}
	return path.Join(category, name+extension), nil
}

// cleanStoredPath validates a relative key read from the database before it is
// used as an object name.
func cleanStoredPath(relative string) (string, error) {
	relative = strings.ReplaceAll(relative, "\\", "/")
	cleaned := path.Clean(relative)
	if relative == "" || strings.HasPrefix(relative, "/") || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("file path is outside storage")
	}
	return cleaned, nil
}

func randomName() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", bytes), nil
}
