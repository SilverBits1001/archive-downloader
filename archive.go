package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type FileEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	// Display is the pretty name (arcade mapping); empty means use Name.
	Display string `json:"display,omitempty"`
}

func (f *FileEntry) Shown() string {
	if f.Display != "" {
		return f.Display
	}
	return f.Name
}

// metadataFile matches archive.org's /metadata/<id> files[] entries.
// size can arrive as a string or a number, so it needs a custom type.
type flexInt64 int64

func (v *flexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*v = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		*v = 0
		return nil
	}
	*v = flexInt64(n)
	return nil
}

type metadataFile struct {
	Name string    `json:"name"`
	Size flexInt64 `json:"size"`
}

type metadataResponse struct {
	Files []metadataFile `json:"files"`
}

var (
	httpClient   *http.Client
	insecureMode bool // set when TLS verification fails (dead RTC / wrong clock)
)

func initHTTP(caPath string) {
	pool := x509.NewCertPool()
	if pem, err := os.ReadFile(caPath); err == nil {
		pool.AppendCertsFromPEM(pem)
	}
	httpClient = &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}
}

func enableInsecureMode() {
	insecureMode = true
	httpClient = &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func isCertError(err error) bool {
	if err == nil {
		return false
	}
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return true
	}
	var unknownAuth x509.UnknownAuthorityError
	var invalidCert x509.CertificateInvalidError
	return errors.As(err, &unknownAuth) || errors.As(err, &invalidCert)
}

// checkNetwork returns nil when archive.org is reachable. On a cert
// failure (usually a wrong device clock) it flips to insecure mode and
// still returns nil, but sets insecureMode so the UI can warn once.
func checkNetwork() error {
	req, _ := http.NewRequest(http.MethodHead, "https://archive.org", nil)
	_, err := httpClient.Do(req)
	if err == nil {
		return nil
	}
	if isCertError(err) {
		enableInsecureMode()
		if _, err2 := httpClient.Do(req); err2 == nil {
			logf("TLS verify failed; continuing without verification (check device clock)")
			return nil
		}
	}
	return err
}

// cleanIdentifier accepts a bare identifier or a full archive.org URL.
func cleanIdentifier(raw string) string {
	s := strings.TrimSpace(raw)
	for _, marker := range []string{"archive.org/details/", "archive.org/download/"} {
		if i := strings.Index(s, marker); i >= 0 {
			s = s[i+len(marker):]
		}
	}
	if i := strings.IndexAny(s, "/ \t"); i >= 0 {
		s = s[:i]
	}
	return s
}

func isJunkFile(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	if strings.Contains(base, "__ia_") || strings.Contains(base, "_thumb") {
		return true
	}
	for _, suf := range []string{"_meta.xml", "_files.xml", "_meta.sqlite", "_files.sqlite"} {
		if strings.HasSuffix(base, suf) {
			return true
		}
	}
	return false
}

type httpStatusError struct{ Code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.Code) }

// fetchMetadata downloads and parses the file list for an item.
func fetchMetadata(id string, headers map[string]string) ([]FileEntry, error) {
	req, err := http.NewRequest(http.MethodGet, "https://archive.org/metadata/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{Code: resp.StatusCode}
	}
	var meta metadataResponse
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}
	var out []FileEntry
	for _, f := range meta.Files {
		if f.Name == "" || isJunkFile(f.Name) {
			continue
		}
		out = append(out, FileEntry{Name: f.Name, Size: int64(f.Size)})
	}
	if len(out) == 0 {
		return nil, errors.New("item has no downloadable files")
	}
	return out, nil
}

// downloadURL builds the file URL, encoding each path segment but
// keeping "/" so files inside item subfolders resolve.
func downloadURL(id, name string) string {
	segs := strings.Split(name, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return "https://archive.org/download/" + url.PathEscape(id) + "/" + strings.Join(segs, "/")
}
