package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"eks-control-plane/backend/internal/models"
)

// imageResolverFunc adapts a closure into a RollbackImageResolver so each test
// can inject its own resolve behaviour (image, error, or "must not be called").
type imageResolverFunc func(ctx context.Context, namespace, name string, revision int64) (string, error)

func (resolverFunction imageResolverFunc) ResolveRollbackImage(ctx context.Context, namespace, name string, revision int64) (string, error) {
	return resolverFunction(ctx, namespace, name, revision)
}

// resolverMustNotBeCalled returns a resolver that fails the test if invoked.
// Used to assert structural denies short-circuit before any resolve work.
func resolverMustNotBeCalled(t *testing.T) RollbackImageResolver {
	t.Helper()
	return imageResolverFunc(func(_ context.Context, _, _ string, _ int64) (string, error) {
		t.Fatalf("resolver was called; expected structural deny to short-circuit")
		return "", nil
	})
}

// resolverReturning builds a resolver that returns a fixed image.
func resolverReturning(image string) RollbackImageResolver {
	return imageResolverFunc(func(_ context.Context, _, _ string, _ int64) (string, error) {
		return image, nil
	})
}

// resolverFailing builds a resolver that returns a fixed error.
func resolverFailing(err error) RollbackImageResolver {
	return imageResolverFunc(func(_ context.Context, _, _ string, _ int64) (string, error) {
		return "", err
	})
}

// Scenario: parseImageVersion covers the full spec table — happy paths,
// digest-pinned variants, non-vN tags, case-sensitivity, and missing tags.
// Mirrors the rules in the implementation spec one-for-one so a change to the
// parser shows up here, not at a higher layer.
func TestParseImageVersion_TableCases(t *testing.T) {
	cases := []struct {
		image       string
		wantVersion int
		wantOK      bool
	}{
		{"foo:v4", 4, true},
		{"registry:5000/repo:v4", 4, true},
		{"foo:v4@sha256:abc", 4, true},
		{"foo@sha256:abc", 0, false},
		{"foo:latest", 0, false},
		{"foo:v4-amd64", 0, false},
		{"foo:V4", 0, false},
		{"foo", 0, false},
		{"", 0, false},
		{"foo:v0", 0, true},
	}
	for _, testCase := range cases {
		gotVersion, gotOK := parseImageVersion(testCase.image)
		if gotVersion != testCase.wantVersion || gotOK != testCase.wantOK {
			t.Errorf("parseImageVersion(%q) = (%d, %v), want (%d, %v)",
				testCase.image, gotVersion, gotOK, testCase.wantVersion, testCase.wantOK)
		}
	}
}

// Scenario: DefaultPolicy().RollbackImageFloors contains exactly the three
// production floors and nothing else. A regression that drops one of the floors
// or silently adds a new one would weaken the production guardrail unnoticed.
func TestDefaultPolicy_RollbackImageFloorsExact(t *testing.T) {
	floors := DefaultPolicy().RollbackImageFloors
	expected := map[string]int{
		"agent":    6,
		"backend":  6,
		"frontend": 8,
	}
	if len(floors) != len(expected) {
		t.Fatalf("RollbackImageFloors has %d entries, want %d: %+v", len(floors), len(expected), floors)
	}
	for deploymentName, expectedFloor := range expected {
		actualFloor, present := floors[deploymentName]
		if !present {
			t.Errorf("floor for %q missing", deploymentName)
			continue
		}
		if actualFloor != expectedFloor {
			t.Errorf("floor[%q] = %d, want %d", deploymentName, actualFloor, expectedFloor)
		}
	}
}

