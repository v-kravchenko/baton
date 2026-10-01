package dashboard

import "strings"

// DiffLine is one line of a line diff: op is ' ', '-' or '+'.
type DiffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// maxDiffCells bounds the LCS table; larger inputs fall back to replace-all.
const maxDiffCells = 4_000_000

// Diff returns a line diff from a to b using an LCS table.
func Diff(a, b string) []DiffLine {
	x := splitLines(a)
	y := splitLines(b)
	// Trim common prefix and suffix to keep the table small.
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	var out []DiffLine
	for _, l := range x[:pre] {
		out = append(out, DiffLine{" ", l})
	}
	mx, my := x[pre:len(x)-suf], y[pre:len(y)-suf]
	if (len(mx)+1)*(len(my)+1) > maxDiffCells {
		for _, l := range mx {
			out = append(out, DiffLine{"-", l})
		}
		for _, l := range my {
			out = append(out, DiffLine{"+", l})
		}
	} else {
		out = append(out, lcsDiff(mx, my)...)
	}
	for _, l := range x[len(x)-suf:] {
		out = append(out, DiffLine{" ", l})
	}
	return out
}

func lcsDiff(x, y []string) []DiffLine {
	n, m := len(x), len(y)
	t := make([][]int32, n+1)
	for i := range t {
		t[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				t[i][j] = t[i+1][j+1] + 1
			} else if t[i+1][j] >= t[i][j+1] {
				t[i][j] = t[i+1][j]
			} else {
				t[i][j] = t[i][j+1]
			}
		}
	}
	var out []DiffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case x[i] == y[j]:
			out = append(out, DiffLine{" ", x[i]})
			i++
			j++
		case t[i+1][j] >= t[i][j+1]:
			out = append(out, DiffLine{"-", x[i]})
			i++
		default:
			out = append(out, DiffLine{"+", y[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, DiffLine{"-", x[i]})
	}
	for ; j < m; j++ {
		out = append(out, DiffLine{"+", y[j]})
	}
	return out
}

func splitLines(s string) []string {
	s = strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
