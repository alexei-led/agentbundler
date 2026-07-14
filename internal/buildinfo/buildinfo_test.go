package buildinfo

import "testing"

func TestVersionUsesInjectedReleaseVersion(t *testing.T) {
	previous := releaseVersion
	releaseVersion = "v1.2.3"
	t.Cleanup(func() { releaseVersion = previous })

	if got := Version(); got != "v1.2.3" {
		t.Fatalf("Version() = %q, want %q", got, "v1.2.3")
	}
}

func TestVersionIsNonEmptyWithoutInjection(t *testing.T) {
	previous := releaseVersion
	releaseVersion = ""
	t.Cleanup(func() { releaseVersion = previous })

	if got := Version(); got == "" {
		t.Fatal("Version() returned an empty version")
	}
}
