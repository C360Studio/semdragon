package cicontract

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	buildAndPushGuard = "github.event_name == 'push' && github.ref == 'refs/heads/main'"
	workflowPath      = ".github/workflows/ci.yml"
)

func TestCIWorkflowTrustBoundaries(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t)
	triggers := requireMappingValue(t, workflow, "on")

	pullRequest, ok := mappingValue(triggers, "pull_request")
	if !ok {
		t.Error("pull_request trigger is missing")
	} else {
		assertTriggerHasNoBaseFilter(t, pullRequest)
	}
	if _, ok := mappingValue(triggers, "pull_request_target"); ok {
		t.Error("pull_request_target must not be configured")
	}

	push := requireMappingValue(t, triggers, "push")
	assertPushIsMainOnly(t, push)

	globalPermissions := requireMappingValue(t, workflow, "permissions")
	assertExactPermissions(t, "global", globalPermissions, map[string]string{
		"contents": "read",
	})

	jobs := requireMappingValue(t, workflow, "jobs")
	buildAndPush := requireMappingValue(t, jobs, "build-and-push")
	buildPermissions := requireMappingValue(t, buildAndPush, "permissions")
	assertExactPermissions(t, "build-and-push", buildPermissions, map[string]string{
		"contents": "read",
		"packages": "write",
	})

	guard := requireScalarValue(t, buildAndPush, "if")
	if guard != buildAndPushGuard {
		t.Errorf("build-and-push guard = %q, want exact %q", guard, buildAndPushGuard)
	}

	allowedNonBuildJobs := map[string]bool{"test": true, "lint": true}
	seenBuildAndPush := false
	forEachMappingEntry(t, jobs, func(jobName string, job *yaml.Node) {
		if jobName == "build-and-push" {
			seenBuildAndPush = true
			return
		}
		if !allowedNonBuildJobs[jobName] {
			t.Errorf("non-build job %q is missing from the explicit unprivileged job inventory", jobName)
		}
		assertJobCannotPublish(t, jobName, job)
	})
	if !seenBuildAndPush {
		t.Error("build-and-push job is missing")
	}

	for _, jobName := range []string{"test", "lint"} {
		job := requireMappingValue(t, jobs, jobName)
		assertExactUnprivilegedJobSteps(t, jobName, job)
	}
}

func TestNonBuildJobPermissionMutationGuards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		jobYAML       string
		wantViolation bool
	}{
		{name: "inherits global read", jobYAML: `{}`, wantViolation: false},
		{name: "explicit read and none", jobYAML: `permissions: {contents: read, packages: none}`, wantViolation: false},
		{name: "read all", jobYAML: `permissions: read-all`, wantViolation: false},
		{name: "write all", jobYAML: `permissions: write-all`, wantViolation: true},
		{name: "contents write", jobYAML: `permissions: {contents: write}`, wantViolation: true},
		{name: "packages write", jobYAML: `permissions: {packages: write}`, wantViolation: true},
		{name: "id token write", jobYAML: `permissions: {id-token: write}`, wantViolation: true},
		{name: "attestations write", jobYAML: `permissions: {attestations: write}`, wantViolation: true},
		{name: "actions write", jobYAML: `permissions: {actions: write}`, wantViolation: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			job := parseYAMLMapping(t, tt.jobYAML)
			gotViolation := len(nonBuildPermissionViolations(job)) > 0
			if gotViolation != tt.wantViolation {
				t.Errorf("permission violations = %v, want violation %v",
					nonBuildPermissionViolations(job), tt.wantViolation)
			}
		})
	}
}

func TestSecretContextMutationGuards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expression string
		wantSecret bool
	}{
		{name: "dot context", expression: `${{ secrets.TOKEN }}`, wantSecret: true},
		{name: "single quoted bracket context", expression: `${{ secrets['TOKEN'] }}`, wantSecret: true},
		{name: "double quoted bracket context", expression: `${{ secrets["TOKEN"] }}`, wantSecret: true},
		{name: "spaced bracket context", expression: `${{ secrets [ 'TOKEN' ] }}`, wantSecret: true},
		{name: "whole context function", expression: `${{ toJSON(secrets) }}`, wantSecret: true},
		{name: "unrelated text", expression: `secrets management documentation`, wantSecret: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			node := &yaml.Node{Kind: yaml.ScalarNode, Value: tt.expression}
			if got := nodeContainsSecret(node); got != tt.wantSecret {
				t.Errorf("nodeContainsSecret(%q) = %v, want %v", tt.expression, got, tt.wantSecret)
			}
		})
	}
}

