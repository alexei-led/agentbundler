package write

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestReplaceOutputWritesDeterministicPathsAndRemovesStaleFiles(t *testing.T) {
	output := filepath.Join(t.TempDir(), "generated")
	mustWriteFile(t, filepath.Join(output, "stale.txt"), "stale")

	plan := model.BuildPlan{
		CompilerFiles: []model.PlannedFile{
			{Path: "z/compiler.txt", Bytes: []byte("compiler-z")},
			{Path: "a/compiler.txt", Bytes: []byte("compiler-a")},
		},
		Targets: []model.TargetPlan{{
			Target: model.TargetClaude,
			Files: []model.PlannedFile{
				{Path: "z.txt", Bytes: []byte("target-z")},
				{Path: "a.txt", Bytes: []byte("target-a")},
			},
		}},
	}

	for attempt := 0; attempt < 2; attempt++ {
		if diagnostics := ReplaceOutput(plan, output); len(diagnostics) != 0 {
			t.Fatalf("ReplaceOutput() diagnostics = %#v", diagnostics)
		}
		assertFile(t, filepath.Join(output, "a/compiler.txt"), "compiler-a")
		assertFile(t, filepath.Join(output, "z/compiler.txt"), "compiler-z")
		assertFile(t, filepath.Join(output, "claude/a.txt"), "target-a")
		assertFile(t, filepath.Join(output, "claude/z.txt"), "target-z")
		if _, err := os.Lstat(filepath.Join(output, "stale.txt")); !os.IsNotExist(err) {
			t.Fatalf("stale output exists or could not be checked: %v", err)
		}
	}
}

func TestReplaceOutputAppliesExecutableIntent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows rejects executable intent")
	}

	output := filepath.Join(t.TempDir(), "generated")
	plan := model.BuildPlan{CompilerFiles: []model.PlannedFile{
		{Path: "bin/run", Bytes: []byte("run"), Executable: true},
		{Path: "data/readme", Bytes: []byte("read"), Executable: false},
	}}
	if diagnostics := ReplaceOutput(plan, output); len(diagnostics) != 0 {
		t.Fatalf("ReplaceOutput() diagnostics = %#v", diagnostics)
	}

	for path, executable := range map[string]bool{
		"bin/run":     true,
		"data/readme": false,
	} {
		info, err := os.Stat(filepath.Join(output, path))
		if err != nil {
			t.Fatalf("os.Stat(%q) error = %v", path, err)
		}
		if got := info.Mode()&0o111 != 0; got != executable {
			t.Errorf("%s executable = %t, want %t (mode %o)", path, got, executable, info.Mode())
		}
	}
}

func TestReplaceOutputStagingFailurePreservesExistingOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "generated")
	mustWriteFile(t, filepath.Join(output, "preserved.txt"), "old")

	diagnostics := ReplaceOutput(model.BuildPlan{CompilerFiles: []model.PlannedFile{
		{Path: "conflict", Bytes: []byte("file")},
		{Path: "conflict/child", Bytes: []byte("child")},
	}}, output)
	if len(diagnostics) != 1 || diagnostics[0].Code != diagnosticWriteFailed {
		t.Fatalf("ReplaceOutput() diagnostics = %#v", diagnostics)
	}
	assertFile(t, filepath.Join(output, "preserved.txt"), "old")
}

func TestReplaceOutputRecoversFallbackJournal(t *testing.T) {
	for _, test := range []struct {
		name          string
		phase         string
		hadOld        bool
		expectedState string
	}{
		{name: "prepared with old root", phase: phasePrepared, hadOld: true, expectedState: "old"},
		{name: "prepared without old root", phase: phasePrepared, hadOld: false, expectedState: "absent"},
		{name: "old moved with old root", phase: phaseOldMoved, hadOld: true, expectedState: "old"},
		{name: "old moved without old root", phase: phaseOldMoved, hadOld: false, expectedState: "absent"},
		{name: "new installed with old root", phase: phaseNewInstalled, hadOld: true, expectedState: "new"},
		{name: "new installed without old root", phase: phaseNewInstalled, hadOld: false, expectedState: "new"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "generated")
			staging := filepath.Join(filepath.Dir(output), ".generated.agentbundler-write-staging-interrupted")
			backup := backupPath(output, staging)
			mustMkdir(t, staging)

			switch test.phase {
			case phasePrepared:
				if test.hadOld {
					mustWriteFile(t, filepath.Join(output, "state.txt"), "old")
				}
			case phaseOldMoved:
				if test.hadOld {
					mustWriteFile(t, filepath.Join(backup, "state.txt"), "old")
				}
				mustWriteFile(t, filepath.Join(output, "state.txt"), "new")
			case phaseNewInstalled:
				mustWriteFile(t, filepath.Join(output, "state.txt"), "new")
				if test.hadOld {
					mustWriteFile(t, filepath.Join(backup, "state.txt"), "old")
				}
			}
			mustWriteJournal(t, output, replacementJournal{
				Output:  output,
				Staging: staging,
				Backup:  backup,
				HadOld:  test.hadOld,
				Phase:   test.phase,
			})

			diagnostics := ReplaceOutput(failingPlan(), output)
			if len(diagnostics) != 1 || diagnostics[0].Code != diagnosticWriteFailed {
				t.Fatalf("ReplaceOutput() diagnostics = %#v", diagnostics)
			}
			switch test.expectedState {
			case "old":
				assertFile(t, filepath.Join(output, "state.txt"), "old")
			case "new":
				assertFile(t, filepath.Join(output, "state.txt"), "new")
			case "absent":
				if _, err := os.Lstat(output); !os.IsNotExist(err) {
					t.Fatalf("output root exists or could not be checked: %v", err)
				}
			}
			if _, err := os.Lstat(journalPath(output)); !os.IsNotExist(err) {
				t.Fatalf("journal remains or could not be checked: %v", err)
			}
		})
	}
}

func failingPlan() model.BuildPlan {
	return model.BuildPlan{CompilerFiles: []model.PlannedFile{
		{Path: "conflict", Bytes: []byte("file")},
		{Path: "conflict/child", Bytes: []byte("child")},
	}}
}

func mustWriteJournal(t *testing.T, output string, journal replacementJournal) {
	t.Helper()
	bytes, err := json.Marshal(journal)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	mustWriteFile(t, journalPath(output), string(bytes))
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q) error = %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}
	if string(got) != want {
		t.Errorf("os.ReadFile(%q) = %q, want %q", path, got, want)
	}
}
