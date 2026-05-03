package filter

import (
	"fmt"
	"strings"
)

func Docker(subcommand string, output string) string {
	switch subcommand {
	case "ps":
		return dockerPS(output)
	case "images":
		return dockerImages(output)
	default:
		return output
	}
}

func dockerPS(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		return output
	}

	var sb strings.Builder
	for i, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if i == 0 {
			// Header: CONTAINER ID, IMAGE, COMMAND, CREATED, STATUS, PORTS, NAMES
			// Keep: ID, IMAGE, STATUS, NAMES
			sb.WriteString(fmt.Sprintf("%-14s  %-30s  %-20s  %s\n", "ID", "IMAGE", "STATUS", "NAMES"))
			continue
		}
		if len(fields) < 7 {
			continue
		}
		id := fields[0]
		if len(id) > 12 {
			id = id[:12]
		}
		image := fields[1]
		if len(image) > 30 {
			image = image[:27] + "..."
		}
		// STATUS is at index 4, NAMES at the last field
		status := fields[4]
		if len(fields) > 5 {
			status = strings.Join(fields[4:len(fields)-1], " ")
		}
		name := fields[len(fields)-1]
		sb.WriteString(fmt.Sprintf("%-14s  %-30s  %-20s  %s\n", id, image, status, name))
	}
	return sb.String()
}

func dockerImages(output string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		return output
	}

	var sb strings.Builder
	for i, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if i == 0 {
			// Header: REPOSITORY, TAG, IMAGE ID, CREATED, SIZE
			sb.WriteString(fmt.Sprintf("%-35s  %-15s  %-14s  %s\n", "REPOSITORY", "TAG", "ID", "SIZE"))
			continue
		}
		if len(fields) < 5 {
			continue
		}
		repo := fields[0]
		if len(repo) > 35 {
			repo = repo[:32] + "..."
		}
		tag := fields[1]
		id := fields[2]
		if len(id) > 12 {
			id = id[:12]
		}
		size := fields[len(fields)-1]
		sb.WriteString(fmt.Sprintf("%-35s  %-15s  %-14s  %s\n", repo, tag, id, size))
	}
	return sb.String()
}
