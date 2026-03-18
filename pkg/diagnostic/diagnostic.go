package diagnostic

import (
	"fmt"
	"sort"
	"strings"
)

// Severity classifies a diagnostic message.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// Diagnostic represents a single compiler diagnostic (error or warning).
type Diagnostic struct {
	Severity Severity
	Code     string // e.g. "E001", "W001"
	Message  string
	Span     Span
	Hint     string   // optional "did you mean?" or help text
	Labels   []Label  // additional labeled spans (e.g. "type annotation here")
}

// Label is a secondary annotation pointing to a source span.
type Label struct {
	Span    Span
	Message string
}

// Collector accumulates diagnostics and supports error/warning limits.
type Collector struct {
	registry    *SourceRegistry
	diagnostics []Diagnostic
	maxErrors   int
	maxWarnings int
	errorCount  int
	warnCount   int
}

// NewCollector creates a Collector with the given registry and limits.
func NewCollector(registry *SourceRegistry, maxErrors, maxWarnings int) *Collector {
	return &Collector{
		registry:    registry,
		maxErrors:   maxErrors,
		maxWarnings: maxWarnings,
	}
}

// Error adds an error diagnostic.
func (c *Collector) Error(code, message string, span Span) {
	c.Add(Diagnostic{
		Severity: SeverityError,
		Code:     code,
		Message:  message,
		Span:     span,
	})
}

// ErrorWithHint adds an error diagnostic with a help hint.
func (c *Collector) ErrorWithHint(code, message string, span Span, hint string) {
	c.Add(Diagnostic{
		Severity: SeverityError,
		Code:     code,
		Message:  message,
		Span:     span,
		Hint:     hint,
	})
}

// Warning adds a warning diagnostic.
func (c *Collector) Warning(code, message string, span Span) {
	c.Add(Diagnostic{
		Severity: SeverityWarning,
		Code:     code,
		Message:  message,
		Span:     span,
	})
}

// WarningWithHint adds a warning diagnostic with a help hint.
func (c *Collector) WarningWithHint(code, message string, span Span, hint string) {
	c.Add(Diagnostic{
		Severity: SeverityWarning,
		Code:     code,
		Message:  message,
		Span:     span,
		Hint:     hint,
	})
}

// Add appends a diagnostic, respecting limits.
func (c *Collector) Add(d Diagnostic) {
	switch d.Severity {
	case SeverityError:
		if c.maxErrors > 0 && c.errorCount >= c.maxErrors {
			return
		}
		c.errorCount++
	case SeverityWarning:
		if c.maxWarnings > 0 && c.warnCount >= c.maxWarnings {
			return
		}
		c.warnCount++
	}
	c.diagnostics = append(c.diagnostics, d)
}

// HasErrors returns true if any error-level diagnostics have been recorded.
func (c *Collector) HasErrors() bool { return c.errorCount > 0 }

// ErrorCount returns the number of error-level diagnostics.
func (c *Collector) ErrorCount() int { return c.errorCount }

// WarningCount returns the number of warning-level diagnostics.
func (c *Collector) WarningCount() int { return c.warnCount }

// Diagnostics returns a copy of all collected diagnostics.
func (c *Collector) Diagnostics() []Diagnostic {
	out := make([]Diagnostic, len(c.diagnostics))
	copy(out, c.diagnostics)
	return out
}

// Registry returns the underlying SourceRegistry.
func (c *Collector) Registry() *SourceRegistry { return c.registry }

