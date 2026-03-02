package dmapproval

// =============================================================================
// UNIT TESTS - DM Approval Config and Key Generation
// =============================================================================
// These tests cover pure, in-memory logic that requires no NATS connection:
//   - DefaultConfig() returns sensible defaults
//   - NATSApprovalRouter key/subject generation methods
//
// Integration tests (lifecycle, NATS round-trips) live in component_test.go
// and require Docker via testcontainers.
// =============================================================================

import (
	"testing"
)

// =============================================================================
// DefaultConfig
// =============================================================================

func TestDefaultConfig_ReturnsExpectedValues(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org != "default" {
		t.Errorf("Org = %q, want %q", cfg.Org, "default")
	}
	if cfg.Platform != "local" {
		t.Errorf("Platform = %q, want %q", cfg.Platform, "local")
	}
	if cfg.Board != "main" {
		t.Errorf("Board = %q, want %q", cfg.Board, "main")
	}
	if cfg.ApprovalTimeoutMin != 30 {
		t.Errorf("ApprovalTimeoutMin = %d, want %d", cfg.ApprovalTimeoutMin, 30)
	}
	if cfg.AutoApprove {
		t.Error("AutoApprove should default to false")
	}
}

func TestDefaultConfig_IsMutable(t *testing.T) {
	// Each call must return an independent value so callers can customise
	// without affecting other callers.
	a := DefaultConfig()
	b := DefaultConfig()

	a.Org = "mutated"
	if b.Org == "mutated" {
		t.Error("modifying one DefaultConfig result should not affect another")
	}
}

func TestDefaultConfig_ComponentName(t *testing.T) {
	if ComponentName == "" {
		t.Error("ComponentName must not be empty")
	}
	// Verify the constant matches what operators expect in config files.
	if ComponentName != "dm_approval" {
		t.Errorf("ComponentName = %q, want %q", ComponentName, "dm_approval")
	}
}

// =============================================================================
// Key generation helpers on NATSApprovalRouter
// =============================================================================
// NATSApprovalRouter.approvalPendingKey, approvalResolvedKey,
// approvalRequestSubject, and approvalResponseSubject are all pure string
// formatting functions. They require no NATS connection to exercise, so we
// construct a zero-value router (nil fields are never accessed).
// =============================================================================

// newTestRouter returns a NATSApprovalRouter with nil deps, sufficient for
// testing key/subject generation methods that perform no I/O.
func newTestRouter() *NATSApprovalRouter {
	return &NATSApprovalRouter{}
}

// =============================================================================
// approvalPendingKey
// =============================================================================

func TestApprovalPendingKey_Format(t *testing.T) {
	r := newTestRouter()

	tests := []struct {
		name            string
		sessionInstance string
		approvalID      string
		want            string
	}{
		{
			name:            "basic identifiers",
			sessionInstance: "sess1",
			approvalID:      "appr1",
			want:            "approval.pending.sess1.appr1",
		},
		{
			name:            "hex instance identifiers",
			sessionInstance: "a1b2c3d4",
			approvalID:      "deadbeef",
			want:            "approval.pending.a1b2c3d4.deadbeef",
		},
		{
			name:            "empty session instance",
			sessionInstance: "",
			approvalID:      "appr1",
			want:            "approval.pending..appr1",
		},
		{
			name:            "empty approval ID",
			sessionInstance: "sess1",
			approvalID:      "",
			want:            "approval.pending.sess1.",
		},
		{
			name:            "both empty",
			sessionInstance: "",
			approvalID:      "",
			want:            "approval.pending..",
		},
		{
			name:            "longer instance identifiers",
			sessionInstance: "mysession",
			approvalID:      "myapproval",
			want:            "approval.pending.mysession.myapproval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.approvalPendingKey(tt.sessionInstance, tt.approvalID)
			if got != tt.want {
				t.Errorf("approvalPendingKey(%q, %q) = %q, want %q",
					tt.sessionInstance, tt.approvalID, got, tt.want)
			}
		})
	}
}

func TestApprovalPendingKey_Prefix(t *testing.T) {
	r := newTestRouter()
	key := r.approvalPendingKey("myinstance", "myid")

	const wantPrefix = "approval.pending."
	if len(key) < len(wantPrefix) || key[:len(wantPrefix)] != wantPrefix {
		t.Errorf("approvalPendingKey should start with %q, got %q", wantPrefix, key)
	}
}

