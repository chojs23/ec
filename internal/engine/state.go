package engine

import (
	"bytes"
	"fmt"

	"github.com/chojs23/ec/internal/markers"
)

type ConflictLabels struct {
	OursLabel   string
	BaseLabel   string
	TheirsLabel string
}

type conflictState struct {
	canonical      markers.ConflictSegment
	output         []byte
	resolution     markers.Resolution
	manual         bool
	labels         ConflictLabels
	labelKnown     bool
	resolvedOurs   bool
	resolvedTheirs bool
	onesideApplied bool
}

type segmentState struct {
	text     []byte
	conflict *conflictState
}

type State struct {
	canonical markers.Document
	segments  []segmentState
	doc       markers.Document
}

func NewState(doc markers.Document) (*State, error) {
	return newStateFromDocument(doc), nil
}

func newStateFromDocument(doc markers.Document) *State {
	canonical := markers.CloneDocument(doc)
	segments := make([]segmentState, 0, len(canonical.Segments))
	for _, seg := range canonical.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			segments = append(segments, segmentState{text: append([]byte(nil), s.Bytes...)})
		case markers.ConflictSegment:
			cs := newConflictState(s)
			segments = append(segments, segmentState{conflict: &cs})
		}
	}
	state := &State{canonical: canonical, segments: segments}
	state.syncDocument()
	return state
}

func newConflictState(seg markers.ConflictSegment) conflictState {
	state := conflictState{
		canonical: seg,
		labels: ConflictLabels{
			OursLabel:   seg.OursLabel,
			BaseLabel:   seg.BaseLabel,
			TheirsLabel: seg.TheirsLabel,
		},
	}
	if seg.Resolution == markers.ResolutionUnset {
		state.output = renderConflictMarkers(seg, state.labels)
		state.applyClassification(markers.ResolutionUnset, false, false, ConflictLabels{}, false)
		return state
	}
	state.setResolved(seg.Resolution)
	return state
}

func (s *State) ApplyResolution(conflictIndex int, resolution markers.Resolution) error {
	if conflictIndex < 0 || conflictIndex >= len(s.canonical.Conflicts) {
		return fmt.Errorf("conflict index %d out of bounds [0, %d)", conflictIndex, len(s.canonical.Conflicts))
	}
	if !isSupportedResolution(resolution) {
		return fmt.Errorf("invalid resolution: %q", resolution)
	}
	segIndex := s.canonical.Conflicts[conflictIndex].SegmentIndex
	conflict := s.segments[segIndex].conflict
	if conflict == nil {
		return fmt.Errorf("internal: conflict index %d points to non-ConflictSegment", conflictIndex)
	}
	conflict.setResolved(resolution)
	s.syncDocument()
	return nil
}

func (s *State) ApplyAll(resolution markers.Resolution) error {
	if !isSupportedResolution(resolution) {
		return fmt.Errorf("invalid resolution: %q", resolution)
	}
	for _, ref := range s.canonical.Conflicts {
		conflict := s.segments[ref.SegmentIndex].conflict
		if conflict == nil {
			return fmt.Errorf("internal: conflict points to non-ConflictSegment")
		}
		conflict.setResolved(resolution)
	}
	s.syncDocument()
	return nil
}

func (s *State) ReplaceDocument(doc markers.Document) {
	next := newStateFromDocument(doc)
	s.canonical = next.canonical
	s.segments = next.segments
	s.doc = next.doc
}

func (s *State) Preview() ([]byte, error) {
	if s.HasUnresolvedConflicts() {
		return nil, fmt.Errorf("%w: conflict without resolution", markers.ErrUnresolved)
	}
	return s.RenderMerged(), nil
}

func (s *State) Document() markers.Document {
	return markers.CloneDocument(s.doc)
}

func (s *State) syncDocument() {
	doc := markers.CloneDocument(s.canonical)
	for i, segment := range s.segments {
		switch seg := doc.Segments[i].(type) {
		case markers.TextSegment:
			seg.Bytes = append([]byte(nil), segment.text...)
			doc.Segments[i] = seg
		case markers.ConflictSegment:
			conflict := segment.conflict
			seg.OursLabel = conflict.canonical.OursLabel
			seg.BaseLabel = conflict.canonical.BaseLabel
			seg.TheirsLabel = conflict.canonical.TheirsLabel
			if conflict.labelKnown {
				seg.OursLabel = conflict.labels.OursLabel
				seg.BaseLabel = conflict.labels.BaseLabel
				seg.TheirsLabel = conflict.labels.TheirsLabel
			}
			seg.Resolution = conflict.resolution
			doc.Segments[i] = seg
		}
	}
	s.doc = doc
}

