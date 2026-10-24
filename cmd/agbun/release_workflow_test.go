package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseWorkflowDisablesSharedToolCaches(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string
				With map[string]any
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	policies := map[string]struct {
		input string
		value bool
	}{
		"actions/setup-go":  {input: "cache", value: false},
		"oven-sh/setup-bun": {input: "no-cache", value: true},
	}
	for jobName, job := range workflow.Jobs {
		for _, step := range job.Steps {
			action, _, _ := strings.Cut(step.Uses, "@")
			if strings.HasPrefix(action, "actions/cache") {
				t.Errorf("%s uses a shared Actions cache", jobName)
			}
			policy, ok := policies[action]
			if !ok {
				continue
			}
			t.Run(jobName+"/"+action, func(t *testing.T) {
				if got := step.With[policy.input]; got != policy.value {
					t.Errorf("%s = %v, want %v", policy.input, got, policy.value)
				}
			})
		}
	}
}

func TestCIKeepsWindowsAdvisoryAndLinuxRequired(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			ContinueOnError bool `yaml:"continue-on-error"`
			Steps           []struct {
				Run             string
				ContinueOnError bool `yaml:"continue-on-error"`
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		job      string
		advisory bool
	}{
		{job: "windows-artifacts", advisory: true},
		{job: "quality", advisory: false},
		{job: "safe-validators", advisory: false},
	} {
		t.Run(test.job, func(t *testing.T) {
			job, ok := workflow.Jobs[test.job]
			if !ok {
				t.Fatalf("missing CI job %q", test.job)
			}
			if job.ContinueOnError != test.advisory {
				t.Errorf("continue-on-error = %v, want %v", job.ContinueOnError, test.advisory)
			}
			for _, step := range job.Steps {
				if step.ContinueOnError {
					t.Errorf("step %q must still report its real outcome", step.Run)
				}
			}
		})
	}
	var windowsTestsRun bool
	for _, step := range workflow.Jobs["windows-artifacts"].Steps {
		if step.Run == "go test ./internal/artifact/..." {
			windowsTestsRun = true
		}
	}
	if !windowsTestsRun {
		t.Error("Windows artifact tests must still run without exclusions")
	}
}
