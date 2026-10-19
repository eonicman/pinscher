package probe

import "testing"

func TestNormalizePCIClass(t *testing.T) {
	cases := map[string]string{
		"0x010601":  "010601",
		"0x0C0330":  "0c0330",
		"010601\n":  "010601",
		"  0x0300 ": "0300",
		"":          "",
		"0x":        "",
	}
	for in, want := range cases {
		if got := NormalizePCIClass(in); got != want {
			t.Errorf("NormalizePCIClass(%q) = %q, want %q", in, got, want)
		}
	}
}

// The class name lookup and lint-boot both depend on the normalized form.
func TestClassNameForNormalizedPCIClass(t *testing.T) {
	if got := classNameForPCI(NormalizePCIClass("0x010601")); got == "" || got == "Unknown" {
		t.Errorf("classNameForPCI for AHCI SATA = %q, want a real class name", got)
	}
}