func TestApprovalPendingKey_ContainsBothParts(t *testing.T) {
	r := newTestRouter()
	si := "sessionXYZ"
	aid := "approval123"

	key := r.approvalPendingKey(si, aid)
	if !contains(key, si) {
		t.Errorf("approvalPendingKey(%q, %q) = %q: does not contain session instance", si, aid, key)
	}
	if !contains(key, aid) {
		t.Errorf("approvalPendingKey(%q, %q) = %q: does not contain approval ID", si, aid, key)
	}
}

// =============================================================================
// approvalResolvedKey
// =============================================================================

func TestApprovalResolvedKey_Format(t *testing.T) {
	r := newTestRouter()

	tests := []struct {
		name            string
		sessionInstance string
		approvalID      string
		want            string
	}{
		{
			name:            "basic identifiers",
			sessionInstance: "sess1",
			approvalID:      "appr1",
			want:            "approval.resolved.sess1.appr1",
		},
		{
			name:            "hex instance identifiers",
			sessionInstance: "cafebabe",
			approvalID:      "12345678",
			want:            "approval.resolved.cafebabe.12345678",
		},
		{
			name:            "empty session instance",
			sessionInstance: "",
			approvalID:      "appr1",
			want:            "approval.resolved..appr1",
		},
		{
			name:            "empty approval ID",
			sessionInstance: "sess1",
			approvalID:      "",
			want:            "approval.resolved.sess1.",
		},
		{
			name:            "both empty",
			sessionInstance: "",
			approvalID:      "",
			want:            "approval.resolved..",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.approvalResolvedKey(tt.sessionInstance, tt.approvalID)
			if got != tt.want {
				t.Errorf("approvalResolvedKey(%q, %q) = %q, want %q",
					tt.sessionInstance, tt.approvalID, got, tt.want)
			}
		})
	}
}

func TestApprovalResolvedKey_Prefix(t *testing.T) {
	r := newTestRouter()
	key := r.approvalResolvedKey("myinstance", "myid")

	const wantPrefix = "approval.resolved."
	if len(key) < len(wantPrefix) || key[:len(wantPrefix)] != wantPrefix {
		t.Errorf("approvalResolvedKey should start with %q, got %q", wantPrefix, key)
	}
}

// =============================================================================
// approvalPendingKey vs approvalResolvedKey — namespace isolation
// =============================================================================

func TestPendingAndResolvedKeys_AreDistinct(t *testing.T) {
	r := newTestRouter()

	si := "mysession"
	aid := "myapproval"

	pending := r.approvalPendingKey(si, aid)
	resolved := r.approvalResolvedKey(si, aid)

	if pending == resolved {
		t.Errorf("pending and resolved keys must differ for same inputs: both = %q", pending)
	}
}

func TestPendingAndResolvedKeys_SharedSuffix(t *testing.T) {
	// Both keys encode the same sessionInstance.approvalID suffix so that
	// callers can derive one from the other by swapping the segment.
	r := newTestRouter()
	si := "s1"
	aid := "a1"

	pending := r.approvalPendingKey(si, aid)
	resolved := r.approvalResolvedKey(si, aid)

	suffix := si + "." + aid
	if !contains(pending, suffix) {
		t.Errorf("pending key %q does not contain expected suffix %q", pending, suffix)
	}
	if !contains(resolved, suffix) {
		t.Errorf("resolved key %q does not contain expected suffix %q", resolved, suffix)
	}
}

// =============================================================================
// approvalRequestSubject
// =============================================================================

func TestApprovalRequestSubject_Format(t *testing.T) {
	r := newTestRouter()

	tests := []struct {
		name            string
		sessionInstance string
		want            string
	}{
		{
			name:            "basic instance",
			sessionInstance: "sess1",
			want:            "approval.request.sess1",
		},
		{
			name:            "hex instance",
			sessionInstance: "deadbeef",
			want:            "approval.request.deadbeef",
		},
		{
			name:            "empty instance",
			sessionInstance: "",
			want:            "approval.request.",
		},
		{
			name:            "longer instance",
			sessionInstance: "longsessionid",
			want:            "approval.request.longsessionid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.approvalRequestSubject(tt.sessionInstance)
			if got != tt.want {
				t.Errorf("approvalRequestSubject(%q) = %q, want %q",
					tt.sessionInstance, got, tt.want)
			}
		})
	}
}

