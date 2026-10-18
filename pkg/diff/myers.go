package diff

import (
	"fmt"
	"sort"
	"strings"
)

type Operation int

const (
	OpEqual Operation = iota
	OpInsert
	OpDelete
)

type Edit struct {
	Op   Operation
	Text string
}

func MyersDiff(a, b []string) []Edit {
	n := len(a)
	m := len(b)
	max := n + m
	if max == 0 {
		return nil
	}

	vSize := 2*max + 1
	vOffset := max

	v := make([]int, vSize)
	trace := make([][]int, 0, max+1)

	for d := 0; d <= max; d++ {
		vCopy := make([]int, vSize)
		copy(vCopy, v)
		trace = append(trace, vCopy)

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[k-1+vOffset] < v[k+1+vOffset]) {
				x = v[k+1+vOffset]
			} else {
				x = v[k-1+vOffset] + 1
			}

			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}

			v[k+vOffset] = x
			if x >= n && y >= m {
				return backtrack(trace, a, b, n, m, max, vOffset)
			}
		}
	}

	return nil
}

func backtrack(trace [][]int, a, b []string, n, m, max, vOffset int) []Edit {
	var edits []Edit
	x := n
	y := m

	for d := len(trace) - 1; d > 0; d-- {
		vPrev := trace[d]
		k := x - y

		var prevK int
		if k == -d || (k != d && vPrev[k-1+vOffset] < vPrev[k+1+vOffset]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevX := vPrev[prevK+vOffset]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x--
			y--
			edits = append(edits, Edit{Op: OpEqual, Text: a[x]})
		}

		if d > 0 {
			if x == prevX {
				y--
				edits = append(edits, Edit{Op: OpInsert, Text: b[y]})
			} else if y == prevY {
				x--
				edits = append(edits, Edit{Op: OpDelete, Text: a[x]})
			}
		}
	}

	for x > 0 && y > 0 {
		x--
		y--
		edits = append(edits, Edit{Op: OpEqual, Text: a[x]})
	}

	for i := 0; i < len(edits)/2; i++ {
		j := len(edits) - 1 - i
		edits[i], edits[j] = edits[j], edits[i]
	}

	return edits
}

type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []Edit
}