// Format renders a single diagnostic as a multi-line string matching
// the Ryx error format:
//
//	error[E001]: type mismatch
//	  --> src/main.ryx:12:15
//	   |
//	12 |     let x: Int = "hello";
//	   |                  ^^^^^^^ expected `Int`, found `String`
//	   |
//	   = help: did you mean `for`?
func (c *Collector) Format(d Diagnostic) string {
	pos := c.registry.Position(d.Span)
	sf := c.registry.File(d.Span.FileID)

	var b strings.Builder

	// Header: severity[code]: message
	b.WriteString(fmt.Sprintf("%s[%s]: %s\n", d.Severity, d.Code, d.Message))

	// Location
	b.WriteString(fmt.Sprintf("  --> %s:%d:%d\n", pos.File, pos.Line, pos.Col))

	if sf == nil {
		return b.String()
	}

	startLine, startCol := sf.OffsetToLineCol(d.Span.Start)
	endLine, endCol := sf.OffsetToLineCol(maxInt(d.Span.End-1, d.Span.Start))
	// endCol should point past the last character
	if d.Span.End > d.Span.Start {
		endCol++
	}

	// Compute gutter width for line numbers
	gutterWidth := digitCount(endLine)
	if gutterWidth < 1 {
		gutterWidth = 1
	}
	gutter := strings.Repeat(" ", gutterWidth)

	// Blank separator
	b.WriteString(fmt.Sprintf("%s |\n", gutter))

	// Source lines with underline
	for line := startLine; line <= endLine; line++ {
		content := sf.Line(line)
		b.WriteString(fmt.Sprintf("%*d | %s\n", gutterWidth, line, content))

		// Build underline
		ulStart := 1
		ulEnd := len(content) + 1
		if line == startLine {
			ulStart = startCol
		}
		if line == endLine {
			ulEnd = endCol
		}
		if ulEnd <= ulStart {
			ulEnd = ulStart + 1
		}

		underline := strings.Repeat(" ", ulStart-1) + strings.Repeat("^", ulEnd-ulStart)
		// Append message on the last underline line
		if line == endLine {
			underline += " " + d.Message
		}
		b.WriteString(fmt.Sprintf("%s | %s\n", gutter, underline))
	}

	// Render secondary labels
	for _, label := range d.Labels {
		labelPos := c.registry.Position(label.Span)
		labelSF := c.registry.File(label.Span.FileID)
		if labelSF == nil {
			continue
		}
		lLine, lCol := labelSF.OffsetToLineCol(label.Span.Start)
		_, lEndCol := labelSF.OffsetToLineCol(maxInt(label.Span.End-1, label.Span.Start))
		if label.Span.End > label.Span.Start {
			lEndCol++
		}
		lGutterWidth := digitCount(lLine)
		if lGutterWidth < gutterWidth {
			lGutterWidth = gutterWidth
		}
		lGutter := strings.Repeat(" ", lGutterWidth)

		if label.Span.FileID != d.Span.FileID || lLine != startLine {
			b.WriteString(fmt.Sprintf("  --> %s:%d:%d\n", labelPos.File, labelPos.Line, labelPos.Col))
			b.WriteString(fmt.Sprintf("%s |\n", lGutter))
			lContent := labelSF.Line(lLine)
			b.WriteString(fmt.Sprintf("%*d | %s\n", lGutterWidth, lLine, lContent))
		}
		ulLen := lEndCol - lCol
		if ulLen < 1 {
			ulLen = 1
		}
		underline := strings.Repeat(" ", lCol-1) + strings.Repeat("-", ulLen)
		underline += " " + label.Message
		b.WriteString(fmt.Sprintf("%s | %s\n", lGutter, underline))
	}

	// Blank separator after source lines
	b.WriteString(fmt.Sprintf("%s |\n", gutter))

	// Help / hint
	if d.Hint != "" {
		b.WriteString(fmt.Sprintf("%s = help: %s\n", gutter, d.Hint))
	}

	return b.String()
}

// FormatAll renders all collected diagnostics into a single string.
func (c *Collector) FormatAll() string {
	var b strings.Builder
	for i, d := range c.diagnostics {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(c.Format(d))
	}
	// Summary line
	if c.errorCount > 0 || c.warnCount > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		parts := make([]string, 0, 2)
		if c.errorCount > 0 {
			parts = append(parts, fmt.Sprintf("%d error(s)", c.errorCount))
		}
		if c.warnCount > 0 {
			parts = append(parts, fmt.Sprintf("%d warning(s)", c.warnCount))
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteByte('\n')
	}
	return b.String()
}

// DidYouMean finds the closest match to name from candidates using
// Levenshtein distance. Returns "" if no candidate is close enough
// (threshold: distance ≤ max(2, len(name)/3)).
func DidYouMean(name string, candidates []string) string {
	if len(candidates) == 0 || name == "" {
		return ""
	}
	threshold := len(name) / 3
	if threshold < 2 {
		threshold = 2
	}
	best := ""
	bestDist := threshold + 1
	for _, c := range candidates {
		d := levenshtein(name, c)
		if d < bestDist {
			bestDist = d
			best = c
		}
	}
	if bestDist > threshold {
		return ""
	}
	return best
}

// SuggestHint returns a formatted "did you mean `X`?" string, or "" if no match.
func SuggestHint(name string, candidates []string) string {
	match := DidYouMean(name, candidates)
	if match == "" {
		return ""
	}
	return fmt.Sprintf("did you mean `%s`?", match)
}

// SortByPosition sorts diagnostics by file ID, then line, then column.
func SortByPosition(registry *SourceRegistry, diags []Diagnostic) {
	sort.Slice(diags, func(i, j int) bool {
		pi := registry.Position(diags[i].Span)
		pj := registry.Position(diags[j].Span)
		if pi.File != pj.File {
			return pi.File < pj.File
		}
		if pi.Line != pj.Line {
			return pi.Line < pj.Line
		}
		return pi.Col < pj.Col
	})
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	// Use single-row optimization.
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = minOf(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev = curr
	}
	return prev[lb]
}

func minOf(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func digitCount(n int) int {
	if n <= 0 {
		return 1
	}
	count := 0
	for n > 0 {
		count++
		n /= 10
	}
	return count
}
