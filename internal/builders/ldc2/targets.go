package ldc2

import (
	"slices"
	"strings"
	"sync"

	_ "embed"

	"github.com/goreleaser/goreleaser/v2/internal/tmpl"
)

var (
	//go:embed all_targets.txt
	allTargetsBts []byte

	allTargets  []string
	targetsOnce sync.Once
)

const (
	keyAbi    = "Abi"
	keyVendor = "Vendor"
)

// Target is a LDC2 build target (LLVM triple format).
type Target struct {
	// The LLVM triple formatted target (arch-[vendor-]os[-abi]).
	Target string
	Os     string
	Arch   string
	Vendor string
	Abi    string
}

// Fields implements build.Target.
func (t Target) Fields() map[string]string {
	return map[string]string{
		tmpl.KeyOS:   t.Os,
		tmpl.KeyArch: t.Arch,
		keyAbi:       t.Abi,
		keyVendor:    t.Vendor,
	}
}

// String implements fmt.Stringer.
func (t Target) String() string {
	return t.Target
}

func convertToGoos(s string) string {
	switch {
	case s == "darwin" || strings.HasPrefix(s, "macos"):
		return "darwin"
	case strings.HasPrefix(s, "ios"):
		return "ios"
	case s == "wasi":
		return "wasip1"
	case s == "emscripten":
		return "js"
	default:
		return s
	}
}

func convertToGoarch(s string) string {
	switch s {
	case "aarch64", "arm64":
		return "arm64"
	case "x86_64":
		return "amd64"
	case "i686", "i386", "x86":
		return "386"
	case "arm", "armv6", "armv6l", "armv7", "armv7a":
		return "arm"
	case "riscv64":
		return "riscv64"
	case "powerpc64le":
		return "ppc64le"
	case "powerpc64":
		return "ppc64"
	case "powerpc":
		return "ppc"
	case "loongarch64":
		return "loong64"
	case "s390x":
		return "s390x"
	case "wasm32":
		return "wasm"
	default:
		return s
	}
}

// isKnownVendor returns true if s is a recognized LLVM vendor string.
func isKnownVendor(s string) bool {
	return slices.Contains([]string{"unknown", "pc", "apple", "none", "scei"}, s)
}

func isValid(target string) bool {
	targetsOnce.Do(func() {
		allTargets = strings.Split(strings.TrimSpace(string(allTargetsBts)), "\n")
	})
	return slices.Contains(allTargets, target)
}

func defaultTargets() []string {
	return []string{
		"x86_64-linux-gnu",
		"x86_64-apple-macos10.12",
		"x86_64-windows-msvc",
		"aarch64-linux-gnu",
		"arm64-apple-macos11.0",
	}
}