func BuildHunks(edits []Edit, contextLines int) []Hunk {
	if len(edits) == 0 {
		return nil
	}

	hasChanges := false
	for _, e := range edits {
		if e.Op != OpEqual {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return nil
	}

	var hunks []Hunk

	type taggedEdit struct {
		Edit
		oldLine int
		newLine int
	}

	var tagged []taggedEdit
	oldCur := 1
	newCur := 1
	for _, e := range edits {
		t := taggedEdit{Edit: e, oldLine: oldCur, newLine: newCur}
		switch e.Op {
		case OpEqual:
			oldCur++
			newCur++
		case OpDelete:
			oldCur++
		case OpInsert:
			newCur++
		}
		tagged = append(tagged, t)
	}

	n := len(tagged)
	i := 0
	for i < n {
		for i < n && tagged[i].Op == OpEqual {
			i++
		}
		if i >= n {
			break
		}

		start := i - contextLines
		if start < 0 {
			start = 0
		}

		end := i
		for end < n {
			if tagged[end].Op != OpEqual {
				end++
			} else {
				equalRun := 0
				peek := end
				for peek < n && tagged[peek].Op == OpEqual {
					equalRun++
					peek++
				}
				if peek < n && equalRun <= 2*contextLines {
					end = peek
				} else {
					end += contextLines
					if end > n {
						end = n
					}
					break
				}
			}
		}

		hunkLines := make([]Edit, 0, end-start)
		oldStart := 0
		oldCount := 0
		newStart := 0
		newCount := 0

		for idx := start; idx < end; idx++ {
			te := tagged[idx]
			hunkLines = append(hunkLines, te.Edit)
			if oldStart == 0 && (te.Op == OpEqual || te.Op == OpDelete) {
				oldStart = te.oldLine
			}
			if newStart == 0 && (te.Op == OpEqual || te.Op == OpInsert) {
				newStart = te.newLine
			}
			if te.Op == OpEqual || te.Op == OpDelete {
				oldCount++
			}
			if te.Op == OpEqual || te.Op == OpInsert {
				newCount++
			}
		}

		if oldCount == 0 {
			oldStart = 0
		} else if oldStart == 0 {
			oldStart = 1
		}
		if newCount == 0 {
			newStart = 0
		} else if newStart == 0 {
			newStart = 1
		}

		hunks = append(hunks, Hunk{
			OldStart: oldStart,
			OldCount: oldCount,
			NewStart: newStart,
			NewCount: newCount,
			Lines:    hunkLines,
		})

		i = end
	}

	return hunks
}

func FormatUnifiedDiff(pathA, pathB string, edits []Edit, contextLines int) string {
	hunks := BuildHunks(edits, contextLines)
	if len(hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	if pathA == "/dev/null" || pathA == "" {
		sb.WriteString("--- /dev/null\n")
	} else {
		sb.WriteString(fmt.Sprintf("--- a/%s\n", pathA))
	}
	if pathB == "/dev/null" || pathB == "" {
		sb.WriteString("+++ /dev/null\n")
	} else {
		sb.WriteString(fmt.Sprintf("+++ b/%s\n", pathB))
	}

	for _, hunk := range hunks {
		var oldPart, newPart string
		if hunk.OldCount == 1 {
			oldPart = fmt.Sprintf("-%d", hunk.OldStart)
		} else {
			oldPart = fmt.Sprintf("-%d,%d", hunk.OldStart, hunk.OldCount)
		}
		if hunk.NewCount == 1 {
			newPart = fmt.Sprintf("+%d", hunk.NewStart)
		} else {
			newPart = fmt.Sprintf("+%d,%d", hunk.NewStart, hunk.NewCount)
		}
		sb.WriteString(fmt.Sprintf("@@ %s %s @@\n", oldPart, newPart))
		for _, line := range hunk.Lines {
			switch line.Op {
			case OpEqual:
				sb.WriteString(" " + line.Text + "\n")
			case OpInsert:
				sb.WriteString("+" + line.Text + "\n")
			case OpDelete:
				sb.WriteString("-" + line.Text + "\n")
			}
		}
	}

	return sb.String()
}

func DiffStrings(pathA, pathB, a, b string) string {
	var linesA, linesB []string
	if len(a) > 0 {
		linesA = strings.Split(strings.TrimSuffix(a, "\n"), "\n")
	}
	if len(b) > 0 {
		linesB = strings.Split(strings.TrimSuffix(b, "\n"), "\n")
	}
	edits := MyersDiff(linesA, linesB)
	return FormatUnifiedDiff(pathA, pathB, edits, 3)
}

type changeChunk struct {
	baseStart int
	baseEnd   int
	lines     []string
}

type taggedChunk struct {
	side      int
	baseStart int
	baseEnd   int
	lines     []string
}

type chunkCluster struct {
	start int
	end   int
	items []taggedChunk
}

func splitLines(s string) []string {
	if len(s) == 0 {
		return nil
	}
	trimmed := strings.TrimSuffix(s, "\n")
	if len(trimmed) == 0 {
		return []string{""}
	}
	return strings.Split(trimmed, "\n")
}

func editsToChunks(edits []Edit) []changeChunk {
	var chunks []changeChunk
	baseIdx := 0
	inChunk := false
	var cur changeChunk

	for _, e := range edits {
		switch e.Op {
		case OpEqual:
			if inChunk {
				cur.baseEnd = baseIdx
				chunks = append(chunks, cur)
				inChunk = false
				cur = changeChunk{}
			}
			baseIdx++
		case OpDelete:
			if !inChunk {
				inChunk = true
				cur.baseStart = baseIdx
				cur.lines = nil
			}
			baseIdx++
		case OpInsert:
			if !inChunk {
				inChunk = true
				cur.baseStart = baseIdx
				cur.lines = nil
			}
			cur.lines = append(cur.lines, e.Text)
		}
	}

	if inChunk {
		cur.baseEnd = baseIdx
		chunks = append(chunks, cur)
	}

	return chunks
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func Merge3(base, ours, theirs, targetName string) (string, bool) {
	if ours == theirs {
		return ours, false
	}
	if ours == base {
		return theirs, false
	}
	if theirs == base {
		return ours, false
	}

	baseLines := splitLines(base)
	ourLines := splitLines(ours)
	theirLines := splitLines(theirs)

	editsA := MyersDiff(baseLines, ourLines)
	chunksA := editsToChunks(editsA)

	editsB := MyersDiff(baseLines, theirLines)
	chunksB := editsToChunks(editsB)

	var allChunks []taggedChunk
	for _, c := range chunksA {
		allChunks = append(allChunks, taggedChunk{side: 1, baseStart: c.baseStart, baseEnd: c.baseEnd, lines: c.lines})
	}
	for _, c := range chunksB {
		allChunks = append(allChunks, taggedChunk{side: 2, baseStart: c.baseStart, baseEnd: c.baseEnd, lines: c.lines})
	}

	sort.Slice(allChunks, func(i, j int) bool {
		if allChunks[i].baseStart != allChunks[j].baseStart {
			return allChunks[i].baseStart < allChunks[j].baseStart
		}
		if allChunks[i].baseEnd != allChunks[j].baseEnd {
			return allChunks[i].baseEnd < allChunks[j].baseEnd
		}
		return allChunks[i].side < allChunks[j].side
	})

	var clusters []chunkCluster
	for _, tc := range allChunks {
		if len(clusters) == 0 {
			clusters = append(clusters, chunkCluster{start: tc.baseStart, end: tc.baseEnd, items: []taggedChunk{tc}})
		} else {
			last := &clusters[len(clusters)-1]
			if tc.baseStart <= last.end {
				if tc.baseEnd > last.end {
					last.end = tc.baseEnd
				}
				last.items = append(last.items, tc)
			} else {
				clusters = append(clusters, chunkCluster{start: tc.baseStart, end: tc.baseEnd, items: []taggedChunk{tc}})
			}
		}
	}

	var out []string
	curBase := 0
	hasConflict := false

	for _, cl := range clusters {
		for curBase < cl.start {
			out = append(out, baseLines[curBase])
			curBase++
		}

		var aChunks, bChunks []taggedChunk
		for _, it := range cl.items {
			if it.side == 1 {
				aChunks = append(aChunks, it)
			} else {
				bChunks = append(bChunks, it)
			}
		}

		buildLines := func(chunks []taggedChunk) []string {
			var res []string
			c := cl.start
			for _, ch := range chunks {
				for c < ch.baseStart {
					res = append(res, baseLines[c])
					c++
				}
				res = append(res, ch.lines...)
				c = ch.baseEnd
			}
			for c < cl.end {
				res = append(res, baseLines[c])
				c++
			}
			return res
		}

		linesA := buildLines(aChunks)
		linesB := buildLines(bChunks)

		if len(bChunks) == 0 {
			out = append(out, linesA...)
		} else if len(aChunks) == 0 {
			out = append(out, linesB...)
		} else if slicesEqual(linesA, linesB) {
			out = append(out, linesA...)
		} else {
			hasConflict = true
			out = append(out, "<<<<<<< HEAD")
			out = append(out, linesA...)
			out = append(out, "=======")
			out = append(out, linesB...)
			out = append(out, ">>>>>>> "+targetName)
		}

		curBase = cl.end
	}

	for curBase < len(baseLines) {
		out = append(out, baseLines[curBase])
		curBase++
	}

	if len(out) == 0 {
		return "", hasConflict
	}
	res := strings.Join(out, "\n")
	if hasConflict || strings.HasSuffix(ours, "\n") || strings.HasSuffix(theirs, "\n") {
		res += "\n"
	}
	return res, hasConflict
}

