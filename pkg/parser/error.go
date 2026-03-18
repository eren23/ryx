package parser

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/lexer"
)

// maxErrors is the default cap before the parser stops collecting.
const maxErrors = 20

// ParseError represents a single parse error with location information.
type ParseError struct {
	Span    diagnostic.Span
	Message string
	Hint    string
}

func (e *ParseError) Error() string { return e.Message }

// ParseResult holds the parsed program and any errors encountered.
type ParseResult struct {
	Program *Program
	Errors  []*ParseError
}

// HasErrors returns true if parsing produced errors.
func (r *ParseResult) HasErrors() bool { return len(r.Errors) > 0 }

// ---------------------------------------------------------------------------
// Error helpers used by the parser
// ---------------------------------------------------------------------------

func (p *Parser) error(span diagnostic.Span, msg string) {
	if len(p.errors) >= maxErrors {
		return
	}
	p.errors = append(p.errors, &ParseError{Span: span, Message: msg})
}

func (p *Parser) errorf(span diagnostic.Span, format string, args ...any) {
	p.error(span, fmt.Sprintf(format, args...))
}

func (p *Parser) errorWithHint(span diagnostic.Span, msg, hint string) {
	if len(p.errors) >= maxErrors {
		return
	}
	p.errors = append(p.errors, &ParseError{Span: span, Message: msg, Hint: hint})
}

func (p *Parser) errorExpected(expected string) {
	tok := p.peek()
	p.errorf(tok.Span, "expected %s, found %s", expected, describeToken(tok))
}

func (p *Parser) errorUnexpected() {
	tok := p.peek()
	p.errorf(tok.Span, "unexpected %s", describeToken(tok))
}

// ---------------------------------------------------------------------------
// Error recovery: synchronize on statement/item boundaries
// ---------------------------------------------------------------------------

// syncTokens are tokens that the parser synchronizes to during error recovery.
var syncTokens = map[lexer.TokenType]bool{
	lexer.LBRACE:    true,
	lexer.RBRACE:    true,
	lexer.SEMICOLON: true,
	lexer.FN:        true,
	lexer.TYPE:      true,
	lexer.STRUCT:    true,
	lexer.TRAIT:     true,
	lexer.IMPL:      true,
	lexer.LET:       true,
	lexer.PUB:       true,
	lexer.IMPORT:    true,
	lexer.MODULE:    true,
	lexer.EOF:       true,
}

// synchronize advances past tokens until a synchronization point is reached.
func (p *Parser) synchronize() {
	for !p.atEnd() {
		// If we just consumed a semicolon or closing brace, we're at a boundary.
		if p.prev().Type == lexer.SEMICOLON || p.prev().Type == lexer.RBRACE {
			return
		}
		if syncTokens[p.peek().Type] {
			return
		}
		p.advance()
	}
}

// ---------------------------------------------------------------------------
// Token description for error messages
// ---------------------------------------------------------------------------

func describeToken(tok lexer.Token) string {
	switch tok.Type {
	case lexer.EOF:
		return "end of file"
	case lexer.IDENT:
		return fmt.Sprintf("identifier `%s`", tok.Value)
	case lexer.INT_LIT:
		return fmt.Sprintf("integer `%s`", tok.Value)
	case lexer.FLOAT_LIT:
		return fmt.Sprintf("float `%s`", tok.Value)
	case lexer.STRING_LIT:
		return fmt.Sprintf("string %q", tok.Value)
	case lexer.CHAR_LIT:
		return fmt.Sprintf("char '%s'", tok.Value)
	case lexer.BOOL_LIT:
		return fmt.Sprintf("`%s`", tok.Value)
	default:
		if tok.Type.IsKeyword() {
			return fmt.Sprintf("keyword `%s`", tok.Value)
		}
		return fmt.Sprintf("`%s`", tok.Value)
	}
}

// ---------------------------------------------------------------------------
// Unclosed delimiter tracking
// ---------------------------------------------------------------------------

type delimInfo struct {
	openTok lexer.Token
	kind    string // "parenthesis", "brace", "bracket"
}

func (p *Parser) pushDelim(tok lexer.Token, kind string) {
	p.delimStack = append(p.delimStack, delimInfo{openTok: tok, kind: kind})
}

func (p *Parser) popDelim() {
	if len(p.delimStack) > 0 {
		p.delimStack = p.delimStack[:len(p.delimStack)-1]
	}
}

func (p *Parser) reportUnclosedDelims() {
	for i := len(p.delimStack) - 1; i >= 0; i-- {
		d := p.delimStack[i]
		p.errorf(d.openTok.Span, "unclosed %s", d.kind)
	}
}
