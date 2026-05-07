package filter

import (
	"strings"
	"testing"
)

func TestTerraformPassthroughOnEmpty(t *testing.T) {
	if got := Terraform(nil, ""); got != "" {
		t.Errorf("Terraform(empty) = %q, want empty", got)
	}
}

func TestTerraformDropsRefreshSpam(t *testing.T) {
	in := `aws_iam_role.foo: Refreshing state... [id=foo]
aws_iam_role.bar: Refreshing state... [id=bar]
aws_iam_role.baz: Refreshing state... [id=baz]
data.aws_caller_identity.current: Reading...
data.aws_caller_identity.current: Read complete after 0s [id=acct-id]

Terraform will perform the following actions:

  # aws_iam_role.foo will be updated in-place
  ~ resource "aws_iam_role" "foo" {
        id   = "foo"
      ~ tags = {
          + "added" = "value"
        }
    }

Plan: 0 to add, 1 to change, 0 to destroy.
`
	out := Terraform(nil, in)
	if len(out) >= len(in) {
		t.Errorf("expected refresh stripping, got %d -> %d\n%s", len(in), len(out), out)
	}
	// Refresh lines must be gone.
	if strings.Contains(out, "Refreshing state") {
		t.Errorf("refresh lines not stripped:\n%s", out)
	}
	// Reading lines must be gone.
	if strings.Contains(out, "Reading...") || strings.Contains(out, "Read complete") {
		t.Errorf("reading lines not stripped:\n%s", out)
	}
	// Resource diff and plan summary must be preserved verbatim.
	for _, must := range []string{
		"aws_iam_role", "added", "value",
		"Plan: 0 to add, 1 to change, 0 to destroy",
	} {
		if !strings.Contains(out, must) {
			t.Errorf("lossless violation: %q missing\n%s", must, out)
		}
	}
}

func TestTerraformPreservesErrors(t *testing.T) {
	// Even though "Reading..." substring would normally drop, an error
	// containing "Reading" must not be dropped.
	in := `aws_iam_role.foo: Refreshing state... [id=foo]
Error: Could not read aws_iam_role.foo
  Reading state failed: insufficient permissions
`
	out := Terraform(nil, in)
	for _, must := range []string{"Error:", "Could not read", "insufficient permissions"} {
		if !strings.Contains(out, must) {
			t.Errorf("error/warning dropped: %q missing\n%s", must, out)
		}
	}
}

func TestTerraformDropsApplyProgress(t *testing.T) {
	in := `aws_lambda_function.x: Creating...
aws_lambda_function.x: Still creating... [10s elapsed]
aws_lambda_function.x: Still creating... [20s elapsed]
aws_lambda_function.x: Creation complete after 25s [id=arn:aws:lambda:...]

Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
`
	out := Terraform(nil, in)
	if strings.Contains(out, "Still creating") {
		t.Errorf("Still creating not stripped:\n%s", out)
	}
	if strings.Contains(out, "Creation complete") {
		t.Errorf("Creation complete not stripped:\n%s", out)
	}
	// The headline summary must be kept.
	if !strings.Contains(out, "Apply complete!") {
		t.Errorf("Apply summary dropped:\n%s", out)
	}
}

func TestTerraformPassthroughWhenNothingToStrip(t *testing.T) {
	// terraform validate / fmt output has none of the stripped patterns.
	in := "Success! The configuration is valid.\n"
	out := Terraform(nil, in)
	if out != in {
		t.Errorf("non-plan output should pass through, got: %q", out)
	}
}

func TestTerraformLosslessContract(t *testing.T) {
	in := "some plain output\nwith no patterns\n"
	out := Terraform(nil, in)
	if len(out) > len(in) {
		t.Errorf("inflated: %d -> %d", len(in), len(out))
	}
}
