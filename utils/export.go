package utils

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// Export assembles data items into a ZIP archive backed by a temporary file.
// Each Add* call writes immediately to disk, keeping memory usage low.
//
// If any Add* call fails, the error is stored and all subsequent Add* calls
// become no-ops. Call Err to check for errors after adding all items.
//
// Microservice developers typically don't create an Export themselves — the SDK
// creates it and passes it to the handler function registered via
// RegisterPrivacyDataExportEndpoint. The handler just calls Add* methods:
//
//	func myExportHandler(c *gin.Context, exp *utils.Export, subject SubjectIdentifiers) error {
//	    exp.AddJSON("User record", "user-record.json", func() (any, error) {
//	        return user.GetUserByID(c, subject.UserID)
//	    })
//	    exp.AddJSON("Enrollments", "enrollments.json", func() (any, error) {
//	        return student.GetEnrollments(c, subject.StudentID)
//	    })
//	    return exp.Err()
//	}
type Export struct {
	tmpFile   *os.File
	zipWriter *zip.Writer
	err       error
}

// NewExport creates a new export backed by a temporary file.
// The caller must call Close when done to clean up resources.
func NewExport() (*Export, error) {
	tmp, err := os.CreateTemp("", "export-*.zip")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}

	return &Export{
		tmpFile:   tmp,
		zipWriter: zip.NewWriter(tmp),
	}, nil
}

// sanitizeZipPath normalizes and validates the path for use in a ZIP entry.
// Returns the cleaned path using forward slashes, or an error if the path
// is absolute or contains traversal segments.
func sanitizeZipPath(p string) (string, error) {
	cleaned := path.Clean(strings.ReplaceAll(p, "\\", "/"))
	if cleaned == "." || path.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("invalid zip entry path: %q", p)
	}
	return cleaned, nil
}

// Err returns the first error that occurred during any Add* call.
func (e *Export) Err() error {
	return e.err
}

// AddJSON marshals v as indented JSON and writes it to the archive at the
// given path. The callback is invoked immediately. If a previous Add* call
// failed, this is a no-op.
func (e *Export) AddJSON(name, path string, fn func() (any, error)) {
	if e.err != nil {
		return
	}

	cleanPath, err := sanitizeZipPath(path)
	if err != nil {
		e.err = err
		return
	}

	v, err := fn()
	if err != nil {
		e.err = fmt.Errorf("collecting %q: %w", name, err)
		return
	}

	w, err := e.zipWriter.Create(cleanPath)
	if err != nil {
		e.err = fmt.Errorf("creating zip entry %q (%s): %w", name, cleanPath, err)
		return
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		e.err = fmt.Errorf("marshaling %q: %w", name, err)
	}
}

// AddBlob writes raw bytes to the archive at the given path.
// The callback is invoked immediately. If a previous Add* call failed,
// this is a no-op.
func (e *Export) AddBlob(name, path string, fn func() ([]byte, error)) {
	if e.err != nil {
		return
	}

	cleanPath, err := sanitizeZipPath(path)
	if err != nil {
		e.err = err
		return
	}

	data, err := fn()
	if err != nil {
		e.err = fmt.Errorf("collecting %q: %w", name, err)
		return
	}

	w, err := e.zipWriter.Create(cleanPath)
	if err != nil {
		e.err = fmt.Errorf("creating zip entry %q (%s): %w", name, cleanPath, err)
		return
	}

	if _, err := w.Write(data); err != nil {
		e.err = fmt.Errorf("writing %q: %w", name, err)
	}
}

// AddFile streams data from an io.Reader into the archive at the given path.
// If the returned reader implements io.Closer, it will be closed after use.
// The callback is invoked immediately. If a previous Add* call failed,
// this is a no-op.
func (e *Export) AddFile(name, path string, fn func() (io.Reader, error)) {
	if e.err != nil {
		return
	}

	cleanPath, err := sanitizeZipPath(path)
	if err != nil {
		e.err = err
		return
	}

	r, err := fn()
	if err != nil {
		e.err = fmt.Errorf("collecting %q: %w", name, err)
		return
	}
	if closer, ok := r.(io.Closer); ok {
		defer func() { _ = closer.Close() }()
	}

	w, err := e.zipWriter.Create(cleanPath)
	if err != nil {
		e.err = fmt.Errorf("creating zip entry %q (%s): %w", name, cleanPath, err)
		return
	}

	if _, err := io.Copy(w, r); err != nil {
		e.err = fmt.Errorf("writing %q: %w", name, err)
	}
}

// UploadTo finalizes the ZIP archive and uploads it via HTTP PUT to the
// presigned S3 URL. Returns any error from Add* calls or the upload itself.
func (e *Export) UploadTo(ctx context.Context, presignedURL string) error {
	if e.err != nil {
		return e.err
	}

	if err := e.zipWriter.Close(); err != nil {
		return fmt.Errorf("closing zip: %w", err)
	}
	e.zipWriter = nil

	if _, err := e.tmpFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seeking temp file: %w", err)
	}

	stat, err := e.tmpFile.Stat()
	if err != nil {
		return fmt.Errorf("stat temp file: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, presignedURL, e.tmpFile)
	if err != nil {
		return fmt.Errorf("creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/zip")
	req.ContentLength = stat.Size()

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("uploading export: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload failed with status %s (%d): %s", resp.Status, resp.StatusCode, string(body))
	}

  // set an error in case any function gets called again
  e.err = fmt.Errorf("Export already uploaded")
	return nil
}

// Close cleans up resources. Safe to call multiple times.
func (e *Export) Close() {
	if e.zipWriter != nil {
		_ = e.zipWriter.Close()
		e.zipWriter = nil
	}
	if e.tmpFile != nil {
		_ = e.tmpFile.Close()
		_ = os.Remove(e.tmpFile.Name())
		e.tmpFile = nil
	}
}
