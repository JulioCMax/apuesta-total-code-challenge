// Package workflows_test contains grep-level, static assertions against the
// GitHub Actions workflow files under .github/workflows/. These are the
// "cheap and catches the exact regression that matters" checks the design
// calls for (threat matrix: fork PRs with repository authority, long-lived
// cloud credentials in a public repository) — not a full workflow-semantics
// test, which would require actually running the workflow (see task 1.9).
package workflows_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

// repoRoot locates the repository root relative to this test file's own
// path, so the test behaves the same regardless of the working directory
// `go test` is invoked from.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine the path of workflows_test.go via runtime.Caller")
	}
	// thisFile: <root>/internal/platform/workflows/workflows_test.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

func readWorkflow(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), ".github", "workflows", name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(content)
}

func TestWorkflows_ParseAsValidYAML(t *testing.T) {
	for _, name := range []string{"ci.yml", "deploy.yml"} {
		t.Run(name, func(t *testing.T) {
			content := readWorkflow(t, name)
			var doc map[string]any
			if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
				t.Fatalf("%s is not valid YAML: %v", name, err)
			}
			if len(doc) == 0 {
				t.Fatalf("%s parsed to an empty document", name)
			}
		})
	}
}

func TestDeployWorkflow_NeverReferencesLongLivedCredentials(t *testing.T) {
	content := readWorkflow(t, "deploy.yml")

	for _, forbidden := range []string{"aws-access-key-id", "aws-secret-access-key"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("deploy.yml must never reference %q — this repository is public, "+
				"and OIDC role assumption (ADR-0011) is the only supported credential path", forbidden)
		}
	}
}

func TestDeployWorkflow_TriggersOnlyOnWorkflowDispatch(t *testing.T) {
	content := readWorkflow(t, "deploy.yml")

	var doc struct {
		On any `yaml:"on"`
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("deploy.yml is not valid YAML: %v", err)
	}

	triggers, ok := doc.On.(map[string]any)
	if !ok {
		t.Fatalf("deploy.yml's on: block has an unexpected shape: %#v", doc.On)
	}

	if _, has := triggers["workflow_dispatch"]; !has {
		t.Error("deploy.yml must declare workflow_dispatch as a trigger")
	}
	for _, forbidden := range []string{"push", "pull_request", "pull_request_target"} {
		if _, has := triggers[forbidden]; has {
			t.Errorf("deploy.yml must not list %q as a trigger — workflow_dispatch only", forbidden)
		}
	}
}

func TestCIWorkflow_RequestsNoIDToken(t *testing.T) {
	content := readWorkflow(t, "ci.yml")

	var doc struct {
		Permissions map[string]string `yaml:"permissions"`
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("ci.yml is not valid YAML: %v", err)
	}

	// Structural check on the parsed permissions map, not a raw substring
	// search: this workflow's own explanatory comments legitimately mention
	// "id-token" in prose (see the design.md D30 note above the permissions
	// block), which a naive grep-on-full-file would misfire on.
	if _, has := doc.Permissions["id-token"]; has {
		t.Error("ci.yml must never request the id-token permission — only deploy.yml " +
			"(human-triggered via workflow_dispatch) may hold OIDC authority (design.md D30)")
	}
}

func TestCIWorkflow_TriggersOnPushAndPullRequest(t *testing.T) {
	content := readWorkflow(t, "ci.yml")

	var doc struct {
		On any `yaml:"on"`
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("ci.yml is not valid YAML: %v", err)
	}

	triggers, ok := doc.On.(map[string]any)
	if !ok {
		t.Fatalf("ci.yml's on: block has an unexpected shape: %#v", doc.On)
	}

	for _, want := range []string{"push", "pull_request"} {
		if _, has := triggers[want]; !has {
			t.Errorf("ci.yml must trigger on %q", want)
		}
	}
}

func TestWorkflows_NeverUsePullRequestTarget(t *testing.T) {
	for _, name := range []string{"ci.yml", "deploy.yml"} {
		t.Run(name, func(t *testing.T) {
			content := readWorkflow(t, name)

			var doc struct {
				On any `yaml:"on"`
			}
			if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
				t.Fatalf("%s is not valid YAML: %v", name, err)
			}

			// Structural check on the parsed on: trigger map, not a raw
			// substring search: ci.yml's own explanatory comments
			// legitimately mention "pull_request_target" in prose to
			// document why it is avoided.
			triggers, ok := doc.On.(map[string]any)
			if !ok {
				t.Fatalf("%s's on: block has an unexpected shape: %#v", name, doc.On)
			}
			if _, has := triggers["pull_request_target"]; has {
				t.Errorf("%s must never use pull_request_target — it would run fork PR "+
					"code with repository authority (threat matrix)", name)
			}
		})
	}
}

func TestCIWorkflow_RunsRaceTestsAgainstDynamoLocal(t *testing.T) {
	content := readWorkflow(t, "ci.yml")

	if !strings.Contains(content, "go test -race ./...") {
		t.Error("ci.yml must run `go test -race ./...` so the DynamoDB-gated " +
			"integration and concurrency tests execute, not just unit tests")
	}
	if !strings.Contains(content, "amazon/dynamodb-local") {
		t.Error("ci.yml must provision an amazon/dynamodb-local instance so the " +
			"integration tests run instead of emitting a skip banner")
	}
}
