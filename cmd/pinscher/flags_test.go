package main

import (
	"os"
	"reflect"
	"testing"
)

// flagValues reads os.Args[2:], so these tests drive it the same way the
// real CLI does rather than refactoring it to take an explicit slice —
// matches how hasFlag works elsewhere in this file. First test file in
// cmd/pinscher; added by Ghost, 2026-10-17, alongside --assert.
func withArgs(args []string, fn func()) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = append([]string{"pinscher"}, args...)
	fn()
}

func TestFlagValuesNone(t *testing.T) {
	withArgs([]string{"lint-boot"}, func() {
		got := flagValues("--assert")
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestFlagValuesSingle(t *testing.T) {
	withArgs([]string{"lint-boot", "--assert", "driver:amdgpu=amdgpu"}, func() {
		got := flagValues("--assert")
		want := []string{"driver:amdgpu=amdgpu"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestFlagValuesRepeated(t *testing.T) {
	withArgs([]string{
		"lint-boot",
		"--assert", "driver:0000:02:00.0=vfio-pci",
		"--json",
		"--assert", "driver:0000:06:00.0=amdgpu",
	}, func() {
		got := flagValues("--assert")
		want := []string{"driver:0000:02:00.0=vfio-pci", "driver:0000:06:00.0=amdgpu"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestFlagValuesTrailingFlagWithNoValue(t *testing.T) {
	// --assert as the very last arg, with nothing after it, should not
	// panic or read past the end of os.Args.
	withArgs([]string{"lint-boot", "--assert"}, func() {
		got := flagValues("--assert")
		if got != nil {
			t.Errorf("got %v, want nil (no value follows the flag)", got)
		}
	})
}
