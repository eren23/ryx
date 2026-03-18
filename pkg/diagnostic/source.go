package diagnostic

import (
	"sort"
	"strings"
)

// Span represents a contiguous range of bytes in a source file.
type Span struct {
	FileID int // index into SourceRegistry.files
	Start  int // inclusive byte offset
	End    int // exclusive byte offset
}

// Len returns the byte length of the span.
func (s Span) Len() int { return s.End - s.Start }

// Merge returns a span covering both s and other. Panics if they refer to different files.
func (s Span) Merge(other Span) Span {
	if s.FileID != other.FileID {
		panic("diagnostic: cannot merge spans from different files")
	}
	start := s.Start
	if other.Start < start {
		start = other.Start
	}
	end := s.End
	if other.End > end {
		end = other.End
	}
	return Span{FileID: s.FileID, Start: start, End: end}
}

// Position is a human-readable source position (1-based line and column).
type Position struct {
	File   string
	Line   int // 1-based
	Col    int // 1-based, in bytes
	Offset int // 0-based byte offset
}

// SourceFile holds metadata and content for a single source file.
type SourceFile struct {
	Name    string
	Content string
	// lineOffsets[i] is the byte offset of the start of line i+1 (0-based index → 1-based line).
	lineOffsets []int
}

// newSourceFile creates a SourceFile and precomputes line offset table.
func newSourceFile(name, content string) *SourceFile {
	sf := &SourceFile{Name: name, Content: content}
	sf.lineOffsets = computeLineOffsets(content)
	return sf
}

// computeLineOffsets returns a slice where entry i is the byte offset of line i+1.
// Line 1 always starts at offset 0.
func computeLineOffsets(content string) []int {
	offsets := []int{0}
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// LineCount returns the number of lines in this file.
func (sf *SourceFile) LineCount() int {
	return len(sf.lineOffsets)
}

// LineStart returns the byte offset of the start of the given 1-based line.
func (sf *SourceFile) LineStart(line int) int {
	if line < 1 || line > len(sf.lineOffsets) {
		return len(sf.Content)
	}
	return sf.lineOffsets[line-1]
}

// LineEnd returns the byte offset of the end of the given 1-based line (before newline).
func (sf *SourceFile) LineEnd(line int) int {
	if line < 1 || line > len(sf.lineOffsets) {
		return len(sf.Content)
	}
	if line < len(sf.lineOffsets) {
		end := sf.lineOffsets[line]
		// Exclude the trailing newline.
		if end > 0 && sf.Content[end-1] == '\n' {
			end--
		}
		return end
	}
	return len(sf.Content)
}

// Line returns the content of the given 1-based line (without trailing newline).
func (sf *SourceFile) Line(line int) string {
	start := sf.LineStart(line)
	end := sf.LineEnd(line)
	if start > len(sf.Content) {
		return ""
	}
	if end > len(sf.Content) {
		end = len(sf.Content)
	}
	return sf.Content[start:end]
}

// OffsetToLineCol converts a byte offset to a 1-based (line, col) pair.
func (sf *SourceFile) OffsetToLineCol(offset int) (line, col int) {
	if offset < 0 {
		return 1, 1
	}
	if offset > len(sf.Content) {
		offset = len(sf.Content)
	}
	// Binary search: find the largest line whose start offset ≤ offset.
	idx := sort.Search(len(sf.lineOffsets), func(i int) bool {
		return sf.lineOffsets[i] > offset
	})
	// idx is the first line whose start > offset, so line = idx (1-based).
	line = idx
	if line < 1 {
		line = 1
	}
	col = offset - sf.lineOffsets[line-1] + 1
	return line, col
}

// SourceRegistry manages a set of source files and provides span-to-position lookups.
type SourceRegistry struct {
	files []*SourceFile
}

// NewSourceRegistry creates an empty registry.
func NewSourceRegistry() *SourceRegistry {
	return &SourceRegistry{}
}

// AddFile registers a source file and returns its file ID (index).
func (r *SourceRegistry) AddFile(name, content string) int {
	id := len(r.files)
	r.files = append(r.files, newSourceFile(name, content))
	return id
}

// File returns the SourceFile for the given ID, or nil if invalid.
func (r *SourceRegistry) File(id int) *SourceFile {
	if id < 0 || id >= len(r.files) {
		return nil
	}
	return r.files[id]
}

// FileCount returns the number of registered files.
func (r *SourceRegistry) FileCount() int {
	return len(r.files)
}

// Position resolves a Span's start offset to a human-readable Position.
func (r *SourceRegistry) Position(s Span) Position {
	sf := r.File(s.FileID)
	if sf == nil {
		return Position{File: "<unknown>", Line: 1, Col: 1, Offset: s.Start}
	}
	line, col := sf.OffsetToLineCol(s.Start)
	return Position{File: sf.Name, Line: line, Col: col, Offset: s.Start}
}

// SpanText returns the source text covered by the span.
func (r *SourceRegistry) SpanText(s Span) string {
	sf := r.File(s.FileID)
	if sf == nil {
		return ""
	}
	start := s.Start
	end := s.End
	if start < 0 {
		start = 0
	}
	if end > len(sf.Content) {
		end = len(sf.Content)
	}
	if start >= end {
		return ""
	}
	return sf.Content[start:end]
}

// Snippet returns the source line(s) for a span, with line numbers.
func (r *SourceRegistry) Snippet(s Span) string {
	sf := r.File(s.FileID)
	if sf == nil {
		return ""
	}
	startLine, _ := sf.OffsetToLineCol(s.Start)
	endLine, _ := sf.OffsetToLineCol(maxInt(s.End-1, s.Start))

	var b strings.Builder
	for line := startLine; line <= endLine; line++ {
		content := sf.Line(line)
		b.WriteString(content)
		if line < endLine {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
