package repository

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// SupabaseFileStore keeps private files in a Supabase Storage bucket through its
// REST API. Files are small enough (XLSX up to 50 MB, images up to 5 MB) to be
// buffered in memory, which also gives XLSX parsing the random access it needs.
type SupabaseFileStore struct {
	baseURL string // https://<ref>.supabase.co/storage/v1
	bucket  string
	key     string
	client  *http.Client
}

// NewSupabaseFileStore connects to the bucket and creates it as a private bucket
// if it does not exist yet.
func NewSupabaseFileStore(projectURL, secretKey, bucket string) (*SupabaseFileStore, error) {
	return newSupabaseFileStore(projectURL, secretKey, bucket, &http.Client{Timeout: 2 * time.Minute})
}

func newSupabaseFileStore(projectURL, secretKey, bucket string, client *http.Client) (*SupabaseFileStore, error) {
	projectURL = strings.TrimRight(strings.TrimSpace(projectURL), "/")
	parsed, err := url.ParseRequestURI(projectURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return nil, fmt.Errorf("SUPABASE_URL must look like https://<project>.supabase.co")
	}
	if strings.TrimSpace(secretKey) == "" {
		return nil, fmt.Errorf("SUPABASE_SECRET_KEY is required when SUPABASE_URL is set")
	}
	if strings.TrimSpace(bucket) == "" || strings.ContainsAny(bucket, "/?#") {
		return nil, fmt.Errorf("invalid storage bucket name %q", bucket)
	}
	store := &SupabaseFileStore{
		baseURL: projectURL + "/storage/v1",
		bucket:  bucket,
		key:     strings.TrimSpace(secretKey),
		client:  client,
	}
	if err := store.ensureBucket(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SupabaseFileStore) Save(reader io.Reader, category, extension string) (string, int64, error) {
	relative, err := newStoredPath(category, extension)
	if err != nil {
		return "", 0, err
	}
	contents, err := io.ReadAll(reader)
	if err != nil {
		return "", 0, err
	}
	if err := s.Put(relative, contents, false); err != nil {
		return "", 0, err
	}
	return relative, int64(len(contents)), nil
}

// Put uploads contents under an exact key. upsert=true overwrites an existing
// object; it is used when copying existing local files into the bucket.
func (s *SupabaseFileStore) Put(relative string, contents []byte, upsert bool) error {
	key, err := cleanStoredPath(relative)
	if err != nil {
		return err
	}
	contentType := mime.TypeByExtension(path.Ext(key))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	request, err := s.newRequest(http.MethodPost, s.objectURL(key), bytes.NewReader(contents))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("x-upsert", fmt.Sprint(upsert))
	return s.do(request, nil, "upload "+key)
}

func (s *SupabaseFileStore) Open(relative string) (StoredFile, error) {
	key, err := cleanStoredPath(relative)
	if err != nil {
		return nil, err
	}
	request, err := s.newRequest(http.MethodGet, s.objectURL(key), nil)
	if err != nil {
		return nil, err
	}
	var contents []byte
	if err := s.do(request, &contents, "download "+key); err != nil {
		return nil, err
	}
	return memoryFile{Reader: bytes.NewReader(contents)}, nil
}

func (s *SupabaseFileStore) Remove(relative string) error {
	key, err := cleanStoredPath(relative)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string][]string{"prefixes": {key}})
	request, err := s.newRequest(http.MethodDelete, s.baseURL+"/object/"+url.PathEscape(s.bucket), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return s.do(request, nil, "delete "+key)
}

func (s *SupabaseFileStore) ensureBucket() error {
	request, err := s.newRequest(http.MethodGet, s.baseURL+"/bucket/"+url.PathEscape(s.bucket), nil)
	if err != nil {
		return err
	}
	if err := s.do(request, nil, "check bucket "+s.bucket); err == nil {
		return nil
	}
	body, _ := json.Marshal(map[string]any{"id": s.bucket, "name": s.bucket, "public": false})
	request, err = s.newRequest(http.MethodPost, s.baseURL+"/bucket", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return s.do(request, nil, "create bucket "+s.bucket)
}

func (s *SupabaseFileStore) objectURL(key string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return s.baseURL + "/object/" + url.PathEscape(s.bucket) + "/" + strings.Join(segments, "/")
}

func (s *SupabaseFileStore) newRequest(method, target string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequest(method, target, body)
	if err != nil {
		return nil, err
	}
	// New secret keys (sb_secret_...) go only in the apikey header; legacy
	// service_role JWT keys are also expected as a bearer token.
	request.Header.Set("apikey", s.key)
	if !strings.HasPrefix(s.key, "sb_") {
		request.Header.Set("Authorization", "Bearer "+s.key)
	}
	return request, nil
}

func (s *SupabaseFileStore) do(request *http.Request, contents *[]byte, action string) error {
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("supabase storage %s: %w", action, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("supabase storage %s: %w", action, err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		const maxMessage = 300
		message := strings.TrimSpace(string(body))
		if len(message) > maxMessage {
			message = message[:maxMessage]
		}
		return fmt.Errorf("supabase storage %s: HTTP %d: %s", action, response.StatusCode, message)
	}
	if contents != nil {
		*contents = body
	}
	return nil
}

type memoryFile struct {
	*bytes.Reader
}

func (memoryFile) Close() error { return nil }
