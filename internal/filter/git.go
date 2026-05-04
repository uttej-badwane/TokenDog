package filter

import (
	"fmt"
	"strings"
)

func Git(subcommand string, output string) string {
	switch subcommand {
	case "status":
		return gitStatus(output)
	case "log":
		return gitLog(output)
	case "diff":
		return gitDiff(output)
	case "branch":
		return gitBranch(output)
	default:
		return output
	}
}

func gitStatus(output string) string {
	lines := strings.Split(output, "\n")

	var branch string
	var modified, added, deleted, untracked []string
	recognized := false

	var section string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "On branch "):
			branch = strings.TrimPrefix(line, "On branch ")
			recognized = true
		case strings.HasPrefix(line, "Your branch is up to date with '"):
			remote := strings.TrimPrefix(line, "Your branch is up to date with '")
			remote = strings.TrimSuffix(remote, "'.")
			branch += " ↑ " + remote
		case strings.HasPrefix(line, "Your branch is ahead"):
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "by" && i+1 < len(parts) {
					branch += fmt.Sprintf(" (↑%s)", parts[i+1])
					break
				}
			}
		case strings.HasPrefix(line, "Your branch is behind"):
			branch += " (↓ behind)"
		case strings.HasPrefix(line, "Changes to be committed"):
			section = "staged"
		case strings.HasPrefix(line, "Changes not staged"):
			section = "unstaged"
		case strings.HasPrefix(line, "Untracked files"):
			section = "untracked"
		case strings.HasPrefix(line, "\t"):
			file := strings.TrimPrefix(line, "\t")
			switch section {
			case "untracked":
				untracked = append(untracked, file)
			case "staged", "unstaged":
				switch {
				case strings.Contains(file, "modified:"):
					modified = append(modified, strings.TrimSpace(strings.SplitN(file, "modified:", 2)[1]))
				case strings.Contains(file, "new file:"):
					added = append(added, strings.TrimSpace(strings.SplitN(file, "new file:", 2)[1]))
				case strings.Contains(file, "deleted:"):
					deleted = append(deleted, strings.TrimSpace(strings.SplitN(file, "deleted:", 2)[1]))
				}
			}
		case trimmed == "" || strings.HasPrefix(trimmed, "(") || strings.HasPrefix(trimmed, "no changes"):
			// skip hints and empty lines
		}
	}

	var sb strings.Builder
	if branch != "" {
		sb.WriteString(fmt.Sprintf("branch: %s\n", branch))
	}
	if len(added) > 0 {
		sb.WriteString(fmt.Sprintf("new(%d): %s\n", len(added), joinTrunc(added, 5)))
	}
	if len(modified) > 0 {
		sb.WriteString(fmt.Sprintf("modified(%d): %s\n", len(modified), joinTrunc(modified, 5)))
	}
	if len(deleted) > 0 {
		sb.WriteString(fmt.Sprintf("deleted(%d): %s\n", len(deleted), joinTrunc(deleted, 5)))
	}
	if len(untracked) > 0 {
		sb.WriteString(fmt.Sprintf("untracked(%d): %s\n", len(untracked), joinTrunc(untracked, 3)))
	}
	// Only return the synthetic "clean" output when we actually recognized
	// a git-status preamble. Otherwise the input was something else entirely
	// (an error, a porcelain format we don't parse, etc.) and we must pass
	// it through verbatim — silently rewriting an error message to "clean"
	// would hide real failures.
	if sb.Len() == 0 {
		if recognized {
			return "clean\n"
		}
		return output
	}
	return sb.String()
}

func gitLog(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		return output
	}

	// Already compact (--oneline or similar): nothing to do — pass through.
	// Earlier versions truncated to 30 lines + "(N more)", but that drops
	// commits the user explicitly asked for and violates the lossless
	// contract. If the user wants fewer commits, they can pass `-N`.
	if !strings.HasPrefix(lines[0], "commit ") {
		return output
	}

	// Full format: convert to compact but keep full commit body
	type entry struct {
		hash, author, date string
		body               []string
	}
	var entries []entry
	var cur entry

	flush := func() {
		if cur.hash != "" {
			entries = append(entries, cur)
		}
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "commit "):
			flush()
			cur = entry{}
			hash := strings.Fields(strings.TrimPrefix(line, "commit "))[0]
			if len(hash) > 7 {
				hash = hash[:7]
			}
			cur.hash = hash
		case strings.HasPrefix(line, "Author: "):
			author := strings.TrimPrefix(line, "Author: ")
			if idx := strings.Index(author, " <"); idx >= 0 {
				author = author[:idx]
			}
			cur.author = author
		case strings.HasPrefix(line, "Date:   "):
			cur.date = strings.TrimSpace(strings.TrimPrefix(line, "Date:   "))
		case strings.HasPrefix(line, "    "):
			if msg := strings.TrimSpace(line); msg != "" {
				cur.body = append(cur.body, msg)
			}
		}
	}
	flush()

	var sb strings.Builder
	for _, e := range entries {
		subject := ""
		if len(e.body) > 0 {
			subject = e.body[0]
		}
		sb.WriteString(fmt.Sprintf("%s  %-20s  %s  %s\n", e.hash, e.author, e.date, subject))
		// Write additional body lines indented — preserves full context
		for _, line := range e.body[1:] {
			sb.WriteString(fmt.Sprintf("         %s\n", line))
		}
	}
	return sb.String()
}

func gitDiff(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		if strings.HasPrefix(line, "index ") {
			continue
		}
		if strings.HasPrefix(line, "--- a/") {
			result = append(result, "--- "+strings.TrimPrefix(line, "--- a/"))
			continue
		}
		if strings.HasPrefix(line, "+++ b/") {
			result = append(result, "+++ "+strings.TrimPrefix(line, "+++ b/"))
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func gitBranch(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			result = append(result, t)
		}
	}
	return strings.Join(result, "\n") + "\n"
}

func joinTrunc(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" (+%d more)", len(items)-max)
}
