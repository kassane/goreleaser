package ldc2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/goreleaser/goreleaser/v2/internal/artifact"
	"github.com/goreleaser/goreleaser/v2/internal/testctx"
	"github.com/goreleaser/goreleaser/v2/internal/testlib"
	api "github.com/goreleaser/goreleaser/v2/pkg/build"
	"github.com/goreleaser/goreleaser/v2/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestDependencies(t *testing.T) {
	require.NotEmpty(t, Default.Dependencies())
}

func TestParse(t *testing.T) {
	for target, dst := range map[string]Target{
		"x86_64-linux-gnu": {
			Target: "x86_64-linux-gnu",
			Os:     "linux",
			Arch:   "amd64",
			Abi:    "gnu",
		},
		"aarch64-linux-musl": {
			Target: "aarch64-linux-musl",
			Os:     "linux",
			Arch:   "arm64",
			Abi:    "musl",
		},
		"x86_64-windows-gnu": {
			Target: "x86_64-windows-gnu",
			Os:     "windows",
			Arch:   "amd64",
			Abi:    "gnu",
		},
		"x86_64-windows-msvc": {
			Target: "x86_64-windows-msvc",
			Os:     "windows",
			Arch:   "amd64",
			Abi:    "msvc",
		},
		"x86_64-unknown-linux-gnu": {
			Target: "x86_64-unknown-linux-gnu",
			Os:     "linux",
			Arch:   "amd64",
			Vendor: "unknown",
			Abi:    "gnu",
		},
		"x86_64-linux": {
			Target: "x86_64-linux",
			Os:     "linux",
			Arch:   "amd64",
		},
		// Versioned macOS and iOS
		"x86_64-apple-macos10.12": {
			Target: "x86_64-apple-macos10.12",
			Os:     "darwin",
			Arch:   "amd64",
			Vendor: "apple",
		},
		"arm64-apple-macos11.0": {
			Target: "arm64-apple-macos11.0",
			Os:     "darwin",
			Arch:   "arm64",
			Vendor: "apple",
		},
		"arm64-apple-ios12.0": {
			Target: "arm64-apple-ios12.0",
			Os:     "ios",
			Arch:   "arm64",
			Vendor: "apple",
		},
		// Android
		"armv6-linux-gnueabihf": {
			Target: "armv6-linux-gnueabihf",
			Os:     "linux",
			Arch:   "arm",
			Abi:    "gnueabihf",
		},
		"armv7a-unknown-linux-androideabi": {
			Target: "armv7a-unknown-linux-androideabi",
			Os:     "linux",
			Arch:   "arm",
			Vendor: "unknown",
			Abi:    "androideabi",
		},
		"aarch64-linux-android": {
			Target: "aarch64-linux-android",
			Os:     "linux",
			Arch:   "arm64",
			Abi:    "android",
		},
		// WebAssembly
		"wasm32-unknown-emscripten": {
			Target: "wasm32-unknown-emscripten",
			Os:     "js",
			Arch:   "wasm",
			Vendor: "unknown",
		},
		"wasm32-unknown-unknown-webassembly": {
			Target: "wasm32-unknown-unknown-webassembly",
			Os:     "js",
			Arch:   "wasm",
			Vendor: "unknown",
			Abi:    "webassembly",
		},
	} {
		t.Run(target, func(t *testing.T) {
			got, err := Default.Parse(target)
			require.NoError(t, err)
			require.IsType(t, Target{}, got)
			require.Equal(t, dst, got.(Target))
		})
	}
	t.Run("invalid", func(t *testing.T) {
		_, err := Default.Parse("linux")
		require.Error(t, err)
	})
}

func TestWithDefaults(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		build, err := Default.WithDefaults(config.Build{})
		require.NoError(t, err)
		require.Equal(t, config.Build{
			Tool:    "dub",
			Command: "build",
			Dir:     ".",
			Targets: defaultTargets(),
			BuildDetails: config.BuildDetails{
				Flags: []string{"--compiler=ldc2", "--build=release"},
			},
		}, build)
	})

	t.Run("invalid target", func(t *testing.T) {
		_, err := Default.WithDefaults(config.Build{
			Targets: []string{"not-a-real-target"},
		})
		require.Error(t, err)
	})

	t.Run("invalid config option", func(t *testing.T) {
		_, err := Default.WithDefaults(config.Build{
			Main: "something",
		})
		require.Error(t, err)
	})
}

func TestAppendDFlags(t *testing.T) {
	t.Run("no existing DFLAGS", func(t *testing.T) {
		env := []string{"FOO=bar", "BAZ=qux"}
		result := appendDFlags(env, "--mtriple=x86_64-linux-gnu")
		require.Contains(t, result, "DFLAGS=--mtriple=x86_64-linux-gnu")
	})
	t.Run("existing DFLAGS", func(t *testing.T) {
		env := []string{"DFLAGS=-O2", "FOO=bar"}
		result := appendDFlags(env, "--mtriple=x86_64-linux-gnu")
		require.Contains(t, result, "DFLAGS=-O2 --mtriple=x86_64-linux-gnu")
	})
}

func TestIsCrossCompilation(t *testing.T) {
	for target, wantCross := range map[string]bool{
		"x86_64-linux-gnu":    runtime.GOOS != "linux" || runtime.GOARCH != "amd64",
		"aarch64-linux-gnu":   runtime.GOOS != "linux" || runtime.GOARCH != "arm64",
		"x86_64-apple-macos10.12": runtime.GOOS != "darwin" || runtime.GOARCH != "amd64",
		"x86_64-windows-msvc": runtime.GOOS != "windows" || runtime.GOARCH != "amd64",
	} {
		t.Run(target, func(t *testing.T) {
			tgt, err := Default.Parse(target)
			require.NoError(t, err)
			require.Equal(t, wantCross, isCrossCompilation(tgt.(Target)))
		})
	}
}

