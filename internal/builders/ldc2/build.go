// Package ldc2 builds D binaries using LDC2 (the LLVM D compiler) via dub.
package ldc2

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/caarlos0/log"
	"github.com/goreleaser/goreleaser/v2/internal/artifact"
	"github.com/goreleaser/goreleaser/v2/internal/builders/base"
	"github.com/goreleaser/goreleaser/v2/internal/dubpkg"
	"github.com/goreleaser/goreleaser/v2/internal/elf"
	"github.com/goreleaser/goreleaser/v2/internal/gio"
	"github.com/goreleaser/goreleaser/v2/internal/tmpl"
	api "github.com/goreleaser/goreleaser/v2/pkg/build"
	"github.com/goreleaser/goreleaser/v2/pkg/config"
	"github.com/goreleaser/goreleaser/v2/pkg/context"
)

// Default builder instance.
//
//nolint:gochecknoglobals
var Default = &Builder{}

// type constraints
var (
	_ api.Builder          = &Builder{}
	_ api.DependingBuilder = &Builder{}
)

//nolint:gochecknoinits
func init() {
	api.Register("ldc2", Default)
}

// Builder is the LDC2 builder.
type Builder struct{}

// Dependencies implements build.DependingBuilder.
func (b *Builder) Dependencies() []string {
	return []string{"dub", "ldc2"}
}

// Parse implements build.Builder.
//
// Parses an LLVM triple of the form arch-[vendor-]os[-abi].
// Examples: x86_64-linux-gnu, arm64-apple-macos11.0, x86_64-windows-msvc,
// x86_64-unknown-linux-gnu.
func (b *Builder) Parse(target string) (api.Target, error) {
	parts := strings.Split(target, "-")
	if len(parts) < 2 {
		return nil, fmt.Errorf("%s is not a valid build target", target)
	}

	t := Target{
		Target: target,
		Arch:   convertToGoarch(parts[0]),
	}

	switch len(parts) {
	case 2:
		// arch-os (e.g., x86_64-linux)
		t.Os = convertToGoos(parts[1])
	case 3:
		// arch-vendor-os (e.g., arm64-apple-macos11.0)
		// OR arch-os-abi (e.g., x86_64-linux-gnu)
		if isKnownVendor(parts[1]) {
			t.Vendor = parts[1]
			t.Os = convertToGoos(parts[2])
		} else {
			t.Os = convertToGoos(parts[1])
			t.Abi = parts[2]
		}
	default:
		// arch-vendor-os-abi (e.g., x86_64-unknown-linux-gnu)
		t.Vendor = parts[1]
		t.Os = convertToGoos(parts[2])
		t.Abi = strings.Join(parts[3:], "-")
	}

	// wasm32-unknown-unknown-webassembly: bare-metal WebAssembly has OS "unknown";
	// map to "js" so goreleaser artifact paths are meaningful.
	if t.Arch == "wasm" && t.Os == "unknown" {
		t.Os = "js"
	}

	return t, nil
}

var once sync.Once

// WithDefaults implements build.Builder.
func (b *Builder) WithDefaults(build config.Build) (config.Build, error) {
	once.Do(func() {
		log.Warn("you are using the experimental LDC2 builder")
	})

	if len(build.Targets) == 0 {
		build.Targets = defaultTargets()
	}

	if build.Tool == "" {
		build.Tool = "dub"
	}

	if build.Command == "" {
		build.Command = "build"
	}

	if build.Dir == "" {
		build.Dir = "."
	}

	if len(build.Flags) == 0 {
		build.Flags = []string{"--compiler=ldc2", "--build=release"}
	}

	if build.Main != "" {
		return build, errors.New("main is not used for ldc2")
	}

	if err := base.ValidateNonGoConfig(build); err != nil {
		return build, err
	}

	for _, t := range build.Targets {
		if !isValid(t) {
			return build, fmt.Errorf("invalid target: %s", t)
		}
	}

	return build, nil
}

