// Package dubpkg provides DUB (D package manager) project file parsing.
package dubpkg

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Package represents a parsed dub.json package descriptor.
type Package struct {
	Name       string `json:"name"`
	TargetName string `json:"targetName"`
}

// BinaryName returns the output binary name produced by dub.
// It uses targetName when explicitly set, otherwise falls back to name.
func (p Package) BinaryName() string {
	if p.TargetName != "" {
		return p.TargetName
	}
	return p.Name
}

// Open reads and parses the dub.json file in the given directory.
func Open(dir string) (Package, error) {
	var pkg Package
	bts, err := os.ReadFile(filepath.Join(dir, "dub.json"))
	if err != nil {
		return pkg, err
	}
	err = json.Unmarshal(bts, &pkg)
	return pkg, err
}
