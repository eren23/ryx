package diagnostic

import (
	"strings"
	"testing"
)

// --- Source / Span Tests ---

func TestOffsetToLineCol_SingleLine(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("test.ryx", "hello world")

	sf := reg.File(0)
	tests := []struct {
		offset   int
		wantLine int
		wantCol  int
	}{
		{0, 1, 1},
		{5, 1, 6},
		{10, 1, 11},
		{11, 1, 12}, // one past end
	}
	for _, tc := range tests {
		line, col := sf.OffsetToLineCol(tc.offset)
		if line != tc.wantLine || col != tc.wantCol {
			t.Errorf("OffsetToLineCol(%d) = (%d, %d), want (%d, %d)",
				tc.offset, line, col, tc.wantLine, tc.wantCol)
		}
	}
}

func TestOffsetToLineCol_MultiLine(t *testing.T) {
	src := "line one\nline two\nline three"
	// offsets:  0-8=line1, 9-16=line2, 17-26=line3
	reg := NewSourceRegistry()
	reg.AddFile("multi.ryx", src)
	sf := reg.File(0)

	tests := []struct {
		offset   int
		wantLine int
		wantCol  int
	}{
		{0, 1, 1},   // 'l' in "line one"
		{8, 1, 9},   // '\n' at end of "line one"
		{9, 2, 1},   // 'l' in "line two"
		{13, 2, 5},  // ' ' in "line two"
		{17, 2, 9},  // '\n' at end of "line two"
		{18, 3, 1},  // 'l' in "line three"
		{27, 3, 10}, // 'e' at end of "line three"
	}
	for _, tc := range tests {
		line, col := sf.OffsetToLineCol(tc.offset)
		if line != tc.wantLine || col != tc.wantCol {
			t.Errorf("OffsetToLineCol(%d) = (%d, %d), want (%d, %d)",
				tc.offset, line, col, tc.wantLine, tc.wantCol)
		}
	}
}

func TestOffsetToLineCol_EdgeCases(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("edge.ryx", "abc\n")
	sf := reg.File(0)

	// Negative offset clamps to (1,1)
	line, col := sf.OffsetToLineCol(-5)
	if line != 1 || col != 1 {
		t.Errorf("negative offset: got (%d, %d), want (1, 1)", line, col)
	}

	// Offset past end clamps
	line, col = sf.OffsetToLineCol(100)
	if line != 2 || col != 1 {
		t.Errorf("past-end offset: got (%d, %d), want (2, 1)", line, col)
	}
}

func TestSourceFile_Line(t *testing.T) {
	src := "first\nsecond\nthird"
	reg := NewSourceRegistry()
	reg.AddFile("lines.ryx", src)
	sf := reg.File(0)

	tests := []struct {
		lineNum int
		want    string
	}{
		{1, "first"},
		{2, "second"},
		{3, "third"},
	}
	for _, tc := range tests {
		got := sf.Line(tc.lineNum)
		if got != tc.want {
			t.Errorf("Line(%d) = %q, want %q", tc.lineNum, got, tc.want)
		}
	}
}

func TestSourceRegistry_MultipleFiles(t *testing.T) {
	reg := NewSourceRegistry()
	id0 := reg.AddFile("a.ryx", "aaa\nbbb")
	id1 := reg.AddFile("b.ryx", "xxx\nyyy\nzzz")

	if id0 != 0 || id1 != 1 {
		t.Fatalf("unexpected IDs: %d, %d", id0, id1)
	}
	if reg.FileCount() != 2 {
		t.Fatalf("FileCount() = %d, want 2", reg.FileCount())
	}

	pos := reg.Position(Span{FileID: 1, Start: 4, End: 7})
	if pos.File != "b.ryx" || pos.Line != 2 || pos.Col != 1 {
		t.Errorf("Position = %+v, want b.ryx:2:1", pos)
	}
}

func TestSourceRegistry_SpanText(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("test.ryx", "let x = 42;")
	text := reg.SpanText(Span{FileID: 0, Start: 4, End: 5})
	if text != "x" {
		t.Errorf("SpanText = %q, want %q", text, "x")
	}
}

