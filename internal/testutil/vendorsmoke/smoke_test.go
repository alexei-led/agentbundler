package vendorsmoke

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunBoundsTimeAndOutput(t *testing.T) {
	t.Setenv("VENDORSMOKE_HELPER", "1")
	executable := os.Args[0]

	output, err := run(Command{
		Name: "large output", Path: executable,
		Args: []string{"-test.run=^TestVendorSmokeHelperProcess$", "--", "output"},
		Env:  Environment(map[string]string{"VENDORSMOKE_HELPER": "1"}), Timeout: 10 * time.Second,
	})
	if err != nil || !strings.Contains(output, "[output truncated after 32768 bytes]") || len(output) > MaxOutputBytes+100 {
		t.Fatalf("bounded output length = %d, truncated = %t, err = %v", len(output), strings.Contains(output, "[output truncated after 32768 bytes]"), err)
	}

	_, err = run(Command{
		Name: "timeout", Path: executable,
		Args: []string{"-test.run=^TestVendorSmokeHelperProcess$", "--", "sleep"},
		Env:  Environment(map[string]string{"VENDORSMOKE_HELPER": "1"}), Timeout: 20 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestSnapshotDetectsFileChanges(t *testing.T) {
	path := t.TempDir() + "/settings.json"
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := Snapshot(t, path)
	if err := os.WriteFile(path, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if before[path] == treeDigest(t, path) {
		t.Fatal("snapshot did not detect a file change")
	}
}

func TestVendorSmokeHelperProcess(t *testing.T) {
	if os.Getenv("VENDORSMOKE_HELPER") != "1" {
		return
	}
	mode := ""
	for index, argument := range os.Args {
		if argument == "--" && index+1 < len(os.Args) {
			mode = os.Args[index+1]
			break
		}
	}
	switch mode {
	case "output":
		_, _ = os.Stdout.Write([]byte(strings.Repeat("x", MaxOutputBytes+1)))
	case "sleep":
		time.Sleep(time.Second)
	default:
		os.Exit(2)
	}
}
