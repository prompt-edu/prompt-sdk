package utils

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskingWriterMasksPasswordSplitAcrossWrites(t *testing.T) {
	var out bytes.Buffer
	w := &maskingWriter{out: &out, password: "s3cr3t"}

	_, err := w.Write([]byte("error: postgres://prompt-postgres:s3c"))
	require.NoError(t, err)
	require.Empty(t, out.String(), "an incomplete line must be held back")

	_, err = w.Write([]byte("r3t@localhost:5432/prompt\n"))
	require.NoError(t, err)
	require.Equal(t, "error: postgres://prompt-postgres:***@localhost:5432/prompt\n", out.String())
}

func TestMaskingWriterFlushesTrailingLine(t *testing.T) {
	var out bytes.Buffer
	w := &maskingWriter{out: &out, password: "s3cr3t"}

	_, err := w.Write([]byte("done s3cr3t"))
	require.NoError(t, err)
	w.flush()
	require.Equal(t, "done ***", out.String())
}
