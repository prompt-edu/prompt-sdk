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

var EXAMPLE_JSON_STRUCT = struct {
	Key string `json:"key"`
}{Key: "value"}

const EXAMPLE_BLOB_FILENAME = "blob"

var EXAMPLE_BLOB_CONTENT = []byte{1, 2, 3, 4, 5, 6}

func TestZipSanitizationInvalidPaths(t *testing.T) {
	invalid_test_paths := []string{"/evil.sh", "../evil.sh", "../../evil.sh", "something/../../evil.sh", "\\..\\..\\Windows", "C:\\Users\\Public\\evil.sh", "D:/evil.sh"}

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

func setupExportEmpty(t *testing.T) *Export {
	exp, err := NewExport()
	require.NoError(t, err)
	t.Cleanup(exp.Close)
	return exp
}

func setupExportOneJSONEntry(t *testing.T) *Export {
	exp := setupExportEmpty(t)
	exp.AddJSON("_", EXAMPLE_JSON_FILENAME, func() (any, error) {
		return EXAMPLE_JSON_STRUCT, nil
	})
	return exp
}

func setupExportOneBlobEntry(t *testing.T) *Export {
	exp := setupExportEmpty(t)
	exp.AddBlob("_", EXAMPLE_BLOB_FILENAME, func() ([]byte, error) {
		return EXAMPLE_BLOB_CONTENT, nil
	})
	return exp
}

func setupExportOneFileEntry(t *testing.T) *Export {
	exp := setupExportEmpty(t)
	exp.AddFile("_", EXAMPLE_BLOB_FILENAME, func() (io.Reader, error) {
		return bytes.NewReader(EXAMPLE_BLOB_CONTENT), nil
	})
	return exp
}

func TestErrorInvalidURL(t *testing.T) {
	invalid_url := "https:// invalid-url"
	exp := setupExportOneJSONEntry(t)
	c := context.Background()
	err := exp.UploadTo(c, invalid_url)
	require.Error(t, err)
}

func TestErrorEmptyZip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	exp := setupExportEmpty(t)
	c := context.Background()
	err := exp.UploadTo(c, server.URL)
	require.Error(t, err)
}

func readZip(t *testing.T, data []byte) *zip.Reader {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	return r
}

func readZipEntry(t *testing.T, data []byte, expectedName string) []byte {
	t.Helper()
	r := readZip(t, data)

	var found *zip.File
	for _, f := range r.File {
		if f.Name == expectedName {
			found = f
			break
		}
	}
	require.NotNil(t, found, "entry %q not found in zip", expectedName)

	f, err := found.Open()
	require.NoError(t, err)
	defer func() { err := f.Close(); require.NoError(t, err) }()

	content, err := io.ReadAll(f)
	require.NoError(t, err)
	return content
}

func newTestServer(t *testing.T, received *[]byte) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		require.Equal(t, "/", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		*received = body
		w.WriteHeader(http.StatusOK)
		err = r.Body.Close()
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestUploadJSONZIP(t *testing.T) {
	var received []byte
	server := newTestServer(t, &received)

	exp := setupExportOneJSONEntry(t)

	err := exp.UploadTo(context.Background(), server.URL)
	require.NoError(t, err)

	content := readZipEntry(t, received, EXAMPLE_JSON_FILENAME)
	expected, err := json.MarshalIndent(EXAMPLE_JSON_STRUCT, "", "  ")
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(content))
}

func TestUploadBlobZIP(t *testing.T) {
	var received []byte
	server := newTestServer(t, &received)

	exp := setupExportOneBlobEntry(t)

	err := exp.UploadTo(context.Background(), server.URL)
	require.NoError(t, err)

	content := readZipEntry(t, received, EXAMPLE_BLOB_FILENAME)
	require.Equal(t, EXAMPLE_BLOB_CONTENT, content)
}

func TestUploadFileZIP(t *testing.T) {
	var received []byte
	server := newTestServer(t, &received)

	exp := setupExportOneFileEntry(t)

	err := exp.UploadTo(context.Background(), server.URL)
	require.NoError(t, err)

	content := readZipEntry(t, received, EXAMPLE_BLOB_FILENAME)
	require.Equal(t, EXAMPLE_BLOB_CONTENT, content)
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
	exp := setupExportEmpty(t)

	tr := &testReader{
		Reader: bytes.NewReader(EXAMPLE_BLOB_CONTENT),
	}

	exp.AddFile("test", "file", func() (io.Reader, error) {
		return tr, nil
	})

	require.NoError(t, exp.Err())
	require.True(t, tr.closed)
}

func TestAddBlobNilSliceIsSkipped(t *testing.T) {
	exp := setupExportEmpty(t)
	exp.AddBlob("_", EXAMPLE_BLOB_FILENAME, func() ([]byte, error) {
		return nil, nil
	})
	require.NoError(t, exp.Err())
	require.True(t, exp.IsEmpty())
}

func TestAddBlobTypedNilSliceIsSkipped(t *testing.T) {
	exp := setupExportEmpty(t)
	exp.AddBlob("_", EXAMPLE_BLOB_FILENAME, func() ([]byte, error) {
		var b []byte = nil
		return b, nil
	})
	require.NoError(t, exp.Err())
	require.True(t, exp.IsEmpty())
}

