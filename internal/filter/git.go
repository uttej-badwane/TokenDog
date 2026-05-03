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

	var section string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "On branch "):
			branch = strings.TrimPrefix(line, "On branch ")
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
	if sb.Len() == 0 {
		return "clean\n"
	}
	return sb.String()
}

func gitLog(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		return output
	}

	// Already compact (--oneline or similar): just limit lines
	if !strings.HasPrefix(lines[0], "commit ") {
		const limit = 30
		if len(lines) > limit {
			extra := len(lines) - limit
			lines = lines[:limit]
			lines = append(lines, fmt.Sprintf("... (%d more)", extra))
		}
		return strings.Join(lines, "\n") + "\n"
	}

	// Full format: convert to compact one-line-per-commit
	type entry struct{ hash, author, date, msg string }
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
			if cur.msg == "" {
				cur.msg = strings.TrimSpace(line)
			}
		}
	}
	flush()

	const limit = 30
	if len(entries) > limit {
		entries = entries[:limit]
	}

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("%s  %-20s  %s  %s\n", e.hash, e.author, e.date, e.msg))
	}
	if len(entries) == limit {
		sb.WriteString("...\n")
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
