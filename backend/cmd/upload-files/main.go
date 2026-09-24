// upload-files копирует файлы из локального хранилища (FILE_STORAGE_DIR) в бакет
// Supabase Storage с теми же ключами, что записаны в базе (imports/..., images/...).
// Существующие объекты перезаписываются, поэтому команду можно запускать повторно.
//
//	SUPABASE_URL=... SUPABASE_SECRET_KEY=... go run ./cmd/upload-files -dir ./data/uploads
package main

import (
	"flag"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

func main() {
	dir := flag.String("dir", "./data/uploads", "локальная папка с файлами (FILE_STORAGE_DIR)")
	flag.Parse()

	bucket := strings.TrimSpace(os.Getenv("SUPABASE_STORAGE_BUCKET"))
	if bucket == "" {
		bucket = "uploads"
	}
	store, err := repository.NewSupabaseFileStore(os.Getenv("SUPABASE_URL"), os.Getenv("SUPABASE_SECRET_KEY"), bucket)
	if err != nil {
		log.Fatal(err)
	}

	count := 0
	err = filepath.WalkDir(*dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(*dir, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := store.Put(filepath.ToSlash(relative), contents, true); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		log.Fatalf("uploaded %d files, then failed: %v", count, err)
	}
	log.Printf("uploaded %d files to bucket %q", count, bucket)
}