func TestPublishMechanismDenylistMutationGuards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		jobYAML     string
		wantBlocked bool
	}{
		{name: "docker login action", jobYAML: `steps: [{uses: docker/login-action@v4}]`, wantBlocked: true},
		{name: "docker build push action", jobYAML: `steps: [{uses: docker/build-push-action@v7}]`, wantBlocked: true},
		{name: "push input", jobYAML: `steps: [{uses: example/build@v1, with: {push: true}}]`, wantBlocked: true},
		{name: "docker push command", jobYAML: `steps: [{run: "docker push ghcr.io/example/image"}]`, wantBlocked: true},
		{name: "whitespace bypass", jobYAML: `steps: [{run: "docker   push ghcr.io/example/image"}]`, wantBlocked: true},
		{name: "ordinary test", jobYAML: `steps: [{run: "go test ./..."}]`, wantBlocked: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			job := parseYAMLMapping(t, tt.jobYAML)
			if got := nodeContainsPublishCapability(job); got != tt.wantBlocked {
				t.Errorf("publish defense-in-depth result = %v, want %v", got, tt.wantBlocked)
			}
		})
	}
}

func readWorkflow(t *testing.T) *yaml.Node {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve workflow contract test path")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	data, err := os.ReadFile(filepath.Join(repositoryRoot, workflowPath))
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parse %s: %v", workflowPath, err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("%s root is not a YAML mapping", workflowPath)
	}
	return document.Content[0]
}

func assertTriggerHasNoBaseFilter(t *testing.T, trigger *yaml.Node) {
	t.Helper()

	if isNullNode(trigger) {
		return
	}
	if trigger.Kind != yaml.MappingNode {
		t.Errorf("pull_request trigger kind = %v, want null or mapping", trigger.Kind)
		return
	}
	for _, key := range []string{"branches", "branches-ignore", "base"} {
		if _, ok := mappingValue(trigger, key); ok {
			t.Errorf("pull_request must apply to all base branches; found %q filter", key)
		}
	}
}

func assertPushIsMainOnly(t *testing.T, trigger *yaml.Node) {
	t.Helper()

	if trigger.Kind != yaml.MappingNode {
		t.Errorf("push trigger kind = %v, want mapping restricted to main", trigger.Kind)
		return
	}
	branches := requireMappingValue(t, trigger, "branches")
	if branches.Kind != yaml.SequenceNode {
		t.Errorf("push branches kind = %v, want sequence", branches.Kind)
		return
	}
	if len(branches.Content) != 1 || branches.Content[0].Value != "main" {
		var got []string
		for _, branch := range branches.Content {
			got = append(got, branch.Value)
		}
		t.Errorf("push branches = %v, want exactly [main]", got)
	}
	if _, ok := mappingValue(trigger, "branches-ignore"); ok {
		t.Error("push must use exactly the main allowlist, not branches-ignore")
	}
}

func assertExactPermissions(t *testing.T, scope string, node *yaml.Node, want map[string]string) {
	t.Helper()

	if node.Kind != yaml.MappingNode {
		t.Errorf("%s permissions kind = %v, want mapping", scope, node.Kind)
		return
	}
	got := make(map[string]string, len(node.Content)/2)
	forEachMappingEntry(t, node, func(name string, value *yaml.Node) {
		if value.Kind != yaml.ScalarNode {
			t.Errorf("%s permission %q kind = %v, want scalar", scope, name, value.Kind)
			return
		}
		got[name] = value.Value
	})
	if len(got) != len(want) {
		t.Errorf("%s permissions = %v, want exactly %v", scope, got, want)
	}
	for name, wantAccess := range want {
		if got[name] != wantAccess {
			t.Errorf("%s permission %q = %q, want %q", scope, name, got[name], wantAccess)
		}
	}
}

func assertJobCannotPublish(t *testing.T, jobName string, job *yaml.Node) {
	t.Helper()

	for _, violation := range nonBuildPermissionViolations(job) {
		t.Errorf("%s %s", jobName, violation)
	}
	if nodeContainsSecret(job) {
		t.Errorf("%s references a GitHub secrets context outside build-and-push", jobName)
	}
	// This finite mechanism inventory is defense-in-depth. The primary trust
	// boundary is the read-only permission check plus complete secrets scan.
	if nodeContainsPublishCapability(job) {
		t.Errorf("%s contains a forbidden login/push/publish mechanism outside build-and-push", jobName)
	}
}

