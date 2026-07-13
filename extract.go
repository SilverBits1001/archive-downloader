package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
)

func isArchive(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip", ".7z", ".rar":
		return true
	}
	return false
}

// safeJoin guards against zip-slip: the joined path must stay in dir.
func safeJoin(dir, name string) (string, error) {
	p := filepath.Join(dir, filepath.Clean("/"+name))
	if !strings.HasPrefix(p, filepath.Clean(dir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry escapes destination: %s", name)
	}
	return p, nil
}

func writeEntry(dest string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm()|0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func extractZip(src, dir string) ([]string, error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var written []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dest, err := safeJoin(dir, f.Name)
		if err != nil {
			return written, err
		}
		rc, err := f.Open()
		if err != nil {
			return written, err
		}
		err = writeEntry(dest, rc, f.Mode())
		rc.Close()
		if err != nil {
			return written, err
		}
		written = append(written, dest)
	}
	return written, nil
}

func extract7z(src, dir string) ([]string, error) {
	r, err := sevenzip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var written []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dest, err := safeJoin(dir, f.Name)
		if err != nil {
			return written, err
		}
		rc, err := f.Open()
		if err != nil {
			return written, err
		}
		err = writeEntry(dest, rc, f.Mode())
		rc.Close()
		if err != nil {
			return written, err
		}
		written = append(written, dest)
	}
	return written, nil
}

func extractRar(src, dir string) ([]string, error) {
	rr, err := rardecode.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer rr.Close()
	var written []string
	for {
		hdr, err := rr.Next()
		if errors.Is(err, io.EOF) {
			return written, nil
		}
		if err != nil {
			return written, err
		}
		if hdr.IsDir {
			continue
		}
		dest, err := safeJoin(dir, hdr.Name)
		if err != nil {
			return written, err
		}
		if err := writeEntry(dest, rr, 0644); err != nil {
			return written, err
		}
		written = append(written, dest)
	}
}

// extractArchive extracts src into dir, deletes src on success, and
// returns the files it wrote.
func extractArchive(src, dir string) ([]string, error) {
	var files []string
	var err error
	switch strings.ToLower(filepath.Ext(src)) {
	case ".zip":
		files, err = extractZip(src, dir)
	case ".7z":
		files, err = extract7z(src, dir)
	case ".rar":
		files, err = extractRar(src, dir)
	default:
		return nil, nil
	}
	if err != nil {
		logf("extraction failed for %s: %v", src, err)
		return files, err
	}
	logf("extracted %d file(s) and removed %s", len(files), src)
	return files, os.Remove(src)
}