// Scenario: mutating the policy.RollbackImageFloors map AFTER New() must not
// change subsequent enforcer decisions. The defensive copy in New() is what
// makes the policy a static safety boundary; without it, code holding a
// reference to the original map could widen the guardrail post-construction.
func TestEnforcer_RollbackImageFloors_DefensiveCopy(t *testing.T) {
	originalFloors := map[string]int{"agent": 6}
	policy := Policy{
		AllowedNamespaces:   []string{allowedNamespace},
		RollbackImageFloors: originalFloors,
	}
	enforcer := New(policy, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))

	// Mutate the caller's map: add a new entry and raise the existing one.
	originalFloors["agent"] = 100
	originalFloors["newcomer"] = 50

	// "agent" with v6 must still allow — the enforcer's snapshot still has 6.
	decision := enforcer.Rollback(context.Background(),
		models.RollbackRequest{Namespace: allowedNamespace, Name: "agent", Revision: 1},
		resolverReturning("repo:v6"))
	if !decision.Allow {
		t.Errorf("post-mutation rollback to v6 was denied; defensive copy did not seal floor: %+v", decision)
	}

	// "newcomer" added to the caller's map after construction must not show up
	// in the enforcer's policy — there's no floor, so any image (or nil
	// resolver path) is allowed without consulting the resolver.
	decision = enforcer.Rollback(context.Background(),
		models.RollbackRequest{Namespace: allowedNamespace, Name: "newcomer", Revision: 1},
		resolverMustNotBeCalled(t))
	if !decision.Allow {
		t.Errorf("post-mutation rollback to newcomer was denied; defensive copy leaked: %+v", decision)
	}
}

// Scenario: structural checks must run before the floor check. An invalid name,
// a negative revision, or an off-allowlist namespace all deny without ever
// asking the resolver to do work — the resolver stub fails the test if called.
func TestEnforcer_Rollback_StructuralBeforeResolver(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	resolver := resolverMustNotBeCalled(t)

	structuralCases := []struct {
		name    string
		request models.RollbackRequest
	}{
		{
			name:    "invalid name",
			request: models.RollbackRequest{Namespace: allowedNamespace, Name: "Agent", Revision: 1},
		},
		{
			name:    "off-allowlist namespace",
			request: models.RollbackRequest{Namespace: "kube-system", Name: "agent", Revision: 1},
		},
		{
			name:    "negative revision",
			request: models.RollbackRequest{Namespace: allowedNamespace, Name: "agent", Revision: -1},
		},
		{
			name:    "invalid namespace",
			request: models.RollbackRequest{Namespace: "BadNs", Name: "agent", Revision: 1},
		},
	}
	for _, testCase := range structuralCases {
		decision := enforcer.Rollback(context.Background(), testCase.request, resolver)
		if decision.Allow {
			t.Errorf("%s: decision = %+v, want deny", testCase.name, decision)
		}
	}
}

// Scenario: Deployment name has no entry in the floor table → allow, and the
// resolver is never consulted. The floor map is the *only* trigger for the
// extra check; deployments outside it use the structural-only path.
func TestEnforcer_Rollback_NoFloorForDeployment(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Rollback(context.Background(),
		models.RollbackRequest{Namespace: allowedNamespace, Name: "someother", Revision: 1},
		resolverMustNotBeCalled(t))
	if !decision.Allow {
		t.Errorf("decision = %+v, want allow when name has no floor", decision)
	}
}

// Scenario: image exactly at the floor → allow. The comparison is `< floor`
// (deny), so equality must permit. v6 against floor 6 is the canonical edge.
func TestEnforcer_Rollback_AtFloor(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Rollback(context.Background(),
		models.RollbackRequest{Namespace: allowedNamespace, Name: "agent", Revision: 1},
		resolverReturning("repo:v6"))
	if !decision.Allow {
		t.Errorf("decision = %+v, want allow at floor", decision)
	}
}

// Scenario: image above the floor → allow. A double-digit tag confirms the
// parser handles >9 (no off-by-one against single-digit assumptions).
func TestEnforcer_Rollback_AboveFloor(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Rollback(context.Background(),
		models.RollbackRequest{Namespace: allowedNamespace, Name: "agent", Revision: 1},
		resolverReturning("repo:v10"))
	if !decision.Allow {
		t.Errorf("decision = %+v, want allow above floor", decision)
	}
}

// Scenario: image strictly below the floor → deny, with the exact reason
// string the spec mandates. The substring assertion plus the equality check
// pins both the format and the values: image, floor, deployment name.
func TestEnforcer_Rollback_BelowFloor(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Rollback(context.Background(),
		models.RollbackRequest{Namespace: allowedNamespace, Name: "agent", Revision: 1},
		resolverReturning("repo:v5"))
	if decision.Allow {
		t.Fatalf("decision = %+v, want deny below floor", decision)
	}
	expectedReason := fmt.Sprintf("target image %s is below floor v%d for deployment %s", "repo:v5", 6, "agent")
	if decision.Reason != expectedReason {
		t.Errorf("reason = %q, want %q", decision.Reason, expectedReason)
	}
}