func nonBuildPermissionViolations(job *yaml.Node) []string {
	permissions, ok := mappingValue(job, "permissions")
	if !ok {
		return nil
	}
	if permissions.Kind == yaml.ScalarNode {
		switch strings.ToLower(permissions.Value) {
		case "read-all":
			return nil
		case "write-all":
			return []string{"grants write-all outside build-and-push"}
		default:
			return []string{"has an unsupported scalar permission declaration"}
		}
	}
	if permissions.Kind != yaml.MappingNode {
		return []string{"permissions must be absent, read-all, or an explicit read-only mapping"}
	}
	var violations []string
	for i := 0; i+1 < len(permissions.Content); i += 2 {
		scope := permissions.Content[i].Value
		access := permissions.Content[i+1]
		if access.Kind != yaml.ScalarNode {
			violations = append(violations, "permission "+scope+" must be a scalar read or none value")
			continue
		}
		switch strings.ToLower(access.Value) {
		case "read", "none":
		case "write":
			violations = append(violations, "grants "+scope+": write outside build-and-push")
		default:
			violations = append(violations, "permission "+scope+" has unsupported access "+access.Value)
		}
	}
	return violations
}

var (
	githubExpressionPattern = regexp.MustCompile(`(?s)\$\{\{.*?\}\}`)
	secretContextPattern    = regexp.MustCompile(`(?i)\bsecrets\b`)
	secretAccessPattern     = regexp.MustCompile(`(?i)\bsecrets\s*(?:\.|\[)`)
)

func nodeContainsSecret(node *yaml.Node) bool {
	if node.Kind == yaml.ScalarNode {
		if strings.EqualFold(node.Value, "secrets") || secretAccessPattern.MatchString(node.Value) {
			return true
		}
		for _, expression := range githubExpressionPattern.FindAllString(node.Value, -1) {
			if secretContextPattern.MatchString(expression) {
				return true
			}
		}
		return false
	}
	for _, child := range node.Content {
		if nodeContainsSecret(child) {
			return true
		}
	}
	return false
}

func nodeContainsPublishCapability(node *yaml.Node) bool {
	if node.Kind == yaml.MappingNode {
		if uses, ok := mappingValue(node, "uses"); ok {
			action := strings.ToLower(uses.Value)
			for _, publishingAction := range []string{
				"docker/login-action",
				"docker/build-push-action",
				"pypa/gh-action-pypi-publish",
				"softprops/action-gh-release",
			} {
				if strings.Contains(action, publishingAction) {
					return true
				}
			}
		}
		if with, ok := mappingValue(node, "with"); ok && with.Kind == yaml.MappingNode {
			if push, ok := mappingValue(with, "push"); ok && strings.EqualFold(push.Value, "true") {
				return true
			}
		}
		if run, ok := mappingValue(node, "run"); ok {
			command := strings.Join(strings.Fields(strings.ToLower(run.Value)), " ")
			for _, publishingCommand := range []string{
				"docker login",
				"docker push",
				"npm publish",
				"pnpm publish",
				"yarn publish",
				"cargo publish",
				"twine upload",
				"helm push",
				"nuget push",
				"gh release create",
			} {
				if strings.Contains(command, publishingCommand) {
					return true
				}
			}
		}
	}
	for _, child := range node.Content {
		if nodeContainsPublishCapability(child) {
			return true
		}
	}
	return false
}

type workflowStepContract struct {
	name string
	uses string
	run  string
	with map[string]string
}

func assertExactUnprivilegedJobSteps(t *testing.T, jobName string, job *yaml.Node) {
	t.Helper()

	want, ok := map[string][]workflowStepContract{
		"test": {
			{uses: "actions/checkout@v6"},
			{uses: "actions/setup-go@v6", with: map[string]string{"go-version-file": "go.mod"}},
			{name: "Unit tests", run: "go test -race ./..."},
		},
		"lint": {
			{uses: "actions/checkout@v6"},
			{uses: "actions/setup-go@v6", with: map[string]string{"go-version-file": "go.mod"}},
			{name: "Install revive", run: "go install github.com/mgechev/revive@latest"},
			{name: "Revive", run: "revive -config revive.toml -formatter friendly ./..."},
			{name: "Vet", run: "go vet ./..."},
		},
	}[jobName]
	if !ok {
		t.Fatalf("no unprivileged step contract for job %q", jobName)
	}

	steps := requireMappingValue(t, job, "steps")
	if steps.Kind != yaml.SequenceNode {
		t.Fatalf("%s steps kind = %v, want sequence", jobName, steps.Kind)
	}
	if len(steps.Content) != len(want) {
		t.Errorf("%s steps = %d, want exact inventoried count %d", jobName, len(steps.Content), len(want))
		return
	}
	for index, step := range steps.Content {
		assertExactWorkflowStep(t, jobName, index, step, want[index])
	}
}

