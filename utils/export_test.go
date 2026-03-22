package utils

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const EXAMPLE_JSON_FILENAME = "example.json"
var EXAMPLE_JSON_STRUCT = struct{Key string `json:"key"` }{ Key: "value" }
const EXAMPLE_BLOB_FILENAME = "blob"
var EXAMPLE_BLOB_CONTENT = []byte{1, 2, 3, 4, 5, 6}

func TestZipSanitizationInvalidPaths(t *testing.T) {
  invalid_test_paths := []string{"/evil.sh", "../evil.sh", "../../evil.sh", "something/../../evil.sh", "\\..\\..\\Windows"}

  for _, path := range invalid_test_paths {
    _, err := sanitizeZipPath(path)
    require.Error(t, err)
  }
}

func TestZipSanitizationValidPaths(t *testing.T) {
  valid_test_paths := []string{"student/example.json", "example.json", "example.pdf", "a/b/c/d/e/test.json"}

  for _, path := range valid_test_paths {
    pathres, err := sanitizeZipPath(path)
    require.NoError(t, err)
    require.Equal(t, path, pathres)
  }
}

func TestExportCreation(t *testing.T) {
  exp, err := NewExport()
  require.NoError(t, err)
  require.IsType(t, &Export{}, exp)
}

func setupExportEmpty(t *testing.T) (context.Context, *Export) {
  exp, err := NewExport()
  require.NoError(t, err)
  c := context.Background()
  return c, exp
}

func setupExportOneJSONEntry(t *testing.T) (context.Context, *Export) {
  c, exp := setupExportEmpty(t)
  exp.AddJSON("_", EXAMPLE_JSON_FILENAME, func() (any, error) {
    return EXAMPLE_JSON_STRUCT, nil
  })
  return c, exp
}

func setupExportOneBlobEntry(t *testing.T) (context.Context, *Export) {
  c, exp := setupExportEmpty(t)
  exp.AddBlob("_", EXAMPLE_BLOB_FILENAME, func() ([]byte, error) {
    return EXAMPLE_BLOB_CONTENT, nil
  })
  return c, exp
}

func setupExportOneFileEntry(t *testing.T) (context.Context, *Export) {
  c, exp := setupExportEmpty(t)
  exp.AddFile("_", EXAMPLE_BLOB_FILENAME, func() (io.Reader, error) {
    return bytes.NewReader(EXAMPLE_BLOB_CONTENT), nil
  })
  return c, exp
}

func TestErrorInvalidURL(t *testing.T) {
  invalid_url := "https:// invalid-url"
  c, exp := setupExportOneJSONEntry(t)
  err := exp.UploadTo(c, invalid_url)
  require.Error(t, err)
}

func TestErrorEmptyZip(t *testing.T) {
  server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    http.Error(w, "bad request", http.StatusBadRequest)
  }))
  defer server.Close()
  c, exp := setupExportEmpty(t)
  err := exp.UploadTo(c, server.URL)
  require.Error(t, err)
}

func buildZip(path string, write func(io.Writer) error) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	w, err := zipWriter.Create(path)
	if err != nil { return nil, err }
	if err := write(w); err != nil { return nil, err }
	if err := zipWriter.Close(); err != nil { return nil, err }
	return buf.Bytes(), nil
}

func getCompareZipJSON() []byte {
	b, err := buildZip(EXAMPLE_JSON_FILENAME, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(EXAMPLE_JSON_STRUCT)
	})
	if err != nil { return nil }
	return b
}

func getCompareZipBlob() []byte {
	b, err := buildZip(EXAMPLE_BLOB_FILENAME, func(w io.Writer) error {
		_, err := w.Write(EXAMPLE_BLOB_CONTENT)
		return err
	})
	if err != nil { return nil }
	return b
}

func newTestServer( t *testing.T, received *[]byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		require.Equal(t, "/", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		*received = body
		w.WriteHeader(http.StatusOK)
    err = r.Body.Close()
    require.NoError(t, err)
	}))
}

func TestUploadJSONZIP(t *testing.T) {
  var received []byte
  server := newTestServer(t, &received)
  defer server.Close()
  c, exp := setupExportOneJSONEntry(t)

  err := exp.UploadTo(c, server.URL)
  require.NoError(t, err)

  zipbytes := getCompareZipJSON()
  require.Equal(t, zipbytes, received)
}


func TestUploadBlobZIP(t *testing.T) {
  var received []byte
  server := newTestServer(t, &received)
  defer server.Close()
  c, exp := setupExportOneBlobEntry(t)

  err := exp.UploadTo(c, server.URL)
  require.NoError(t, err)

  zipbytes := getCompareZipBlob()
  require.Equal(t, zipbytes, received)
}

func TestUploadFileZIP(t *testing.T) {
  var received []byte
  server := newTestServer(t, &received)
  defer server.Close()
  c, exp := setupExportOneFileEntry(t)

  err := exp.UploadTo(c, server.URL)
  require.NoError(t, err)

  zipbytes := getCompareZipBlob()
  require.Equal(t, zipbytes, received)
}

type testReader struct {
	io.Reader
	closed bool
}

func (t *testReader) Close() error {
	t.closed = true
	return nil
}

func TestAddFileClosesReader(t *testing.T) {
	exp, err := NewExport()
  require.NoError(t, err)

	tr := &testReader{
		Reader: bytes.NewReader(EXAMPLE_BLOB_CONTENT),
	}

	exp.AddFile("test", "file", func() (io.Reader, error) {
		return tr, nil
	})

	require.NoError(t, exp.Err())
	require.True(t, tr.closed)
}

func TestAddJSONErrorPropagation(t *testing.T) {
	exp, err := NewExport()
  require.NoError(t, err)

	exp.AddJSON("_", "file.json", func() (any, error) {
		return nil, fmt.Errorf("whatever")
	})

	require.Error(t, exp.Err())
}

func TestExportCloseCleansUp(t *testing.T) {
	exp, err := NewExport()
	require.NoError(t, err)

	name := exp.tmpFile.Name()

	exp.Close()

	_, err = os.Stat(name)
	require.True(t, os.IsNotExist(err))
}