// Scenario: an image whose tag does not match ^v(\d+)$ exactly → deny with the
// unparseable reason. Covers each unparseable category called out in the spec:
// digest pin, latest, suffixed tag (v4-amd64), wrong case (V4).
func TestEnforcer_Rollback_UnparseableImage(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	unparseableImages := []string{
		"repo@sha256:abc",
		"repo:latest",
		"repo:v4-amd64",
		"repo:V4",
	}
	for _, image := range unparseableImages {
		decision := enforcer.Rollback(context.Background(),
			models.RollbackRequest{Namespace: allowedNamespace, Name: "agent", Revision: 1},
			resolverReturning(image))
		if decision.Allow {
			t.Errorf("image %q: decision = %+v, want deny", image, decision)
			continue
		}
		expectedReason := "target image " + image + " has no parseable v<int> tag"
		if decision.Reason != expectedReason {
			t.Errorf("image %q: reason = %q, want %q", image, decision.Reason, expectedReason)
		}
	}
}

// Scenario: resolver returns an error → deny with the wrapped reason. The
// "could not resolve target image: " prefix is the contract the route layer
// relies on to render the failure as a guardrail outcome (not a 500).
func TestEnforcer_Rollback_ResolverError(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Rollback(context.Background(),
		models.RollbackRequest{Namespace: allowedNamespace, Name: "agent", Revision: 1},
		resolverFailing(errors.New("boom")))
	if decision.Allow {
		t.Fatalf("decision = %+v, want deny on resolver error", decision)
	}
	if decision.Reason != "could not resolve target image: boom" {
		t.Errorf("reason = %q, want %q", decision.Reason, "could not resolve target image: boom")
	}
}

// Scenario: every Rollback decision — allow, structural deny, floor deny, and
// resolver-error deny — emits exactly one guardrail.decision audit line. The
// single-audit-per-call contract is what makes the log a credible trail; a
// duplicated allow+deny pair would falsely show both outcomes for one request.
func TestEnforcer_Rollback_SingleAuditPerCall(t *testing.T) {
	cases := []struct {
		name     string
		request  models.RollbackRequest
		resolver RollbackImageResolver
	}{
		{
			name:     "allow path (no floor)",
			request:  models.RollbackRequest{Namespace: allowedNamespace, Name: "someother", Revision: 1},
			resolver: nil,
		},
		{
			name:     "structural deny",
			request:  models.RollbackRequest{Namespace: "kube-system", Name: "agent", Revision: 1},
			resolver: nil,
		},
		{
			name:     "floor deny",
			request:  models.RollbackRequest{Namespace: allowedNamespace, Name: "agent", Revision: 1},
			resolver: resolverReturning("repo:v5"),
		},
		{
			name:     "resolver-error deny",
			request:  models.RollbackRequest{Namespace: allowedNamespace, Name: "agent", Revision: 1},
			resolver: resolverFailing(errors.New("boom")),
		},
	}
	for _, testCase := range cases {
		logBuffer := &bytes.Buffer{}
		enforcer := New(DefaultPolicy(), slog.New(slog.NewJSONHandler(logBuffer, nil)))
		enforcer.Rollback(context.Background(), testCase.request, testCase.resolver)

		auditLines := strings.Split(strings.TrimSpace(logBuffer.String()), "\n")
		if len(auditLines) != 1 {
			t.Fatalf("%s: expected exactly 1 audit line, got %d:\n%s",
				testCase.name, len(auditLines), logBuffer.String())
		}
		var auditEntry map[string]any
		if err := json.Unmarshal([]byte(auditLines[0]), &auditEntry); err != nil {
			t.Fatalf("%s: audit line not JSON: %v", testCase.name, err)
		}
		if auditEntry["msg"] != "guardrail.decision" || auditEntry["action"] != "rollback" {
			t.Errorf("%s: audit shape = %v", testCase.name, auditEntry)
		}
	}
}