func assertExactWorkflowStep(
	t *testing.T,
	jobName string,
	index int,
	step *yaml.Node,
	want workflowStepContract,
) {
	t.Helper()

	if step.Kind != yaml.MappingNode {
		t.Errorf("%s step %d kind = %v, want mapping", jobName, index, step.Kind)
		return
	}
	for i := 0; i+1 < len(step.Content); i += 2 {
		switch step.Content[i].Value {
		case "name", "uses", "run", "with":
		default:
			t.Errorf("%s step %d has unreviewed key %q", jobName, index, step.Content[i].Value)
		}
	}
	assertOptionalScalar(t, jobName, index, step, "name", want.name)
	assertOptionalScalar(t, jobName, index, step, "uses", want.uses)
	assertOptionalScalar(t, jobName, index, step, "run", want.run)
	assertExactOptionalMapping(t, jobName, index, step, "with", want.with)
}

func assertOptionalScalar(
	t *testing.T,
	jobName string,
	index int,
	step *yaml.Node,
	key string,
	want string,
) {
	t.Helper()

	got, exists := mappingValue(step, key)
	if want == "" {
		if exists {
			t.Errorf("%s step %d has unexpected %s", jobName, index, key)
		}
		return
	}
	if !exists || got.Kind != yaml.ScalarNode || got.Value != want {
		var gotValue string
		if exists {
			gotValue = got.Value
		}
		t.Errorf("%s step %d %s = %q, want %q", jobName, index, key, gotValue, want)
	}
}

func assertExactOptionalMapping(
	t *testing.T,
	jobName string,
	index int,
	step *yaml.Node,
	key string,
	want map[string]string,
) {
	t.Helper()

	got, exists := mappingValue(step, key)
	if want == nil {
		if exists {
			t.Errorf("%s step %d has unexpected %s mapping", jobName, index, key)
		}
		return
	}
	if !exists || got.Kind != yaml.MappingNode {
		t.Errorf("%s step %d %s is missing or not a mapping", jobName, index, key)
		return
	}
	if len(got.Content)/2 != len(want) {
		t.Errorf("%s step %d %s entries = %d, want %d", jobName, index, key, len(got.Content)/2, len(want))
	}
	for name, wantValue := range want {
		value, ok := mappingValue(got, name)
		if !ok || value.Kind != yaml.ScalarNode || value.Value != wantValue {
			var gotValue string
			if ok {
				gotValue = value.Value
			}
			t.Errorf("%s step %d %s.%s = %q, want %q", jobName, index, key, name, gotValue, wantValue)
		}
	}
}

func requireMappingValue(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()

	value, ok := mappingValue(mapping, key)
	if !ok {
		t.Fatalf("missing YAML key %q", key)
	}
	return value
}

func requireScalarValue(t *testing.T, mapping *yaml.Node, key string) string {
	t.Helper()

	value := requireMappingValue(t, mapping, key)
	if value.Kind != yaml.ScalarNode {
		t.Fatalf("YAML key %q kind = %v, want scalar", key, value.Kind)
	}
	return value.Value
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1], true
		}
	}
	return nil, false
}

func forEachMappingEntry(t *testing.T, mapping *yaml.Node, visit func(string, *yaml.Node)) {
	t.Helper()

	if mapping.Kind != yaml.MappingNode {
		t.Fatalf("YAML node kind = %v, want mapping", mapping.Kind)
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		visit(mapping.Content[i].Value, mapping.Content[i+1])
	}
}

func isNullNode(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}

func parseYAMLMapping(t *testing.T, source string) *yaml.Node {
	t.Helper()

	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		t.Fatalf("parse YAML fixture: %v", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("YAML fixture root is not a mapping")
	}
	return document.Content[0]
}
