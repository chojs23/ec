package tui

import (
	"bytes"
	"testing"

	"github.com/chojs23/ec/internal/markers"
)

func parseSingleConflictDoc(t *testing.T) markers.Document {
	t.Helper()
	data := []byte("start\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	return doc
}

func conflictSegment(t *testing.T, doc markers.Document, index int) markers.ConflictSegment {
	t.Helper()
	ref := doc.Conflicts[index]
	seg, ok := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	if !ok {
		t.Fatalf("expected conflict segment")
	}
	return seg
}

func setConflictResolution(doc *markers.Document, index int, res markers.Resolution) {
	ref := doc.Conflicts[index]
	seg := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	seg.Resolution = res
	doc.Segments[ref.SegmentIndex] = seg
}

func TestApplyMergedResolutionsMatchesSelections(t *testing.T) {
	testCases := []struct {
		name       string
		merged     string
		resolution markers.Resolution
	}{
		{name: "ours", merged: "start\nours\nend\n", resolution: markers.ResolutionOurs},
		{name: "theirs", merged: "start\ntheirs\nend\n", resolution: markers.ResolutionTheirs},
		{name: "both", merged: "start\nours\ntheirs\nend\n", resolution: markers.ResolutionBoth},
		{name: "none", merged: "start\nend\n", resolution: markers.ResolutionNone},
	}

	for _, tc := range testCases {
		doc := parseSingleConflictDoc(t)
		updated, manual, _, _, err := applyMergedResolutions(doc, []byte(tc.merged))
		if err != nil {
			t.Fatalf("%s: applyMergedResolutions error: %v", tc.name, err)
		}
		if len(manual) != 0 {
			t.Fatalf("%s: expected no manual resolutions", tc.name)
		}
		seg := conflictSegment(t, updated, 0)
		if seg.Resolution != tc.resolution {
			t.Fatalf("%s: resolution = %q, want %q", tc.name, seg.Resolution, tc.resolution)
		}
	}
}

func TestApplyMergedResolutionsManualEdit(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	updated, manual, _, _, err := applyMergedResolutions(doc, []byte("start\nmanual\nend\n"))
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionUnset {
		t.Fatalf("resolution = %q, want unset", seg.Resolution)
	}
	if got := string(manual[0]); got != "manual\n" {
		t.Fatalf("manual resolution = %q, want %q", got, "manual\n")
	}
}

func TestApplyMergedResolutionsSkipsConflictMarkers(t *testing.T) {
	merged := "start\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nend\n"
	doc := parseSingleConflictDoc(t)
	updated, manual, labels, known, err := applyMergedResolutions(doc, []byte(merged))
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolutions when markers are present")
	}
	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionUnset {
		t.Fatalf("resolution = %q, want unset", seg.Resolution)
	}
	if labels[0].OursLabel != "HEAD" || labels[0].TheirsLabel != "branch" {
		t.Fatalf("labels[0] = %+v, want HEAD/branch", labels[0])
	}
	if !known[0] {
		t.Fatalf("known[0] = false, want true")
	}
}