// --- Diagnostic Formatting Tests ---

func TestFormat_BasicError(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("src/main.ryx", "fn main() {\n    let x: Int = \"hello\";\n}\n")

	d := Diagnostic{
		Severity: SeverityError,
		Code:     "E001",
		Message:  "type mismatch",
		Span:     Span{FileID: 0, Start: 29, End: 36}, // "hello"
	}

	c := NewCollector(reg, 20, 100)
	output := c.Format(d)

	// Verify key parts of the output
	if !strings.Contains(output, `error[E001]: type mismatch`) {
		t.Errorf("missing header in:\n%s", output)
	}
	if !strings.Contains(output, `--> src/main.ryx:2:`) {
		t.Errorf("missing location in:\n%s", output)
	}
	if !strings.Contains(output, `"hello"`) {
		t.Errorf("missing source line in:\n%s", output)
	}
	if !strings.Contains(output, "^") {
		t.Errorf("missing underline carets in:\n%s", output)
	}
}

func TestFormat_Warning(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("src/main.ryx", "fn main() {\n    let unused = 42;\n}\n")

	d := Diagnostic{
		Severity: SeverityWarning,
		Code:     "W001",
		Message:  "unused variable",
		Span:     Span{FileID: 0, Start: 20, End: 26}, // "unused"
		Hint:     "prefix with `_` to suppress this warning",
	}

	c := NewCollector(reg, 20, 100)
	output := c.Format(d)

	if !strings.Contains(output, `warning[W001]: unused variable`) {
		t.Errorf("missing warning header in:\n%s", output)
	}
	if !strings.Contains(output, `= help: prefix with`) {
		t.Errorf("missing hint in:\n%s", output)
	}
}

func TestFormat_WithHint(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("src/main.ryx", "fn main() {\n    foo + 1\n}\n")

	d := Diagnostic{
		Severity: SeverityError,
		Code:     "E002",
		Message:  "unknown variable `foo`",
		Span:     Span{FileID: 0, Start: 16, End: 19}, // "foo"
		Hint:     "did you mean `for`?",
	}

	c := NewCollector(reg, 20, 100)
	output := c.Format(d)

	if !strings.Contains(output, `= help: did you mean`) {
		t.Errorf("missing 'did you mean' hint in:\n%s", output)
	}
}

// --- Multi-Error Accumulation ---

func TestCollector_MultiError(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("test.ryx", "aaa\nbbb\nccc\n")

	c := NewCollector(reg, 20, 100)
	c.Error("E001", "first error", Span{FileID: 0, Start: 0, End: 3})
	c.Error("E002", "second error", Span{FileID: 0, Start: 4, End: 7})
	c.Warning("W001", "a warning", Span{FileID: 0, Start: 8, End: 11})

	if c.ErrorCount() != 2 {
		t.Errorf("ErrorCount() = %d, want 2", c.ErrorCount())
	}
	if c.WarningCount() != 1 {
		t.Errorf("WarningCount() = %d, want 1", c.WarningCount())
	}
	if !c.HasErrors() {
		t.Error("HasErrors() should be true")
	}
	if len(c.Diagnostics()) != 3 {
		t.Errorf("len(Diagnostics()) = %d, want 3", len(c.Diagnostics()))
	}
}

func TestCollector_ErrorLimit(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("test.ryx", "x\ny\nz\n")

	c := NewCollector(reg, 2, 1) // max 2 errors, 1 warning
	c.Error("E001", "err1", Span{FileID: 0, Start: 0, End: 1})
	c.Error("E002", "err2", Span{FileID: 0, Start: 2, End: 3})
	c.Error("E003", "err3", Span{FileID: 0, Start: 4, End: 5}) // should be dropped

	if c.ErrorCount() != 2 {
		t.Errorf("ErrorCount() = %d, want 2 (limit exceeded)", c.ErrorCount())
	}
	if len(c.Diagnostics()) != 2 {
		t.Errorf("len(Diagnostics()) = %d, want 2", len(c.Diagnostics()))
	}

	c.Warning("W001", "warn1", Span{FileID: 0, Start: 0, End: 1})
	c.Warning("W002", "warn2", Span{FileID: 0, Start: 2, End: 3}) // should be dropped

	if c.WarningCount() != 1 {
		t.Errorf("WarningCount() = %d, want 1 (limit exceeded)", c.WarningCount())
	}
}