func (s *State) Clone() *State {
	clone := &State{canonical: markers.CloneDocument(s.canonical), doc: markers.CloneDocument(s.doc)}
	clone.segments = make([]segmentState, len(s.segments))
	for i, segment := range s.segments {
		if segment.conflict == nil {
			clone.segments[i] = segmentState{text: append([]byte(nil), segment.text...)}
			continue
		}
		conflict := *segment.conflict
		conflict.output = append([]byte(nil), segment.conflict.output...)
		clone.segments[i] = segmentState{conflict: &conflict}
	}
	return clone
}

func (s *State) RenderMerged() []byte {
	var out bytes.Buffer
	for _, segment := range s.segments {
		if segment.conflict == nil {
			out.Write(segment.text)
			continue
		}
		out.Write(segment.conflict.output)
	}
	return out.Bytes()
}

func (s *State) HasUnresolvedConflicts() bool {
	for _, ref := range s.canonical.Conflicts {
		conflict := s.segments[ref.SegmentIndex].conflict
		if conflict != nil && conflict.resolution == markers.ResolutionUnset && !conflict.manual {
			return true
		}
	}
	return false
}

func (s *State) ManualResolved() map[int][]byte {
	manual := map[int][]byte{}
	for idx, ref := range s.canonical.Conflicts {
		conflict := s.segments[ref.SegmentIndex].conflict
		if conflict != nil && conflict.manual {
			manual[idx] = append([]byte(nil), conflict.output...)
		}
	}
	return manual
}

func (s *State) MergedLabels() ([]ConflictLabels, []bool) {
	labels := make([]ConflictLabels, len(s.canonical.Conflicts))
	known := make([]bool, len(s.canonical.Conflicts))
	for idx, ref := range s.canonical.Conflicts {
		conflict := s.segments[ref.SegmentIndex].conflict
		if conflict == nil {
			continue
		}
		labels[idx] = conflict.labels
		known[idx] = conflict.labelKnown
	}
	return labels, known
}

func (s *State) ImportMerged(merged []byte) error {
	parsed, err := markers.Parse(merged)
	if err == nil && len(parsed.Conflicts) == len(s.canonical.Conflicts) && s.canImportParsedDocument(parsed) {
		s.importParsedDocument(parsed)
		return nil
	}

	oldLines := markers.SplitLinesKeepEOL(s.RenderMerged())
	newLines := markers.SplitLinesKeepEOL(merged)
	segmentLines, lineToSegment, boundaryOwner := s.segmentLineOwnership()
	_ = segmentLines
	ops := diffLines(oldLines, newLines)
	assigned := make([][][]byte, len(s.segments))
	oldCursor := 0
	pendingDeletedOwner := -1

	for _, op := range ops {
		switch op.kind {
		case diffInsert:
			target := boundaryOwner[oldCursor]
			if pendingDeletedOwner != -1 {
				target = pendingDeletedOwner
			}
			assigned[target] = append(assigned[target], op.newLines...)
			pendingDeletedOwner = -1
		case diffEqual:
			for _, line := range op.newLines {
				if oldCursor >= len(lineToSegment) {
					break
				}
				target := lineToSegment[oldCursor]
				assigned[target] = append(assigned[target], line)
				oldCursor++
			}
			pendingDeletedOwner = -1
		case diffDelete:
			if len(op.oldLines) > 0 && oldCursor < len(lineToSegment) {
				pendingDeletedOwner = lineToSegment[oldCursor]
			}
			oldCursor += len(op.oldLines)
		}
	}

	for i, segment := range s.segments {
		updated := joinLines(assigned[i])
		if segment.conflict == nil {
			s.segments[i].text = updated
			continue
		}
		conflict := s.segments[i].conflict
		conflict.output = updated
		conflict.classifyUpdatedOutput()
	}
	s.syncDocument()
	return nil
}

func (s *State) canImportParsedDocument(doc markers.Document) bool {
	if len(doc.Segments) != len(s.canonical.Segments) {
		return false
	}
	for i, seg := range s.canonical.Segments {
		switch seg.(type) {
		case markers.TextSegment:
			if _, ok := doc.Segments[i].(markers.TextSegment); !ok {
				return false
			}
		case markers.ConflictSegment:
			if _, ok := doc.Segments[i].(markers.ConflictSegment); !ok {
				return false
			}
		}
	}
	return true
}

