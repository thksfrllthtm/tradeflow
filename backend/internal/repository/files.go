package repository

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileStore keeps user files outside the frontend's public directory. Paths in
// the database are relative to Root and are never derived from client filenames.
type FileStore struct {
	Root string
}

func NewFileStore(root string) (*FileStore, error) {
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
	return &FileStore{Root: absRoot}, nil
}

func (s *FileStore) Save(reader io.Reader, category, extension string) (string, int64, error) {
	if s == nil {
		return "", 0, fmt.Errorf("file storage is not configured")
	}
	extension = strings.ToLower(strings.TrimSpace(extension))
	if extension != "" && !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	name, err := randomName()
	if err != nil {
		return "", 0, err
	}
	relative := filepath.Join(category, name+extension)
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
	return filepath.ToSlash(relative), size, nil
}

func (s *FileStore) Open(relative string) (*os.File, error) {
	fullPath, err := s.resolve(relative)
	if err != nil {
		return nil, err
	}
	return os.Open(fullPath)
}

func (s *FileStore) Remove(relative string) error {
	fullPath, err := s.resolve(relative)
	if err != nil {
		return err
	}
	return os.Remove(fullPath)
}

func (s *FileStore) resolve(relative string) (string, error) {
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

func randomName() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", bytes), nil
}
