package tests

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type step struct {
	ID              string            `yaml:"id"`
	If              string            `yaml:"if"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	With            map[string]string `yaml:"with"`
	Env             map[string]string `yaml:"env"`
}

type workflow struct {
	On struct {
		WorkflowCall struct {
			Inputs  map[string]yaml.Node `yaml:"inputs"`
			Secrets map[string]yaml.Node `yaml:"secrets"`
			Outputs map[string]struct {
				Value string `yaml:"value"`
			} `yaml:"outputs"`
		} `yaml:"workflow_call"`
	} `yaml:"on"`
	Permissions yaml.Node `yaml:"permissions"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress *bool  `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		If          string            `yaml:"if"`
		Permissions yaml.Node         `yaml:"permissions"`
		Outputs     map[string]string `yaml:"outputs"`
		Steps       []step            `yaml:"steps"`
	} `yaml:"jobs"`
}

func readWorkflow(t *testing.T) workflow {
	t.Helper()
	raw, err := os.ReadFile("../.github/workflows/go-release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var w workflow
	if err := yaml.Unmarshal(raw, &w); err != nil {
		t.Fatal(err)
	}
	if len(w.Jobs["release"].Steps) == 0 {
		t.Fatal("the shared workflow has no release steps")
	}
	return w
}

func role(t *testing.T, steps []step, action string) int {
	t.Helper()
	found := -1
	for i, s := range steps {
		if strings.HasPrefix(s.Uses, action+"@") {
			if found != -1 {
				t.Fatalf("%s is called more than once", action)
			}
			found = i
		}
	}
	if found == -1 {
		t.Fatalf("the shared workflow does not call %s", action)
	}
	return found
}

func TestGoReleaseComposesActions(t *testing.T) {
	w := readWorkflow(t)
	steps := w.Jobs["release"].Steps
	checkout := role(t, steps, "actions/checkout")
	ci := role(t, steps, "michielvha/go-ci-action")
	release := role(t, steps, "michielvha/goreleaser-action")
	if checkout >= ci || ci >= release {
		t.Fatal("release must follow checkout and blocking Go CI")
	}
	for _, s := range steps {
		if s.Run != "" || s.ContinueOnError {
			t.Errorf("%s adds shell logic or advisory failure handling to the shared workflow", s.Uses)
		}
	}
	if steps[ci].ID == "" {
		t.Fatal("the Go CI step exposes no output identity")
	}
	want := "${{ steps." + steps[ci].ID + ".outputs.go-version }}"
	if steps[release].With["go-version"] != want {
		t.Error("publication does not use the Go version resolved by CI")
	}
}

func TestGoReleaseBuildsEventCommit(t *testing.T) {
	w := readWorkflow(t)
	job := w.Jobs["release"]
	checkout := job.Steps[role(t, job.Steps, "actions/checkout")]
	if checkout.With["ref"] != "${{ inputs.tag && format('refs/tags/{0}', inputs.tag) || github.sha }}" ||
		checkout.With["persist-credentials"] != "false" {
		t.Error("checkout must use the given tag or the tag event's commit, without persisting credentials")
	}
	for _, condition := range []string{
		"github.event_name == 'push'", "refs/tags/v", "!github.event.deleted",
	} {
		if !strings.Contains(job.If, condition) {
			t.Errorf("publication lacks its %q event guard", condition)
		}
	}
}

// Same-workflow arrangement: a version job created the tag in this run, so no
// tag event starts publication; the caller passes the tag instead.
func TestGoReleaseAcceptsSameWorkflowTag(t *testing.T) {
	w := readWorkflow(t)
	if _, ok := w.On.WorkflowCall.Inputs["tag"]; !ok {
		t.Fatal("the release workflow must accept the tag created earlier in the same run")
	}
	job := w.Jobs["release"]
	if !strings.Contains(job.If, "startsWith(inputs.tag, 'v')") {
		t.Error("a supplied tag must be an exact v-prefixed version tag")
	}
	release := job.Steps[role(t, job.Steps, "michielvha/goreleaser-action")]
	if release.Env["GORELEASER_CURRENT_TAG"] != "${{ inputs.tag }}" {
		t.Error("GoReleaser must release the supplied tag, not rediscover one on a branch run")
	}
	if !strings.Contains(w.Concurrency.Group, "inputs.tag") {
		t.Error("same-workflow releases must serialize per tag, not per branch")
	}
}

func TestGoReleaseLimitsSigningSecret(t *testing.T) {
	w := readWorkflow(t)
	if _, exists := w.On.WorkflowCall.Inputs["workflow-ref"]; exists {
		t.Error("the Go workflow must not require a second pin for helper checkout")
	}
	steps := w.Jobs["release"].Steps
	release := role(t, steps, "michielvha/goreleaser-action")
	for i, s := range steps {
		secret := s.With["pgp-private-key"]
		if i == release {
			if secret != "${{ secrets.pgp-private-key }}" {
				t.Error("the release action does not receive its explicit signing secret")
			}
		} else if secret != "" {
			t.Errorf("%s receives a signing key outside artifact publication", s.Uses)
		}
	}
}

func TestGoReleasePinsAndSerializes(t *testing.T) {
	w := readWorkflow(t)
	if w.Concurrency.CancelInProgress == nil || *w.Concurrency.CancelInProgress ||
		!strings.Contains(w.Concurrency.Group, "github.repository") ||
		!strings.Contains(w.Concurrency.Group, "github.ref") {
		t.Error("publication must serialize per repository/ref without cancelling an active release")
	}

	pinned := regexp.MustCompile(`@[0-9a-f]{40}$`)
	for _, s := range w.Jobs["release"].Steps {
		if !pinned.MatchString(s.Uses) {
			t.Errorf("%q is not an immutable action implementation", s.Uses)
		}
	}

}

func TestGoReleaseUsesCICache(t *testing.T) {
	w := readWorkflow(t)
	steps := w.Jobs["release"].Steps
	release := role(t, steps, "michielvha/goreleaser-action")
	if steps[release].With["go-cache"] != "false" {
		t.Error("the release action must reuse the Go CI cache without restoring it twice")
	}
}
