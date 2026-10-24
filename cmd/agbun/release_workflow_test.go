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