func TestToZigTarget(t *testing.T) {
	for triple, want := range map[string]string{
		// Linux
		"x86_64-linux-gnu":   "x86_64-linux-gnu",
		"aarch64-linux-musl": "aarch64-linux-musl",
		// macOS: darwin → macos; versioned macos stays versioned
		"x86_64-apple-macos10.12": "x86_64-macos.10.12",
		// arm64 (Apple alias for aarch64) → aarch64 in zig
		"arm64-apple-macos11.0": "aarch64-macos.11.0",
		// iOS
		"arm64-apple-ios12.0": "aarch64-ios.12.0",
		// Windows
		"x86_64-windows-msvc": "x86_64-windows-msvc",
		"x86_64-windows-gnu":  "x86_64-windows-gnu",
		// Other OS
		"x86_64-freebsd": "x86_64-freebsd",
		// WASM
		"wasm32-wasi":                      "wasm32-wasi",
		"wasm32-unknown-emscripten":        "wasm32-emscripten",
		"wasm32-unknown-unknown-webassembly": "wasm32-freestanding-webassembly",
		// Android
		"armv6-linux-gnueabihf":             "arm-linux-gnueabihf",
		"armv7a-unknown-linux-androideabi":  "arm-linux-android",
		"aarch64-linux-android":             "aarch64-linux-android",
	} {
		t.Run(triple, func(t *testing.T) {
			tgt, err := Default.Parse(triple)
			require.NoError(t, err)
			require.Equal(t, want, toZigTarget(tgt.(Target)))
		})
	}
}

func TestZigCCFlags(t *testing.T) {
	testlib.CheckPath(t, "zig")

	tgt, err := Default.Parse("aarch64-linux-gnu")
	require.NoError(t, err)

	flags, cleanup, err := zigCCFlags(tgt.(Target))
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	require.Len(t, flags, 1)
	require.True(t, strings.HasPrefix(flags[0], "--gcc="), "expected --gcc= flag, got %q", flags[0])

	// The wrapper script must be executable.
	wrapper := strings.TrimPrefix(flags[0], "--gcc=")
	fi, err := os.Stat(wrapper)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&0o111, "wrapper script must be executable")

	// The zig target must appear AFTER "$@" so it overrides any -target flag
	// that ldc2 appends when invoking the wrapper as a C compiler driver.
	content, err := os.ReadFile(wrapper)
	require.NoError(t, err)
	require.Contains(t, string(content), "\"$@\" -target aarch64-linux-gnu")
}

func TestBuild(t *testing.T) {
	testlib.CheckPath(t, "dub")
	testlib.CheckPath(t, "ldc2")

	folder := testlib.Mktmp(t)
	folder = filepath.Join(folder, "proj")
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "source"), 0o755))

	// Write a minimal dub.json
	dubJSON := map[string]any{
		"name":        "proj",
		"description": "A test D project",
		"authors":     []string{"Test"},
		"targetType":  "executable",
	}
	dubJSONBytes, err := json.Marshal(dubJSON)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(folder, "dub.json"), dubJSONBytes, 0o644))

	// Write a minimal D main source file
	require.NoError(t, os.WriteFile(
		filepath.Join(folder, "source", "app.d"),
		[]byte("void main() {}\n"),
		0o644,
	))

	modTime := time.Now().AddDate(-1, 0, 0).Round(time.Second).UTC()
	ctx := testctx.WrapWithCfg(t.Context(), config.Project{
		Dist:        "dist",
		ProjectName: "proj",
		Builds: []config.Build{
			{
				ID:           "default",
				Dir:          "./proj/",
				ModTimestamp: fmt.Sprintf("%d", modTime.Unix()),
				BuildDetails: config.BuildDetails{
					Flags: []string{"--compiler=ldc2", "--build=release"},
				},
			},
		},
	})

	build, err := Default.WithDefaults(ctx.Config.Builds[0])
	require.NoError(t, err)

	options := api.Options{
		Name: "proj",
		Path: filepath.Join("dist", "proj-x86_64-linux-gnu", "proj"),
	}
	options.Target, err = Default.Parse("x86_64-linux-gnu")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(options.Path), 0o755))

	require.NoError(t, Default.Build(ctx, build, options))

	bins := ctx.Artifacts.List()
	require.Len(t, bins, 1)

	bin := bins[0]
	require.Equal(t, "proj", bin.Name)
	require.Equal(t, filepath.ToSlash(options.Path), bin.Path)
	require.Equal(t, "linux", bin.Goos)
	require.Equal(t, "amd64", bin.Goarch)
	require.Equal(t, "x86_64-linux-gnu", bin.Target)
	require.Equal(t, artifact.Binary, bin.Type)
	require.Equal(t, "proj", bin.Extra[artifact.ExtraBinary])
	require.Equal(t, "ldc2", bin.Extra[artifact.ExtraBuilder])
	require.Equal(t, "", bin.Extra[artifact.ExtraExt])
	require.Equal(t, "default", bin.Extra[artifact.ExtraID])
	require.Equal(t, "gnu", bin.Extra[keyAbi])

	require.FileExists(t, bin.Path)
	fi, err := os.Stat(bin.Path)
	require.NoError(t, err)
	require.True(t, modTime.Equal(fi.ModTime()))
}
