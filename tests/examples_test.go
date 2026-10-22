package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type caller struct {
	Jobs map[string]struct {
		Uses    string            `yaml:"uses"`
		With    map[string]string `yaml:"with"`
		Secrets map[string]string `yaml:"secrets"`
	} `yaml:"jobs"`
}

var (
	ownCall   = regexp.MustCompile(`^vhco-pro/go-release-workflow/\.github/workflows/([\w.-]+\.yml)@`)
	outputRef = regexp.MustCompile(`needs\.(\w+)\.outputs\.([\w-]+)`)
)

// The examples are what callers copy, so they must match the workflow's current
// inputs and secrets.
func TestExamplesMatchTheWorkflow(t *testing.T) {
	files, err := filepath.Glob("../examples/*/*.yml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples found: %v", err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var c caller
		if err := yaml.Unmarshal(raw, &c); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for name, job := range c.Jobs {
			m := ownCall.FindStringSubmatch(job.Uses)
			if m == nil {
				continue
			}
			if !strings.HasSuffix(job.Uses, "@FULL_COMMIT_SHA") {
				t.Errorf("%s job %s must pin FULL_COMMIT_SHA for the caller to replace", file, name)
			}
			var w workflow
			own, err := os.ReadFile(filepath.Join("..", ".github", "workflows", m[1]))
			if err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(own, &w); err != nil {
				t.Fatal(err)
			}
			for input := range job.With {
				if _, ok := w.On.WorkflowCall.Inputs[input]; !ok {
					t.Errorf("%s job %s passes unknown input %q", file, name, input)
				}
			}
			for secret := range job.Secrets {
				if _, ok := w.On.WorkflowCall.Secrets[secret]; !ok {
					t.Errorf("%s job %s passes unknown secret %q", file, name, secret)
				}
			}
		}
	}
}

// Each arrangement is complete: separate workflows release on the tag event,
// the same workflow releases the tag its version job pushed.
func TestBothArrangementsReachTheRelease(t *testing.T) {
	read := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	separate := read("../examples/separate-workflows/release.yml")
	if !strings.Contains(separate, "tags: ['v*.*.*']") || strings.Contains(separate, "tag:") {
		t.Error("the separate release workflow must start from the tag event, not a tag input")
	}
	same := read("../examples/same-workflow/ci.yml")
	for _, want := range []string{
		"needs.version.outputs.tagged == 'true'", "tag: ${{ needs.version.outputs.tag }}", "contents: write",
	} {
		if !strings.Contains(same, want) {
			t.Errorf("the same-workflow example lacks %q", want)
		}
	}
	if strings.Contains(same, "release-app-client-id") {
		t.Error("the same-workflow example is the no-App arrangement")
	}
	for _, file := range []string{"../examples/separate-workflows/ci.yml", "../examples/same-workflow/ci.yml"} {
		for _, ref := range outputRef.FindAllStringSubmatch(read(file), -1) {
			if ref[1] == "version" && ref[2] != "tagged" && ref[2] != "tag" && ref[2] != "version" {
				t.Errorf("%s reads unknown version output %s", file, ref[2])
			}
		}
	}
}