func TestFormatAll_Summary(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("test.ryx", "aaa\nbbb\n")

	c := NewCollector(reg, 20, 100)
	c.Error("E001", "some error", Span{FileID: 0, Start: 0, End: 3})
	c.Warning("W001", "some warning", Span{FileID: 0, Start: 4, End: 7})

	output := c.FormatAll()

	if !strings.Contains(output, "1 error(s)") {
		t.Errorf("missing error summary in:\n%s", output)
	}
	if !strings.Contains(output, "1 warning(s)") {
		t.Errorf("missing warning summary in:\n%s", output)
	}
}

// --- Did You Mean Tests ---

func TestDidYouMean(t *testing.T) {
	candidates := []string{"for", "fn", "if", "else", "match", "let", "mut", "return"}

	tests := []struct {
		input string
		want  string
	}{
		{"foo", "for"},
		{"fo", "for"},
		{"les", "let"},
		{"metch", "match"},
		{"retrun", "return"},
		{"xyzabc", ""},
	}
	for _, tc := range tests {
		got := DidYouMean(tc.input, candidates)
		if got != tc.want {
			t.Errorf("DidYouMean(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSuggestHint(t *testing.T) {
	candidates := []string{"for", "fn", "let"}
	hint := SuggestHint("foo", candidates)
	if hint != `did you mean `+"`for`?" {
		t.Errorf("SuggestHint = %q", hint)
	}

	noHint := SuggestHint("xyzxyzxyz", candidates)
	if noHint != "" {
		t.Errorf("SuggestHint should be empty for distant input, got %q", noHint)
	}
}

// --- Sort Tests ---

func TestSortByPosition(t *testing.T) {
	reg := NewSourceRegistry()
	reg.AddFile("a.ryx", "aaa\nbbb\nccc\n")
	reg.AddFile("b.ryx", "xxx\n")

	diags := []Diagnostic{
		{Severity: SeverityError, Span: Span{FileID: 0, Start: 8, End: 11}},  // a.ryx:3
		{Severity: SeverityError, Span: Span{FileID: 1, Start: 0, End: 3}},   // b.ryx:1
		{Severity: SeverityError, Span: Span{FileID: 0, Start: 0, End: 3}},   // a.ryx:1
		{Severity: SeverityWarning, Span: Span{FileID: 0, Start: 4, End: 7}}, // a.ryx:2
	}

	SortByPosition(reg, diags)

	// Expected order: a.ryx:1, a.ryx:2, a.ryx:3, b.ryx:1
	expectedFiles := []int{0, 0, 0, 1}
	expectedStarts := []int{0, 4, 8, 0}
	for i, d := range diags {
		if d.Span.FileID != expectedFiles[i] || d.Span.Start != expectedStarts[i] {
			t.Errorf("diags[%d] = file:%d start:%d, want file:%d start:%d",
				i, d.Span.FileID, d.Span.Start, expectedFiles[i], expectedStarts[i])
		}
	}
}

// --- Span Tests ---

func TestSpan_Merge(t *testing.T) {
	s1 := Span{FileID: 0, Start: 5, End: 10}
	s2 := Span{FileID: 0, Start: 8, End: 15}
	merged := s1.Merge(s2)
	if merged.Start != 5 || merged.End != 15 {
		t.Errorf("Merge = %+v, want Start:5 End:15", merged)
	}
}

func TestSpan_Len(t *testing.T) {
	s := Span{Start: 3, End: 10}
	if s.Len() != 7 {
		t.Errorf("Len() = %d, want 7", s.Len())
	}
}

func TestSpan_MergePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Merge with different FileIDs should panic")
		}
	}()
	s1 := Span{FileID: 0, Start: 0, End: 5}
	s2 := Span{FileID: 1, Start: 0, End: 5}
	s1.Merge(s2)
}
