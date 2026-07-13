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

func extractZip(src, dir string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dest, err := safeJoin(dir, f.Name)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeEntry(dest, rc, f.Mode())
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extract7z(src, dir string) error {
	r, err := sevenzip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dest, err := safeJoin(dir, f.Name)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeEntry(dest, rc, f.Mode())
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractRar(src, dir string) error {
	rr, err := rardecode.OpenReader(src)
	if err != nil {
		return err
	}
	defer rr.Close()
	for {
		hdr, err := rr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.IsDir {
			continue
		}
		dest, err := safeJoin(dir, hdr.Name)
		if err != nil {
			return err
		}
		if err := writeEntry(dest, rr, 0644); err != nil {
			return err
		}
	}
}

// extractArchive extracts src into dir and deletes src on success.
func extractArchive(src, dir string) error {
	var err error
	switch strings.ToLower(filepath.Ext(src)) {
	case ".zip":
		err = extractZip(src, dir)
	case ".7z":
		err = extract7z(src, dir)
	case ".rar":
		err = extractRar(src, dir)
	default:
		return nil
	}
	if err != nil {
		logf("extraction failed for %s: %v", src, err)
		return err
	}
	logf("extracted and removed %s", src)
	return os.Remove(src)
}