func TestAddFileNilReaderIsSkipped(t *testing.T) {
	exp := setupExportEmpty(t)
	exp.AddFile("_", EXAMPLE_BLOB_FILENAME, func() (io.Reader, error) {
		return nil, nil
	})
	require.NoError(t, exp.Err())
	require.True(t, exp.IsEmpty())
}

func TestAddFileTypedNilReaderIsSkipped(t *testing.T) {
	exp := setupExportEmpty(t)
	exp.AddFile("_", EXAMPLE_BLOB_FILENAME, func() (io.Reader, error) {
		var r *bytes.Reader = nil
		return r, nil
	})
	require.NoError(t, exp.Err())
	require.True(t, exp.IsEmpty())
}

func TestAddJSONErrorPropagation(t *testing.T) {
	exp := setupExportEmpty(t)

	exp.AddJSON("_", "file.json", func() (any, error) {
		return nil, fmt.Errorf("whatever")
	})

	require.Error(t, exp.Err())
}

func TestAddAfterUploadReturnsError(t *testing.T) {
	var received []byte
	server := newTestServer(t, &received)

	exp := setupExportOneJSONEntry(t)

	err := exp.UploadTo(context.Background(), server.URL)
	require.NoError(t, err)

	exp.AddJSON("late", "late.json", func() (any, error) {
		return "should not work", nil
	})
	require.ErrorIs(t, exp.Err(), ErrExportFinished)
}

func TestUploadAfterUploadReturnsError(t *testing.T) {
	var received []byte
	server := newTestServer(t, &received)

	exp := setupExportOneJSONEntry(t)
	c := context.Background()

	err := exp.UploadTo(c, server.URL)
	require.NoError(t, err)

	err = exp.UploadTo(c, server.URL)
	require.ErrorIs(t, err, ErrExportFinished)
}

func TestIsEmptyOnFreshExport(t *testing.T) {
	exp := setupExportEmpty(t)
	require.True(t, exp.IsEmpty())
}

func TestIsEmptyAfterAddJSON(t *testing.T) {
	exp := setupExportOneJSONEntry(t)
	require.False(t, exp.IsEmpty())
}

func TestIsEmptyAfterAddBlob(t *testing.T) {
	exp := setupExportOneBlobEntry(t)
	require.False(t, exp.IsEmpty())
}

func TestIsEmptyAfterAddFile(t *testing.T) {
	exp := setupExportOneFileEntry(t)
	require.False(t, exp.IsEmpty())
}

func TestIsEmptyAfterAddJSONNilValue(t *testing.T) {
	exp := setupExportEmpty(t)
	exp.AddJSON("_", EXAMPLE_JSON_FILENAME, func() (any, error) {
		return nil, nil
	})
	require.NoError(t, exp.Err())
	require.True(t, exp.IsEmpty())
}

func readContents(t *testing.T, data []byte) struct {
	ExportedAt string `json:"exported_at"`
	Items      []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"items"`
} {
	t.Helper()
	raw := readZipEntry(t, data, "contents.json")
	var manifest struct {
		ExportedAt string `json:"exported_at"`
		Items      []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	return manifest
}

func TestContentsWrittenForNonEmptyExport(t *testing.T) {
	var received []byte
	server := newTestServer(t, &received)

	exp := setupExportOneJSONEntry(t)
	require.NoError(t, exp.UploadTo(context.Background(), server.URL))

	r := readZip(t, received)
	names := make([]string, len(r.File))
	for i, f := range r.File {
		names[i] = f.Name
	}
	require.Contains(t, names, "contents.json")
}

func TestContentsNotWrittenForEmptyExport(t *testing.T) {
	var received []byte
	server := newTestServer(t, &received)

	exp := setupExportEmpty(t)
	err := exp.UploadTo(context.Background(), server.URL)
	require.NoError(t, err)

	r := readZip(t, received)
	for _, f := range r.File {
		require.NotEqual(t, "contents.json", f.Name)
	}
}

func TestContentsManifestEntries(t *testing.T) {
	var received []byte
	server := newTestServer(t, &received)

	exp := setupExportOneJSONEntry(t)
	require.NoError(t, exp.UploadTo(context.Background(), server.URL))

	manifest := readContents(t, received)
	require.Len(t, manifest.Items, 1)
	require.Equal(t, "_", manifest.Items[0].Name)
	require.Equal(t, EXAMPLE_JSON_FILENAME, manifest.Items[0].Path)
	require.NotEmpty(t, manifest.ExportedAt)
}

func TestContentsAlwaysTwoTopLevelEntriesWithNestedPaths(t *testing.T) {
	var received []byte
	server := newTestServer(t, &received)

	exp := setupExportEmpty(t)
	exp.AddJSON("Nested", "student/records.json", func() (any, error) { return EXAMPLE_JSON_STRUCT, nil })
	require.NoError(t, exp.UploadTo(context.Background(), server.URL))

	r := readZip(t, received)
	// student/records.json + contents.json = 2 entries, guaranteeing macOS creates a folder
	require.Len(t, r.File, 2)
}

func TestExportCloseCleansUp(t *testing.T) {
	exp, err := NewExport()
	require.NoError(t, err)

	name := exp.tmpFile.Name()

	exp.Close()

	_, err = os.Stat(name)
	require.True(t, os.IsNotExist(err))
}