// Build implements build.Builder.
func (b *Builder) Build(ctx *context.Context, build config.Build, options api.Options) error {
	// Read the actual binary name that dub will produce from dub.json.
	dub, err := dubpkg.Open(build.Dir)
	if err != nil {
		return fmt.Errorf("could not read dub.json: %w", err)
	}
	dubBinaryName := dub.BinaryName()

	t := options.Target.(Target)
	a := &artifact.Artifact{
		Type:   artifact.Binary,
		Path:   options.Path,
		Name:   options.Name,
		Goos:   t.Os,
		Goarch: t.Arch,
		Target: t.Target,
		Extra: map[string]any{
			artifact.ExtraBinary:  strings.TrimSuffix(filepath.Base(options.Path), options.Ext),
			artifact.ExtraExt:     options.Ext,
			artifact.ExtraID:      build.ID,
			artifact.ExtraBuilder: "ldc2",
			keyAbi:                t.Abi,
		},
	}

	env := []string{}
	env = append(env, ctx.Env.Strings()...)

	tpl := tmpl.New(ctx).
		WithBuildOptions(options).
		WithEnvS(env).
		WithArtifact(a)

	dubbin, err := tpl.Apply(build.Tool)
	if err != nil {
		return err
	}

	destDir := filepath.Join("dub-out", t.Target)
	command := []string{
		dubbin,
		build.Command,
		"--dest=" + destDir,
	}

	tenv, err := base.TemplateEnv(build.Env, tpl)
	if err != nil {
		return err
	}

	// Build DFLAGS: start with --mtriple for cross-compilation targeting.
	dflags := []string{"--mtriple=" + t.Target}

	// For cross-compilation, add a C/linker driver so ldc2 can produce a
	// correct binary for the target OS/architecture.
	if isCrossCompilation(t) {
		linkerFlags, cleanup, linkerErr := crossLinkerFlags(t)
		if linkerErr != nil {
			return linkerErr
		}
		if cleanup != nil {
			defer cleanup()
		}
		dflags = append(dflags, linkerFlags...)
	}

	env = appendDFlags(append(env, tenv...), strings.Join(dflags, " "))

	flags, err := tpl.Slice(build.Flags, tmpl.NonEmpty())
	if err != nil {
		return err
	}
	command = append(command, flags...)

	if err := base.Exec(ctx, command, env, build.Dir); err != nil {
		return err
	}

	// dub names the output binary after the dub package name, not the
	// goreleaser project name.  On non-Windows hosts cross-compiling for
	// Windows, dub may also omit the .exe suffix.
	realPath := filepath.Join(build.Dir, destDir, dubBinaryName+options.Ext)
	if _, statErr := os.Stat(realPath); os.IsNotExist(statErr) && options.Ext != "" {
		realPath = filepath.Join(build.Dir, destDir, dubBinaryName)
	}

	if err := gio.Copy(realPath, options.Path); err != nil {
		return err
	}

	if err := base.ChTimes(build, tpl, a); err != nil {
		return err
	}

	if elf.IsDynamicallyLinked(a.Path) {
		a.Extra[artifact.ExtranDynLink] = true
	}

	ctx.Artifacts.Add(a)
	return nil
}

// isCrossCompilation reports whether t requires cross-compilation from the
// current host.
func isCrossCompilation(t Target) bool {
	return t.Os != runtime.GOOS || t.Arch != runtime.GOARCH
}

// crossLinkerFlags returns DFLAGS entries that configure ldc2's C compiler
// and linker for cross-compilation.
//
// Strategy:
//  1. If zig is in PATH, write a small wrapper script that invokes
//     `zig cc -target <triple>` and tell ldc2 to use it via --gcc=.
//     zig cc bundles its own sysroot and linker, so no extra toolchain
//     installation is needed for most targets.
//  2. Otherwise fall back to ldc2's built-in LLD via --link-internally.
func crossLinkerFlags(t Target) (flags []string, cleanup func(), err error) {
	if _, lookErr := exec.LookPath("zig"); lookErr == nil {
		return zigCCFlags(t)
	}
	log.WithField("target", t.Target).
		Warn("zig not found; falling back to --link-internally for cross-compilation")
	return []string{"--link-internally"}, nil, nil
}

