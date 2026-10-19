package lintboot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression test: lint-boot silently skipped every PCI device when the class
// code kept sysfs's "0x" prefix. These cases use real class values.
func TestBootCriticalCategory(t *testing.T) {
	cases := []struct {
		class string
		want  string
		ok    bool
	}{
		{"0x010601", "storage", true}, // AHCI SATA, raw sysfs form
		{"010601", "storage", true},   // normalized form
		{"0x010802", "storage", true}, // NVMe
		{"0x0c0330", "usb", true},     // xHCI
		{"0x0c0320", "usb", true},     // EHCI
		{"0x030000", "gpu", true},     // VGA
		{"0x020000", "network", true}, // Ethernet
		{"0X0C0330", "usb", true},     // odd casing is tolerated
		{"0x060400", "", false},       // PCI bridge: not boot-critical
		{"0x088000", "", false},       // system peripheral
		{"0x0c05", "", false},         // SMBus: not boot-critical
		{"", "", false},
		{"0x", "", false},
	}
	for _, c := range cases {
		got, ok := bootCriticalCategory(c.class)
		if got != c.want || ok != c.ok {
			t.Errorf("bootCriticalCategory(%q) = (%q, %v), want (%q, %v)", c.class, got, ok, c.want, c.ok)
		}
	}
}

// Regression tests for the built-in module check (found 2026-10-19 on Ubuntu
// 24.04: ehci-pci is built in, modprobe reports ehci_pci, and the old
// substring match on the underscore name never found it).
func TestModuleInBuiltinList(t *testing.T) {
	list := "kernel/drivers/usb/host/ehci-hcd.ko\n" +
		"kernel/drivers/usb/host/ehci-pci.ko\n" +
		"kernel/drivers/ata/libahci.ko\n" +
		"kernel/fs/ext4/ext4.ko\n"
	cases := []struct {
		module string
		want   bool
	}{
		{"ehci_pci", true}, // underscore spelling vs dash in the file
		{"ehci-pci", true}, // same module, dash spelling
		{"EHCI_PCI", true}, // case-insensitive
		{"ext4", true},
		{"libahci", true},
		{"ahci", false}, // must NOT match as a substring of libahci
		{"radeon", false},
		{"", false},
	}
	for _, c := range cases {
		if got := moduleInBuiltinList(list, c.module); got != c.want {
			t.Errorf("moduleInBuiltinList(%q) = %v, want %v", c.module, got, c.want)
		}
	}
}

func TestModulesToCheck(t *testing.T) {
	cases := []struct {
		name    string
		bound   string
		aliases []string
		want    []string
	}{
		{"radeon, not amdgpu", "radeon", []string{"radeon", "amdgpu"}, []string{"radeon"}},
		{"pata_jmicron, not pata_acpi", "pata_jmicron", []string{"pata_jmicron", "pata_acpi"}, []string{"pata_jmicron"}},
		{"dash vs underscore", "ehci-pci", []string{"ehci_pci"}, []string{"ehci_pci"}},
		{"bound name differs from module: keep all", "xhci_hcd", []string{"xhci_pci"}, []string{"xhci_pci"}},
		{"nothing bound: keep all", "", []string{"radeon", "amdgpu"}, []string{"radeon", "amdgpu"}},
		{"no aliases", "ahci", nil, nil},
	}
	for _, c := range cases {
		got := modulesToCheck(c.bound, c.aliases)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			}
		}
	}
}

func TestFixText(t *testing.T) {
	deb := fixText("radeon", "/boot/initrd.img-7.0.0-38-generic")
	if !strings.Contains(deb, "update-initramfs") || strings.Contains(deb, "mkinitcpio") {
		t.Errorf("Debian-style initramfs should get an initramfs-tools fix, got %q", deb)
	}
	arch := fixText("radeon", "/boot/initramfs-linux-cachyos.img")
	if !strings.Contains(arch, "mkinitcpio") || strings.Contains(arch, "update-initramfs") {
		t.Errorf("Arch-style initramfs should get a mkinitcpio fix, got %q", arch)
	}
}

// Real shapes: an Arch/CachyOS listing prints full paths ending in .ko.zst; a
// Debian one does too. Non-module lines and the -a summary text are ignored.
func TestParseModuleList(t *testing.T) {
	out := "usr/lib/modules/7.1.8-1-cachyos/kernel/drivers/gpu/drm/amd/amdgpu/amdgpu.ko.zst\n" +
		"usr/lib/modules/7.1.8-1-cachyos/kernel/drivers/nvme/host/nvme-core.ko.zst\n" +
		"usr/lib/modules/7.0.0/kernel/drivers/ata/ahci.ko.xz\n" +
		"usr/lib/modules/7.0.0/kernel/fs/ext4/ext4.ko\n" +
		"usr/bin/busybox\n" +
		"==> Included modules (28):\n"
	got := parseModuleList(out)
	want := map[string]bool{"amdgpu": true, "nvme_core": true, "ahci": true, "ext4": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d modules", got, len(want))
	}
	for _, m := range got {
		if !want[m] {
			t.Errorf("unexpected module %q in %v", m, got)
		}
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindInitramfsIn(t *testing.T) {
	// CachyOS: image named after the kernel package, not the version.
	root := t.TempDir()
	writeFile(t, root, "usr/lib/modules/7.1.8-1-cachyos/pkgbase", "linux-cachyos\n")
	writeFile(t, root, "boot/initramfs-linux-cachyos.img", "x")
	writeFile(t, root, "boot/initramfs-linux-cachyos-fallback.img", "x")
	if got, want := findInitramfsIn(root, "7.1.8-1-cachyos"), filepath.Join(root, "boot/initramfs-linux-cachyos.img"); got != want {
		t.Errorf("CachyOS: got %q, want %q", got, want)
	}

	// Ubuntu: initrd.img-<version>.
	root = t.TempDir()
	writeFile(t, root, "boot/initrd.img-7.0.0-38-generic", "x")
	if got, want := findInitramfsIn(root, "7.0.0-38-generic"), filepath.Join(root, "boot/initrd.img-7.0.0-38-generic"); got != want {
		t.Errorf("Ubuntu: got %q, want %q", got, want)
	}

	// Unknown name but exactly one non-fallback image: use it.
	root = t.TempDir()
	writeFile(t, root, "boot/initramfs-custom.img", "x")
	writeFile(t, root, "boot/initramfs-custom-fallback.img", "x")
	if got, want := findInitramfsIn(root, "6.1.0"), filepath.Join(root, "boot/initramfs-custom.img"); got != want {
		t.Errorf("single image: got %q, want %q", got, want)
	}

	// Two ambiguous images: refuse to guess.
	root = t.TempDir()
	writeFile(t, root, "boot/initramfs-a.img", "x")
	writeFile(t, root, "boot/initramfs-b.img", "x")
	if got := findInitramfsIn(root, "6.1.0"); got != "" {
		t.Errorf("ambiguous: got %q, want empty", got)
	}
}