func TestApplyMergedResolutionsAllowsNonConflictDeletion(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	merged := []byte("ours\nend\n")

	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolutions")
	}
	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionOurs {
		t.Fatalf("resolution = %q, want %q", seg.Resolution, markers.ResolutionOurs)
	}
	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsPreservesNonConflictEditsWhenResolved(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	merged := []byte("start edited\nextra line\nours\nend changed\n")

	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolutions")
	}
	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionOurs {
		t.Fatalf("resolution = %q, want %q", seg.Resolution, markers.ResolutionOurs)
	}
	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsHandlesEditedSingleLineSeparator(t *testing.T) {
	data := []byte("intro\n<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> branch\nanchor-one\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\ntail\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("intro\nanchor-one@@\nmanual2\ntail\n")
	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}

	if _, ok := manual[0]; ok {
		t.Fatalf("conflict 0 should not be manual")
	}
	seg0 := conflictSegment(t, updated, 0)
	if seg0.Resolution != markers.ResolutionNone {
		t.Fatalf("conflict 0 resolution = %q, want %q", seg0.Resolution, markers.ResolutionNone)
	}

	if got := string(manual[1]); got != "manual2\n" {
		t.Fatalf("manual[1] = %q, want %q", got, "manual2\\n")
	}
	seg1 := conflictSegment(t, updated, 1)
	if seg1.Resolution != markers.ResolutionUnset {
		t.Fatalf("conflict 1 resolution = %q, want unset", seg1.Resolution)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected fully resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsKeepsDuplicatePrefixOutsideConflict(t *testing.T) {
	data := []byte("keep\n<<<<<<< HEAD\nkeep\n=======\ndrop\n>>>>>>> branch\ntail\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("keep\ntail\n")
	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolutions")
	}

	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionNone {
		t.Fatalf("resolution = %q, want %q", seg.Resolution, markers.ResolutionNone)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsKeepsFuzzyPrefixOutsideConflict(t *testing.T) {
	data := []byte("keep root\n<<<<<<< HEAD\nkeep root!\n=======\ndrop\n>>>>>>> branch\ntail\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("keep root!\ntail\n")
	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolutions")
	}

	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionNone {
		t.Fatalf("resolution = %q, want %q", seg.Resolution, markers.ResolutionNone)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsKeepsDuplicateSuffixOutsideConflict(t *testing.T) {
	data := []byte("gone\nkeep\n<<<<<<< HEAD\nkeep\n=======\ndrop\n>>>>>>> branch\ntail\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("keep\ntail\n")
	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolutions")
	}

	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionNone {
		t.Fatalf("resolution = %q, want %q", seg.Resolution, markers.ResolutionNone)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func witrLicenseDiff3Fixture() []byte {
	return []byte("                                 Apache License\n" +
		"                           Version 2.0, January 2004\n" +
		"                        http://www.apache.org/licenses/\n" +
		"\n" +
		"   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION\n" +
		"\n" +
		"   1. Definitions.\n" +
		"\n" +
		"      \"License\" shall mean the terms and conditions for use, reproduction,\n" +
		"      and distribution as defined by Sections 1 through 9 of this document.\n" +
		"\n" +
		"      \"Licensor\" shall mean the copyright owner or entity authorized by\n" +
		"      the copyright owner that is granting the License.B\n" +
		"\n" +
		"      \"Legal Entity\" shall mean the union of the acting entity and all\n" +
		"      other entities that control, are controlled by, or are under common\n" +
		"      control with that entity. For the purposes of this definition,\n" +
		"      \"control\" means (i) the power, direct or indirect, to cause the\n" +
		"      direction or management of such entity, whether by contract or\n" +
		"      otherwise, or (ii) ownership of fifty percent (50%) or more of the\n" +
		"<<<<<<< HEAD\n" +
		"      outstanding shares, or (iii) beneficial ownership of such entity.A\n" +
		"||||||| base\n" +
		"      outstanding shares, or (iii) beneficial ownership of such entity.\n" +
		"=======\n" +
		"      outstanding shares, or (iii) beneficial ownership of such entity.B\n" +
		">>>>>>> branch\n" +
		"\n" +
		"<<<<<<< HEAD\n" +
		"      \"You\" (or \"Your\") shall mean an individual or Legal Entity\n" +
		"      exercising permissions granted by this License.A\n" +
		"||||||| base\n" +
		"      \"You\" (or \"Your\") shall mean an individual or Legal Entity\n" +
		"      exercising permissions granted by this License.\n" +
		"=======\n" +
		"      \"You\" (or \"Your\") shall mean an individual or Legal EntityB\n" +
		"      exercising permissions granted by this License.\n" +
		">>>>>>> branch\n" +
		"\n" +
		"<<<<<<< HEAD\n" +
		"||||||| base\n" +
		"      \"Source\" form shall mean the preferred form for making modifications,\n" +
		"=======\n" +
		"asdsadf\n" +
		"      \"Source\" form shall mean the preferred form for making modifications,\n" +
		">>>>>>> branch\n" +
		"      including but not limited to software source code, documentation\n" +
		"      source, and configuration files.\n")
}

func TestApplyMergedResolutionsWitrLicenseScenario(t *testing.T) {
	diff3 := witrLicenseDiff3Fixture()

	doc, err := markers.Parse(diff3)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("                                 Apache License\n" +
		"                           Version 2.0, January 2004\n" +
		"                        http://www.apache.org/licenses/\n" +
		"\n" +
		"   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION\n" +
		"\n" +
		"   1. Definitions.@@#@#@#\n" +
		"\n" +
		"      \"License\" shall mean the terms and conditions for use, reproduction,\n" +
		"      and distribution as defined by Sections 1 through 9 of this document.\n" +
		"\n" +
		"      \"Licensor\" shall mean the copyright owner or entity authorized by\n" +
		"      the copyright owner that is granting the License.B\n" +
		"\n" +
		"      \"Legal Entity\" shall mean the union of the acting entity and all\n" +
		"      other entities that control, are controlled by, or are under common\n" +
		"      control with that entity. For the purposes of this definition,\n" +
		"      \"control\" means (i) the power, direct or indirect, to cause the\n" +
		"      direction or management of such entity, whether by contract or\n" +
		"      otherwise, or (ii) ownership of fifty percent (50%) or more of the\n" +
		"      outstanding shares, or (iii) beneficial ownership of such entity.A\n" +
		"\n" +
		"      \"You\" (or \"Your\") shall mean an individual or Legal Entity\n" +
		"      exercising permissions granted by this License.A\n" +
		"\n" +
		"      including but not limited to software source code, documentation\n" +
		"      source, and configuration files.\n")

	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("manualResolved len = %d, want 0", len(manual))
	}

	if got := conflictSegment(t, updated, 0).Resolution; got != markers.ResolutionOurs {
		t.Fatalf("conflict 0 resolution = %q, want %q", got, markers.ResolutionOurs)
	}
	if got := conflictSegment(t, updated, 1).Resolution; got != markers.ResolutionOurs {
		t.Fatalf("conflict 1 resolution = %q, want %q", got, markers.ResolutionOurs)
	}
	if got := conflictSegment(t, updated, 2).Resolution; got != markers.ResolutionOurs {
		t.Fatalf("conflict 2 resolution = %q, want %q", got, markers.ResolutionOurs)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected fully resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered merged output mismatch:\nrendered=%q\nmerged=%q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsWitrLicensePartialConflictStaysManual(t *testing.T) {
	doc, err := markers.Parse(witrLicenseDiff3Fixture())
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("                                 Apache License\n" +
		"                           Version 2.0, January 2004\n" +
		"                        http://www.apache.org/licenses/\n" +
		"\n" +
		"   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION\n" +
		"\n" +
		"   1. Definitions.\n" +
		"\n" +
		"      \"License\" shall mean the terms and conditions for use, reproduction,\n" +
		"      and distribution as defined by Sections 1 through 9 of this document.\n" +
		"\n" +
		"      \"Licensor\" shall mean the copyright owner or entity authorized by\n" +
		"      the copyright owner that is granting the License.B@#\n" +
		"\n" +
		"      \"Legal Entity\" shall mean the union of the acting entity and all\n" +
		"      other entities that control, are controlled by, or are under common\n" +
		"      control with that entity. For the purposes of this definition,\n" +
		"      outstanding shares, or (iii) beneficial ownership of such entity.A\n" +
		"\n" +
		"\n" +
		"\n" +
		"\n" +
		"\n" +
		"\n" +
		"      \"You\" (or \"Your\") shall mean an individual or Legal Entity\n" +
		"\n" +
		"\n" +
		"\n" +
		"\n" +
		"\n" +
		"\n" +
		"asdsadf\n" +
		"      \"Source\" form shall mean the preferred form for making modifications,\n" +
		"      including but not limited to software source code, documentation\n" +
		"      source, and configuration files.\n")

	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 1 {
		t.Fatalf("manualResolved len = %d, want 1", len(manual))
	}

	if got := conflictSegment(t, updated, 1).Resolution; got != markers.ResolutionUnset {
		t.Fatalf("conflict 1 resolution = %q, want unset for manual conflict", got)
	}
	if got := conflictSegment(t, updated, 2).Resolution; got != markers.ResolutionTheirs {
		t.Fatalf("conflict 2 resolution = %q, want %q", got, markers.ResolutionTheirs)
	}

	manualBytes, ok := manual[1]
	if !ok {
		t.Fatalf("expected manual resolution for conflict 1")
	}
	if !bytes.Contains(manualBytes, []byte("\"You\" (or \"Your\") shall mean an individual or Legal Entity\n")) {
		t.Fatalf("manual conflict missing kept LICENSE line: %q", string(manualBytes))
	}
	if bytes.Contains(manualBytes, []byte("asdsadf\n")) {
		t.Fatalf("manual conflict consumed next conflict output: %q", string(manualBytes))
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected fully resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered merged output mismatch")
	}
}

func TestApplyMergedResolutionsKeepsTrueEmptyConflictWithBlankSeparator(t *testing.T) {
	data := []byte("start\n<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> branch\n\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("start\n\ntheirs2\nend\n")
	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolutions, got %d: manual=%v", len(manual), manual)
	}
	if got := conflictSegment(t, updated, 0).Resolution; got != markers.ResolutionNone {
		t.Fatalf("conflict 0 resolution = %q, want %q", got, markers.ResolutionNone)
	}
	if got := conflictSegment(t, updated, 1).Resolution; got != markers.ResolutionTheirs {
		t.Fatalf("conflict 1 resolution = %q, want %q", got, markers.ResolutionTheirs)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected fully resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsPrefersSingleNonEmptySideOverBoth(t *testing.T) {
	data := []byte("start\n<<<<<<< HEAD\n||||||| base\nsource\n=======\nasdsadf\nsource\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("start\nasdsadf\nsource\nend\n")
	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolution, got %d", len(manual))
	}

	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionTheirs {
		t.Fatalf("resolution = %q, want %q", seg.Resolution, markers.ResolutionTheirs)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsEmptyOursReopensAsOurs(t *testing.T) {
	data := []byte("start\n<<<<<<< HEAD\n=======\ntheirs\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("start\nend\n")
	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolution, got %d", len(manual))
	}

	if got := conflictSegment(t, updated, 0).Resolution; got != markers.ResolutionOurs {
		t.Fatalf("resolution = %q, want %q", got, markers.ResolutionOurs)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsEmptyTheirsReopensAsTheirs(t *testing.T) {
	data := []byte("start\n<<<<<<< HEAD\nours\n=======\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("start\nend\n")
	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolution, got %d", len(manual))
	}

	if got := conflictSegment(t, updated, 0).Resolution; got != markers.ResolutionTheirs {
		t.Fatalf("resolution = %q, want %q", got, markers.ResolutionTheirs)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsEmptyBothStaysNone(t *testing.T) {
	data := []byte("start\n<<<<<<< HEAD\n=======\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	merged := []byte("start\nend\n")
	updated, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolution, got %d", len(manual))
	}

	if got := conflictSegment(t, updated, 0).Resolution; got != markers.ResolutionNone {
		t.Fatalf("resolution = %q, want %q", got, markers.ResolutionNone)
	}

	rendered, unresolved, err := renderMergedOutput(updated, manual, labels, known)
	if err != nil {
		t.Fatalf("renderMergedOutput error: %v", err)
	}
	if unresolved {
		t.Fatalf("expected resolved output")
	}
	if string(rendered) != string(merged) {
		t.Fatalf("rendered = %q, want %q", string(rendered), string(merged))
	}
}

func TestApplyMergedResolutionsAlignsLabelsToOriginalConflictIndex(t *testing.T) {
	doc := parseMultiConflictDoc(t)
	merged := []byte("start\nmanual1\nmid\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nend\n")
	_, manual, labels, known, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if got := string(manual[0]); got != "manual1\n" {
		t.Fatalf("manual[0] = %q, want %q", got, "manual1\\n")
	}
	if labels[0].OursLabel != "" || labels[0].TheirsLabel != "" {
		t.Fatalf("labels[0] = %+v, want empty labels for manually resolved conflict", labels[0])
	}
	if labels[1].OursLabel != "HEAD" || labels[1].TheirsLabel != "branch" {
		t.Fatalf("labels[1] = %+v, want HEAD/branch", labels[1])
	}
	if known[0] {
		t.Fatalf("known[0] = true, want false")
	}
	if !known[1] {
		t.Fatalf("known[1] = false, want true")
	}
}

func TestAllResolvedWithManualOverride(t *testing.T) {
	data := []byte("start\n<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> branch\nmid\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	setConflictResolution(&doc, 0, markers.ResolutionOurs)
	if allResolved(doc, map[int][]byte{}) {
		t.Fatalf("expected unresolved without manual override")
	}

	manual := map[int][]byte{1: []byte("manual\n")}
	if !allResolved(doc, manual) {
		t.Fatalf("expected resolved with manual override")
	}
}

func TestContainsConflictMarkers(t *testing.T) {
	if !containsConflictMarkers([][]byte{[]byte("<<<<<<< HEAD\n")}) {
		t.Fatalf("expected conflict markers to be detected")
	}
	if containsConflictMarkers([][]byte{[]byte("ok\n"), []byte("line\n")}) {
		t.Fatalf("did not expect conflict markers")
	}
}