// toZigTarget converts an LLVM target triple to zig's target format.
//
// Differences from LLVM triples:
//   - No vendor field (apple, unknown, pc stripped out)
//   - "darwin" → "macos"; versioned "macos10.12" → "macos.10.12"
//   - "ios12.0" → "ios.12.0"
//   - "wasip1" → "wasi"
//   - "arm64" → "aarch64" (zig canonical name)
//   - "armv6", "armv7a" → "arm"
//   - "androideabi" ABI → "android"
//   - "unknown" OS (bare-metal wasm) → "freestanding"
func toZigTarget(t Target) string {
	parts := strings.Split(t.Target, "-")
	arch := parts[0]

	// Normalize arch to zig canonical names.
	switch arch {
	case "arm64":
		arch = "aarch64"
	case "armv6", "armv6l", "armv7a":
		arch = "arm"
	}

	// Extract the raw OS component from the original triple, skipping any vendor.
	var rawOs string
	switch len(parts) {
	case 2:
		rawOs = parts[1]
	default:
		if isKnownVendor(parts[1]) {
			rawOs = parts[2]
		} else {
			rawOs = parts[1]
		}
	}

	// Convert raw OS component to zig format.
	var zigOs string
	switch {
	case rawOs == "darwin":
		zigOs = "macos"
	case strings.HasPrefix(rawOs, "macos"):
		// e.g. "macos10.12" → "macos.10.12"
		if v := strings.TrimPrefix(rawOs, "macos"); v != "" {
			zigOs = "macos." + v
		} else {
			zigOs = "macos"
		}
	case strings.HasPrefix(rawOs, "ios"):
		// e.g. "ios12.0" → "ios.12.0"
		if v := strings.TrimPrefix(rawOs, "ios"); v != "" {
			zigOs = "ios." + v
		} else {
			zigOs = "ios"
		}
	case rawOs == "wasi", rawOs == "wasip1":
		zigOs = "wasi"
	case rawOs == "emscripten":
		zigOs = "emscripten"
	case rawOs == "unknown":
		zigOs = "freestanding" // bare-metal WebAssembly
	default:
		zigOs = rawOs
	}

	if t.Abi != "" {
		// Normalize Android ABI: "androideabi" → "android" (zig uses "android").
		abi := t.Abi
		if abi == "androideabi" {
			abi = "android"
		}
		return arch + "-" + zigOs + "-" + abi
	}
	return arch + "-" + zigOs
}

// zigCCFlags creates a temporary `zigcc` wrapper script and returns the
// ldc2 DFLAGS needed to use it.  The returned cleanup func removes the
// wrapper; callers must defer it.
func zigCCFlags(t Target) (flags []string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "goreleaser-zigcc-*")
	if err != nil {
		return nil, nil, fmt.Errorf("could not create zigcc wrapper: %w", err)
	}
	path := f.Name()

	zigTarget := toZigTarget(t)

	// For macOS/iOS targets, zig cc cannot provide Apple system libraries (e.g. libobjc).
	// ldc2 adds -lobjc to its default Darwin link flags; we must strip it so the
	// linker does not fail looking for a library it cannot find.
	// betterC D programs (no D runtime) don't actually call into libobjc.
	stripObjC := t.Os == "darwin" || t.Os == "ios"

	var script string
	if stripObjC {
		// Use bash arrays to filter -lobjc while preserving argument quoting.
		// ldc2 appends "-target <llvm-triple>" last; our -target comes after so zig
		// sees ours last (zig uses the last -target flag when there are duplicates).
		script = fmt.Sprintf(`#!/bin/bash
args=()
for a in "$@"; do [[ "$a" != "-lobjc" ]] && args+=("$a"); done
exec zig cc "${args[@]}" -target %s
`, zigTarget)
	} else {
		// Simple wrapper: put our zig-format target AFTER "$@" so it takes
		// precedence over ldc2's appended "-target <llvm-triple>".
		script = fmt.Sprintf("#!/bin/sh\nexec zig cc \"$@\" -target %s\n", zigTarget)
	}

	_, err = fmt.Fprint(f, script)
	f.Close()
	if err != nil {
		os.Remove(path)
		return nil, nil, fmt.Errorf("could not write zigcc wrapper: %w", err)
	}

	if err := os.Chmod(path, 0o755); err != nil {
		os.Remove(path)
		return nil, nil, fmt.Errorf("could not chmod zigcc wrapper: %w", err)
	}

	cleanup = func() { os.Remove(path) }
	flags = []string{"--gcc=" + path}
	return flags, cleanup, nil
}

// appendDFlags appends flag to the DFLAGS environment variable entry,
// creating it if it does not already exist.
func appendDFlags(env []string, flag string) []string {
	for i, e := range env {
		if strings.HasPrefix(e, "DFLAGS=") {
			env[i] = e + " " + flag
			return env
		}
	}
	return append(env, "DFLAGS="+flag)
}
