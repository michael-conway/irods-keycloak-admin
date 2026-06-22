package repair

import "testing"

func TestMirrorPolicyNormalizesRootAndManagedPaths(t *testing.T) {
	policy := newMirrorPathPolicy("kc-irods/")

	if policy.Root() != "/kc-irods" {
		t.Fatalf("unexpected root: %q", policy.Root())
	}
	if got := policy.GroupPath("/project-alpha/"); got != "/kc-irods/project-alpha" {
		t.Fatalf("unexpected group path: %q", got)
	}
	if got := policy.GroupNameFromPath("kc-irods/project-alpha/"); got != "project-alpha" {
		t.Fatalf("unexpected group name: %q", got)
	}
	if !policy.IsManagedPath("kc-irods/project-alpha/") {
		t.Fatal("expected managed path match")
	}
	if policy.IsManagedPath("/other/project-alpha") {
		t.Fatal("unexpected managed path match for unrelated subtree")
	}
}
