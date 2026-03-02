package ldc2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValid(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		for _, target := range []string{
			"x86_64-linux-gnu",
			"aarch64-linux-musl",
			"x86_64-windows-gnu",
			"x86_64-windows-msvc",
			"x86_64-apple-macos10.12",
			"arm64-apple-macos11.0",
			"arm64-apple-ios12.0",
			"armv6-linux-gnueabihf",
			"armv7a-unknown-linux-androideabi",
			"aarch64-linux-android",
			"wasm32-unknown-emscripten",
			"wasm32-unknown-unknown-webassembly",
		} {
			t.Run(target, func(t *testing.T) {
				require.True(t, isValid(target))
			})
		}
	})
	t.Run("invalid", func(t *testing.T) {
		for _, target := range []string{
			"fake-target",
			"x86_64-unknown-linux-gnu",
			"aarch64-unknown-linux-gnu",
		} {
			t.Run(target, func(t *testing.T) {
				require.False(t, isValid(target))
			})
		}
	})
}
