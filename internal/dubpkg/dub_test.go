package dubpkg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	t.Run("name only", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dub.json"), []byte(`{"name":"myapp"}`), 0o644))
		pkg, err := Open(dir)
		require.NoError(t, err)
		require.Equal(t, "myapp", pkg.BinaryName())
	})

	t.Run("targetName overrides name", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dub.json"), []byte(`{"name":"myapp","targetName":"my-binary"}`), 0o644))
		pkg, err := Open(dir)
		require.NoError(t, err)
		require.Equal(t, "my-binary", pkg.BinaryName())
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := Open(t.TempDir())
		require.Error(t, err)
	})
}