func (s *State) importParsedDocument(doc markers.Document) {
	for i, parsed := range doc.Segments {
		switch seg := parsed.(type) {
		case markers.TextSegment:
			s.segments[i].text = append([]byte(nil), seg.Bytes...)
		case markers.ConflictSegment:
			conflict := s.segments[i].conflict
			if conflict == nil {
				continue
			}
			conflict.output = renderConflictMarkers(seg, ConflictLabels{
				OursLabel:   seg.OursLabel,
				BaseLabel:   seg.BaseLabel,
				TheirsLabel: seg.TheirsLabel,
			})
			conflict.classifyUpdatedOutput()
		}
	}
	s.syncDocument()
}

func (s *State) segmentLineOwnership() ([]int, []int, []int) {
	segmentLines := make([]int, len(s.segments))
	totalLines := 0
	for i, segment := range s.segments {
		lines := markers.SplitLinesKeepEOL(s.segmentBytes(segment))
		segmentLines[i] = len(lines)
		totalLines += len(lines)
	}

	lineToSegment := make([]int, totalLines)
	boundaryOwner := make([]int, totalLines+1)
	for i := range boundaryOwner {
		boundaryOwner[i] = -1
	}

	cursor := 0
	for i, count := range segmentLines {
		boundaryOwner[cursor] = i
		for j := 0; j < count; j++ {
			lineToSegment[cursor+j] = i
		}
		cursor += count
	}
	if len(s.segments) > 0 && boundaryOwner[totalLines] == -1 {
		boundaryOwner[totalLines] = len(s.segments) - 1
	}
	for i := range boundaryOwner {
		if boundaryOwner[i] != -1 {
			continue
		}
		if i > 0 {
			boundaryOwner[i] = lineToSegment[i-1]
			continue
		}
		boundaryOwner[i] = 0
	}
	return segmentLines, lineToSegment, boundaryOwner
}

func (s *State) segmentBytes(segment segmentState) []byte {
	if segment.conflict == nil {
		return segment.text
	}
	return segment.conflict.output
}

func (c *conflictState) setResolved(resolution markers.Resolution) {
	c.output = renderResolution(c.canonical, resolution)
	c.applyClassification(resolution, resolution == markers.ResolutionUnset, false, ConflictLabels{}, false)
}

func (c *conflictState) classifyUpdatedOutput() {
	resolution, unresolved, manual, labels, known := classifyConflictOutput(c.canonical, c.output)
	c.applyClassification(resolution, unresolved, manual, labels, known)
}

func (c *conflictState) applyClassification(resolution markers.Resolution, unresolved bool, manual bool, labels ConflictLabels, known bool) {
	c.resolution = resolution
	c.manual = manual
	c.labelKnown = known
	if known {
		c.labels = labels
	} else {
		c.labels = ConflictLabels{
			OursLabel:   c.canonical.OursLabel,
			BaseLabel:   c.canonical.BaseLabel,
			TheirsLabel: c.canonical.TheirsLabel,
		}
	}
	c.resolvedOurs, c.resolvedTheirs, c.onesideApplied = classifyResolvedSides(c.canonical, resolution, unresolved, manual)
}

func classifyResolvedSides(seg markers.ConflictSegment, resolution markers.Resolution, unresolved bool, manual bool) (bool, bool, bool) {
	if unresolved {
		return false, false, false
	}
	if manual {
		return true, true, false
	}
	switch resolution {
	case markers.ResolutionOurs:
		resolvedTheirs := len(seg.Theirs) == 0
		return true, resolvedTheirs, !resolvedTheirs
	case markers.ResolutionTheirs:
		resolvedOurs := len(seg.Ours) == 0
		return resolvedOurs, true, !resolvedOurs
	case markers.ResolutionBoth, markers.ResolutionNone:
		return true, true, false
	default:
		return false, false, false
	}
}

func renderResolution(seg markers.ConflictSegment, resolution markers.Resolution) []byte {
	switch resolution {
	case markers.ResolutionOurs:
		return append([]byte(nil), seg.Ours...)
	case markers.ResolutionTheirs:
		return append([]byte(nil), seg.Theirs...)
	case markers.ResolutionBoth:
		return append(append([]byte(nil), seg.Ours...), seg.Theirs...)
	case markers.ResolutionNone:
		return nil
	default:
		return renderConflictMarkers(seg, ConflictLabels{OursLabel: seg.OursLabel, BaseLabel: seg.BaseLabel, TheirsLabel: seg.TheirsLabel})
	}
}

