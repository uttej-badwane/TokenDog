package filter

import (
	"fmt"
	"strings"
)

func Ls(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	var dirs, files []string
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		parsed, isDir := parseLsLine(line)
		if parsed == "" {
			continue
		}
		if isDir {
			dirs = append(dirs, parsed)
		} else {
			files = append(files, parsed)
		}
	}

	var sb strings.Builder
	for _, d := range dirs {
		sb.WriteString(d + "\n")
	}
	for _, f := range files {
		sb.WriteString(f + "\n")
	}
	return sb.String()
}

func parseLsLine(line string) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 9 {
		return line, false
	}
	perms := fields[0]
	size := fields[4]
	name := strings.Join(fields[8:], " ")

	// Skip . and .. — every directory has them, they carry no information.
	// Other dotfiles ARE preserved: when a user runs `ls -la`, they explicitly
	// want to see them, and dropping them would violate the lossless contract.
	if name == "." || name == ".." {
		return "", false
	}

	isDir := strings.HasPrefix(perms, "d")
	isLink := strings.HasPrefix(perms, "l")
	isExec := !isDir && !isLink && strings.ContainsRune(perms[3:], 'x')

	marker := ""
	sizeStr := ""
	switch {
	case isDir:
		marker = "/"
	case isLink:
		marker = "@"
	case isExec:
		marker = "*"
	default:
		sizeStr = "  " + humanSizeStr(size)
	}

	return fmt.Sprintf("%-30s%s", name+marker, sizeStr), isDir
}

func humanSizeStr(s string) string {
	var n int
	fmt.Sscanf(s, "%d", &n)
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1fM", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
