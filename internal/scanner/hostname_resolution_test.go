package scanner

import "testing"

func TestNetBIOSEnabledByDefault(t *testing.T) {
	t.Setenv("NETBIOS_ENABLED", "")
	if !envBool("NETBIOS_ENABLED", true) {
		t.Fatal("NetBIOS should be enabled by default")
	}
}

func TestNetBIOSCanBeDisabled(t *testing.T) {
	t.Setenv("NETBIOS_ENABLED", "false")
	if envBool("NETBIOS_ENABLED", true) {
		t.Fatal("NetBIOS false override was ignored")
	}
}