func renderConflictMarkers(seg markers.ConflictSegment, labels ConflictLabels) []byte {
	copySeg := seg
	copySeg.Resolution = markers.ResolutionUnset
	var out bytes.Buffer
	markers.AppendConflictSegment(&out, copySeg, labels.OursLabel, labels.BaseLabel, labels.TheirsLabel)
	return out.Bytes()
}

func classifyConflictOutput(seg markers.ConflictSegment, output []byte) (markers.Resolution, bool, bool, ConflictLabels, bool) {
	both := append(append([][]byte{}, markers.SplitLinesKeepEOL(seg.Ours)...), markers.SplitLinesKeepEOL(seg.Theirs)...)
	bothBytes := joinLines(both)
	switch {
	case bytes.Equal(output, seg.Ours):
		return markers.ResolutionOurs, false, false, ConflictLabels{}, false
	case bytes.Equal(output, seg.Theirs):
		return markers.ResolutionTheirs, false, false, ConflictLabels{}, false
	case bytes.Equal(output, bothBytes):
		return markers.ResolutionBoth, false, false, ConflictLabels{}, false
	case len(output) == 0:
		return markers.ResolutionNone, false, false, ConflictLabels{}, false
	}

	parsed, err := markers.Parse(output)
	if err == nil && len(parsed.Conflicts) == 1 && len(parsed.Segments) == 1 {
		if unresolved, ok := parsed.Segments[parsed.Conflicts[0].SegmentIndex].(markers.ConflictSegment); ok {
			return markers.ResolutionUnset, true, false, ConflictLabels{
				OursLabel:   unresolved.OursLabel,
				BaseLabel:   unresolved.BaseLabel,
				TheirsLabel: unresolved.TheirsLabel,
			}, true
		}
	}

	return markers.ResolutionUnset, false, true, ConflictLabels{}, false
}

func isSupportedResolution(resolution markers.Resolution) bool {
	switch resolution {
	case markers.ResolutionOurs, markers.ResolutionTheirs, markers.ResolutionBoth, markers.ResolutionNone:
		return true
	default:
		return false
	}
}

func joinLines(lines [][]byte) []byte {
	if len(lines) == 0 {
		return nil
	}
	joined := make([]byte, 0)
	for _, line := range lines {
		joined = append(joined, line...)
	}
	return joined
}

type diffKind int

const (
	diffEqual diffKind = iota
	diffDelete
	diffInsert
)

type diffOp struct {
	kind     diffKind
	oldLines [][]byte
	newLines [][]byte
}

func diffLines(oldLines [][]byte, newLines [][]byte) []diffOp {
	n := len(oldLines)
	m := len(newLines)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if bytes.Equal(oldLines[i], newLines[j]) {
				dp[i][j] = dp[i+1][j+1] + 1
				continue
			}
			if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []diffOp
	appendOp := func(kind diffKind, oldLine []byte, newLine []byte) {
		if len(ops) > 0 && ops[len(ops)-1].kind == kind {
			switch kind {
			case diffEqual:
				ops[len(ops)-1].oldLines = append(ops[len(ops)-1].oldLines, oldLine)
				ops[len(ops)-1].newLines = append(ops[len(ops)-1].newLines, newLine)
			case diffDelete:
				ops[len(ops)-1].oldLines = append(ops[len(ops)-1].oldLines, oldLine)
			case diffInsert:
				ops[len(ops)-1].newLines = append(ops[len(ops)-1].newLines, newLine)
			}
			return
		}
		op := diffOp{kind: kind}
		switch kind {
		case diffEqual:
			op.oldLines = [][]byte{oldLine}
			op.newLines = [][]byte{newLine}
		case diffDelete:
			op.oldLines = [][]byte{oldLine}
		case diffInsert:
			op.newLines = [][]byte{newLine}
		}
		ops = append(ops, op)
	}

	i, j := 0, 0
	for i < n && j < m {
		if bytes.Equal(oldLines[i], newLines[j]) {
			appendOp(diffEqual, oldLines[i], newLines[j])
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			appendOp(diffDelete, oldLines[i], nil)
			i++
			continue
		}
		appendOp(diffInsert, nil, newLines[j])
		j++
	}
	for i < n {
		appendOp(diffDelete, oldLines[i], nil)
		i++
	}
	for j < m {
		appendOp(diffInsert, nil, newLines[j])
		j++
	}
	return ops
}