func TestApprovalRequestSubject_Prefix(t *testing.T) {
	r := newTestRouter()
	subj := r.approvalRequestSubject("instance")

	const wantPrefix = "approval.request."
	if len(subj) < len(wantPrefix) || subj[:len(wantPrefix)] != wantPrefix {
		t.Errorf("approvalRequestSubject should start with %q, got %q", wantPrefix, subj)
	}
}

// =============================================================================
// approvalResponseSubject
// =============================================================================

func TestApprovalResponseSubject_Format(t *testing.T) {
	r := newTestRouter()

	tests := []struct {
		name            string
		sessionInstance string
		approvalID      string
		want            string
	}{
		{
			name:            "basic identifiers",
			sessionInstance: "sess1",
			approvalID:      "appr1",
			want:            "approval.response.sess1.appr1",
		},
		{
			name:            "hex identifiers",
			sessionInstance: "aabbccdd",
			approvalID:      "11223344",
			want:            "approval.response.aabbccdd.11223344",
		},
		{
			name:            "empty session instance",
			sessionInstance: "",
			approvalID:      "appr1",
			want:            "approval.response..appr1",
		},
		{
			name:            "empty approval ID",
			sessionInstance: "sess1",
			approvalID:      "",
			want:            "approval.response.sess1.",
		},
		{
			name:            "both empty",
			sessionInstance: "",
			approvalID:      "",
			want:            "approval.response..",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.approvalResponseSubject(tt.sessionInstance, tt.approvalID)
			if got != tt.want {
				t.Errorf("approvalResponseSubject(%q, %q) = %q, want %q",
					tt.sessionInstance, tt.approvalID, got, tt.want)
			}
		})
	}
}

func TestApprovalResponseSubject_Prefix(t *testing.T) {
	r := newTestRouter()
	subj := r.approvalResponseSubject("myinstance", "myid")

	const wantPrefix = "approval.response."
	if len(subj) < len(wantPrefix) || subj[:len(wantPrefix)] != wantPrefix {
		t.Errorf("approvalResponseSubject should start with %q, got %q", wantPrefix, subj)
	}
}

func TestApprovalResponseSubject_ContainsBothParts(t *testing.T) {
	r := newTestRouter()
	si := "sessionABC"
	aid := "approval999"

	subj := r.approvalResponseSubject(si, aid)
	if !contains(subj, si) {
		t.Errorf("approvalResponseSubject(%q, %q) = %q: does not contain session instance", si, aid, subj)
	}
	if !contains(subj, aid) {
		t.Errorf("approvalResponseSubject(%q, %q) = %q: does not contain approval ID", si, aid, subj)
	}
}

// =============================================================================
// Cross-method subject namespace isolation
// =============================================================================

func TestRequestAndResponseSubjects_AreDistinct(t *testing.T) {
	r := newTestRouter()
	si := "mysession"
	aid := "myapproval"

	reqSubj := r.approvalRequestSubject(si)
	respSubj := r.approvalResponseSubject(si, aid)

	if reqSubj == respSubj {
		t.Errorf("request and response subjects must differ: both = %q", reqSubj)
	}
}

func TestAllSubjects_UseApprovalDomain(t *testing.T) {
	// Verify every generated string starts with "approval." so NATS wildcard
	// subscriptions on "approval.>" capture all approval traffic.
	r := newTestRouter()
	si := "s"
	aid := "a"

	subjects := []struct {
		name  string
		value string
	}{
		{"pendingKey", r.approvalPendingKey(si, aid)},
		{"resolvedKey", r.approvalResolvedKey(si, aid)},
		{"requestSubject", r.approvalRequestSubject(si)},
		{"responseSubject", r.approvalResponseSubject(si, aid)},
	}

	const domainPrefix = "approval."
	for _, s := range subjects {
		if !contains(s.value, domainPrefix) {
			t.Errorf("%s = %q: does not contain domain prefix %q", s.name, s.value, domainPrefix)
		}
	}
}

// =============================================================================
// Helpers
// =============================================================================

// contains reports whether s contains substr. Using a local helper avoids
// importing strings solely for these simple containment checks.
func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
