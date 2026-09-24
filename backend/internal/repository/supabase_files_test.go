package repository

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeStorage imitates the parts of the Supabase Storage API the store uses.
type fakeStorage struct {
	mu      sync.Mutex
	buckets map[string]bool
	objects map[string][]byte
	headers []http.Header
}

func (f *fakeStorage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.headers = append(f.headers, r.Header.Clone())
	path := strings.TrimPrefix(r.URL.Path, "/storage/v1")
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/bucket/"):
		if !f.buckets[strings.TrimPrefix(path, "/bucket/")] {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		}
	case r.Method == http.MethodPost && path == "/bucket":
		var body struct {
			ID     string `json:"id"`
			Public bool   `json:"public"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Public {
			http.Error(w, "bucket must be private", http.StatusBadRequest)
			return
		}
		f.buckets[body.ID] = true
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/object/"):
		key := strings.TrimPrefix(path, "/object/")
		if _, exists := f.objects[key]; exists && r.Header.Get("x-upsert") != "true" {
			http.Error(w, "duplicate", http.StatusConflict)
			return
		}
		f.objects[key], _ = io.ReadAll(r.Body)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/object/"):
		contents, ok := f.objects[strings.TrimPrefix(path, "/object/")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write(contents)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/object/"):
		bucket := strings.TrimPrefix(path, "/object/")
		var body struct {
			Prefixes []string `json:"prefixes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, prefix := range body.Prefixes {
			delete(f.objects, bucket+"/"+prefix)
		}
		_, _ = w.Write([]byte("[]"))
	default:
		http.Error(w, "unexpected "+r.Method+" "+path, http.StatusTeapot)
	}
}

func newFakeStorage(t *testing.T) (*fakeStorage, *httptest.Server) {
	t.Helper()
	fake := &fakeStorage{buckets: map[string]bool{}, objects: map[string][]byte{}}
	server := httptest.NewTLSServer(fake)
	t.Cleanup(server.Close)
	return fake, server
}

func TestSupabaseFileStoreRoundTrip(t *testing.T) {
	fake, server := newFakeStorage(t)
	store, err := newSupabaseFileStore(server.URL, "sb_secret_test", "uploads", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if !fake.buckets["uploads"] {
		t.Fatal("missing bucket must be created")
	}

	key, size, err := store.Save(strings.NewReader("hello"), "images", "JPG")
	if err != nil {
		t.Fatal(err)
	}
	if size != 5 || !strings.HasPrefix(key, "images/") || !strings.HasSuffix(key, ".jpg") {
		t.Fatalf("unexpected key %q size %d", key, size)
	}
	file, err := store.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	contents, _ := io.ReadAll(file)
	if string(contents) != "hello" || file.Size() != 5 {
		t.Fatalf("read back %q (size %d)", contents, file.Size())
	}
	if err := store.Remove(key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(key); err == nil {
		t.Fatal("removed file must not open")
	}

	for _, header := range fake.headers {
		if header.Get("apikey") != "sb_secret_test" || header.Get("Authorization") != "" {
			t.Fatalf("new secret keys go only in the apikey header, got %v", header)
		}
	}
}

func TestSupabaseFileStoreLegacyKeyAndUpsert(t *testing.T) {
	fake, server := newFakeStorage(t)
	fake.buckets["uploads"] = true
	store, err := newSupabaseFileStore(server.URL, "eyJlegacy", "uploads", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("imports/a.xlsx", []byte("1"), false); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("imports/a.xlsx", []byte("2"), false); err == nil {
		t.Fatal("upload without upsert must not overwrite")
	}
	if err := store.Put("imports/a.xlsx", []byte("2"), true); err != nil {
		t.Fatal(err)
	}
	if string(fake.objects["uploads/imports/a.xlsx"]) != "2" {
		t.Fatal("upsert must overwrite")
	}
	if got := fake.headers[len(fake.headers)-1].Get("Authorization"); got != "Bearer eyJlegacy" {
		t.Fatalf("legacy JWT key must also be sent as bearer token, got %q", got)
	}
}

func TestSupabaseFileStoreRejectsUnsafePaths(t *testing.T) {
	_, server := newFakeStorage(t)
	store, err := newSupabaseFileStore(server.URL, "sb_secret_test", "uploads", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "/etc/passwd", "../other/x", "images/../../x"} {
		if _, err := store.Open(key); err == nil || !strings.Contains(err.Error(), "outside storage") {
			t.Fatalf("key %q must be rejected, got %v", key, err)
		}
	}
}
