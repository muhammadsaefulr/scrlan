package scanner

import "testing"

func TestEnvBoolAcceptsCommonValues(t *testing.T) {
	values := map[string]bool{
		"true": true, " TRUE ": true, "1": true, "yes": true, "on": true,
		"false": false, "0": false, "no": false, "off": false,
	}
	for value, want := range values {
		t.Setenv("TEST_BOOL", value)
		if got := envBool("TEST_BOOL", !want); got != want {
			t.Fatalf("envBool(%q) = %v, want %v", value, got, want)
		}
	}
	t.Setenv("TEST_BOOL", "unknown")
	if got := envBool("TEST_BOOL", true); !got {
		t.Fatal("invalid value did not use fallback")
	}
}
