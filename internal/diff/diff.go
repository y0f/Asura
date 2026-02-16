package diff

import (
	"fmt"
	"strings"
)

// Compute returns a unified diff between old and new content using LCS.
func Compute(old, new string) string {
	oldLines := splitLines(old)
	newLines := splitLines(new)

	lcs := lcsTable(oldLines, newLines)
	return buildDiff(oldLines, newLines, lcs)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// lcsTable builds the LCS length table.
func lcsTable(a, b []string) [][]int {
	m := len(a)
	n := len(b)
	table := make([][]int, m+1)
	for i := range table {
		table[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				table[i][j] = table[i-1][j-1] + 1
			} else if table[i-1][j] >= table[i][j-1] {
				table[i][j] = table[i-1][j]
			} else {
				table[i][j] = table[i][j-1]
			}
		}
	}
	return table
}

// buildDiff constructs a unified-style diff from the LCS table.
func buildDiff(old, new []string, table [][]int) string {
	var sb strings.Builder
	var changes []diffLine

	i := len(old)
	j := len(new)

	for i > 0 || j > 0 {
		if i > 0 && j > 0 && old[i-1] == new[j-1] {
			changes = append(changes, diffLine{op: ' ', text: old[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || table[i][j-1] >= table[i-1][j]) {
			changes = append(changes, diffLine{op: '+', text: new[j-1]})
			j--
		} else if i > 0 {
			changes = append(changes, diffLine{op: '-', text: old[i-1]})
			i--
		}
	}

	// Reverse
	for left, right := 0, len(changes)-1; left < right; left, right = left+1, right-1 {
		changes[left], changes[right] = changes[right], changes[left]
	}

	// Format output
	for _, c := range changes {
		if c.op == ' ' {
			fmt.Fprintf(&sb, " %s\n", c.text)
		} else {
			fmt.Fprintf(&sb, "%c%s\n", c.op, c.text)
		}
	}

	return sb.String()
}

type diffLine struct {
	op   byte
	text string
}
