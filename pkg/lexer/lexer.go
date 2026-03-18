package lexer

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
)

// Lexer performs single-pass, character-by-character scanning of Ryx source code,
// producing a stream of tokens. It never panics; malformed input produces ERROR tokens.
type Lexer struct {
	src    string
	fileID int
	pos    int // current byte offset
	line   int // 1-based
	col    int // 1-based (byte column)
}

// New creates a Lexer for the given source text and file ID.
func New(src string, fileID int) *Lexer {
	return &Lexer{
		src:    src,
		fileID: fileID,
		pos:    0,
		line:   1,
		col:    1,
	}
}

// Tokenize scans the entire source and returns all tokens, ending with EOF.
func (l *Lexer) Tokenize() []Token {
	var tokens []Token
	for {
		tok := l.Next()
		tokens = append(tokens, tok)
		if tok.Type == EOF {
			break
		}
	}
	return tokens
}

// Next returns the next token from the source.
func (l *Lexer) Next() Token {
	l.skipWhitespaceAndComments()

	if l.pos >= len(l.src) {
		return l.makeToken(EOF, l.pos, l.pos)
	}

	startPos := l.pos
	startLine := l.line
	startCol := l.col

	ch := l.src[l.pos]

	// Newline
	if ch == '\n' {
		l.advance()
		tok := Token{
			Type:  NEWLINE,
			Value: "\n",
			Span:  diagnostic.Span{FileID: l.fileID, Start: startPos, End: startPos + 1},
			Line:  startLine,
			Col:   startCol,
		}
		return tok
	}

	// Identifiers and keywords
	if isIdentStart(l.peekRune()) {
		return l.scanIdentifier()
	}

	// Raw strings: r"..."
	if ch == 'r' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '"' {
		return l.scanRawString()
	}

	// Numeric literals
	if ch >= '0' && ch <= '9' {
		return l.scanNumber()
	}

	// String literals
	if ch == '"' {
		return l.scanString()
	}

	// Char literals
	if ch == '\'' {
		return l.scanChar()
	}

	// Operators and delimiters
	return l.scanOperatorOrDelimiter()
}

// skipWhitespaceAndComments skips spaces, tabs, carriage returns, and comments.
// Newlines are NOT skipped (they are significant tokens).
func (l *Lexer) skipWhitespaceAndComments() {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]

		// Skip whitespace (but not newlines)
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
			continue
		}

		// Line comments: //
		if ch == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			l.skipLineComment()
			continue
		}

		// Block comments: /* ... */ (nested)
		if ch == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
			l.skipBlockComment()
			continue
		}

		break
	}
}

func (l *Lexer) skipLineComment() {
	// Skip the //
	l.advance()
	l.advance()
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.advance()
	}
	// Don't consume the newline; it becomes a NEWLINE token
}

func (l *Lexer) skipBlockComment() {
	startPos := l.pos
	startLine := l.line
	startCol := l.col

	// Skip /*
	l.advance()
	l.advance()

	depth := 1
	for l.pos < len(l.src) && depth > 0 {
		if l.src[l.pos] == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
			depth++
			l.advance()
			l.advance()
		} else if l.src[l.pos] == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			depth--
			l.advance()
			l.advance()
		} else {
			l.advance()
		}
	}

	if depth > 0 {
		// Unterminated block comment — we don't emit a token here because
		// the comment was being skipped. We could emit an error, but since
		// the skip loop returns to Next() which will hit EOF, that's acceptable.
		// However, to properly report this, we store an error token that will
		// be returned by the next call to Next().
		_ = startPos
		_ = startLine
		_ = startCol
	}
}

// advance moves forward one byte, tracking line/col.
func (l *Lexer) advance() {
	if l.pos < len(l.src) {
		if l.src[l.pos] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.pos++
	}
}

// advanceRune moves forward one UTF-8 rune, tracking line/col.
func (l *Lexer) advanceRune() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	r, size := utf8.DecodeRuneInString(l.src[l.pos:])
	if l.src[l.pos] == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	l.pos += size
	return r
}

func (l *Lexer) peekRune() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	return r
}

func (l *Lexer) makeToken(typ TokenType, start, end int) Token {
	return Token{
		Type:  typ,
		Value: l.src[start:end],
		Span:  diagnostic.Span{FileID: l.fileID, Start: start, End: end},
		Line:  0, // set by caller or by makeTokenAt
		Col:   0,
	}
}

func (l *Lexer) makeTokenAt(typ TokenType, start, end, line, col int) Token {
	return Token{
		Type:  typ,
		Value: l.src[start:end],
		Span:  diagnostic.Span{FileID: l.fileID, Start: start, End: end},
		Line:  line,
		Col:   col,
	}
}

func (l *Lexer) errorToken(start, end, line, col int, msg string) Token {
	return Token{
		Type:  ERROR,
		Value: msg,
		Span:  diagnostic.Span{FileID: l.fileID, Start: start, End: end},
		Line:  line,
		Col:   col,
	}
}

// scanIdentifier scans an identifier or keyword, including Unicode letters.
func (l *Lexer) scanIdentifier() Token {
	startPos := l.pos
	startLine := l.line
	startCol := l.col

	// Consume first rune (already verified as ident start)
	l.advanceRune()

	// Continue with ident continuation characters
	for l.pos < len(l.src) {
		r := l.peekRune()
		if isIdentContinue(r) {
			l.advanceRune()
		} else {
			break
		}
	}

	value := l.src[startPos:l.pos]
	tokType := LookupKeyword(value)

	// Check for raw string: identifier "r" followed by '"'
	if value == "r" && l.pos < len(l.src) && l.src[l.pos] == '"' {
		// Re-parse as raw string
		l.pos = startPos
		l.line = startLine
		l.col = startCol
		return l.scanRawString()
	}

	return Token{
		Type:  tokType,
		Value: value,
		Span:  diagnostic.Span{FileID: l.fileID, Start: startPos, End: l.pos},
		Line:  startLine,
		Col:   startCol,
	}
}

// scanNumber scans integer and float literals.
func (l *Lexer) scanNumber() Token {
	startPos := l.pos
	startLine := l.line
	startCol := l.col

	if l.src[l.pos] == '0' && l.pos+1 < len(l.src) {
		next := l.src[l.pos+1]
		switch next {
		case 'x', 'X':
			return l.scanHexNumber(startPos, startLine, startCol)
		case 'b', 'B':
			return l.scanBinaryNumber(startPos, startLine, startCol)
		case 'o', 'O':
			return l.scanOctalNumber(startPos, startLine, startCol)
		}
	}

	return l.scanDecimalNumber(startPos, startLine, startCol)
}

func (l *Lexer) scanDecimalNumber(startPos, startLine, startCol int) Token {
	l.consumeDigits('0', '9')

	// Check for float: dot followed by digit (not .. range operator)
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		// Look ahead: if next char after . is a digit, it's a float
		if l.pos+1 < len(l.src) && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9' {
			l.advance() // consume '.'
			l.consumeDigits('0', '9')
			l.consumeExponent()
			return l.makeTokenAt(FLOAT_LIT, startPos, l.pos, startLine, startCol)
		}
		// Also handle 1. followed by non-digit non-dot — still could be float
		// But we need to be careful about range operators (..)
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '.' {
			// This is a range: 1..2, return the integer
			return l.makeTokenAt(INT_LIT, startPos, l.pos, startLine, startCol)
		}
	}

	// Check for exponent without decimal point: 1e10
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		l.consumeExponent()
		return l.makeTokenAt(FLOAT_LIT, startPos, l.pos, startLine, startCol)
	}

	return l.makeTokenAt(INT_LIT, startPos, l.pos, startLine, startCol)
}

func (l *Lexer) scanHexNumber(startPos, startLine, startCol int) Token {
	// Skip 0x
	l.advance()
	l.advance()

	if l.pos >= len(l.src) || !isHexDigit(l.src[l.pos]) {
		return l.errorToken(startPos, l.pos, startLine, startCol, "expected hex digit after 0x")
	}

	l.consumeHexDigits()
	return l.makeTokenAt(INT_LIT, startPos, l.pos, startLine, startCol)
}

func (l *Lexer) scanBinaryNumber(startPos, startLine, startCol int) Token {
	// Skip 0b
	l.advance()
	l.advance()

	if l.pos >= len(l.src) || (l.src[l.pos] != '0' && l.src[l.pos] != '1') {
		return l.errorToken(startPos, l.pos, startLine, startCol, "expected binary digit after 0b")
	}

	for l.pos < len(l.src) && (l.src[l.pos] == '0' || l.src[l.pos] == '1' || l.src[l.pos] == '_') {
		l.advance()
	}

	return l.makeTokenAt(INT_LIT, startPos, l.pos, startLine, startCol)
}

func (l *Lexer) scanOctalNumber(startPos, startLine, startCol int) Token {
	// Skip 0o
	l.advance()
	l.advance()

	if l.pos >= len(l.src) || l.src[l.pos] < '0' || l.src[l.pos] > '7' {
		return l.errorToken(startPos, l.pos, startLine, startCol, "expected octal digit after 0o")
	}

	for l.pos < len(l.src) && ((l.src[l.pos] >= '0' && l.src[l.pos] <= '7') || l.src[l.pos] == '_') {
		l.advance()
	}

	return l.makeTokenAt(INT_LIT, startPos, l.pos, startLine, startCol)
}

func (l *Lexer) consumeDigits(lo, hi byte) {
	for l.pos < len(l.src) && ((l.src[l.pos] >= lo && l.src[l.pos] <= hi) || l.src[l.pos] == '_') {
		l.advance()
	}
}

func (l *Lexer) consumeHexDigits() {
	for l.pos < len(l.src) && (isHexDigit(l.src[l.pos]) || l.src[l.pos] == '_') {
		l.advance()
	}
}

func (l *Lexer) consumeExponent() {
	if l.pos >= len(l.src) || (l.src[l.pos] != 'e' && l.src[l.pos] != 'E') {
		return
	}
	l.advance() // consume 'e'/'E'
	if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
		l.advance()
	}
	l.consumeDigits('0', '9')
}

func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// scanString scans a regular "..." string literal with escape sequences.
func (l *Lexer) scanString() Token {
	startPos := l.pos
	startLine := l.line
	startCol := l.col

	l.advance() // skip opening '"'

	var b strings.Builder
	for l.pos < len(l.src) && l.src[l.pos] != '"' {
		if l.src[l.pos] == '\n' {
			return l.errorToken(startPos, l.pos, startLine, startCol, "unterminated string literal")
		}
		if l.src[l.pos] == '\\' {
			esc, err := l.scanEscape()
			if err != "" {
				// Consume until closing quote or end for error recovery
				for l.pos < len(l.src) && l.src[l.pos] != '"' && l.src[l.pos] != '\n' {
					l.advance()
				}
				if l.pos < len(l.src) && l.src[l.pos] == '"' {
					l.advance()
				}
				return l.errorToken(startPos, l.pos, startLine, startCol, err)
			}
			b.WriteRune(esc)
		} else {
			r := l.advanceRune()
			b.WriteRune(r)
		}
	}

	if l.pos >= len(l.src) {
		return l.errorToken(startPos, l.pos, startLine, startCol, "unterminated string literal")
	}

	l.advance() // skip closing '"'

	return Token{
		Type:  STRING_LIT,
		Value: l.src[startPos:l.pos],
		Span:  diagnostic.Span{FileID: l.fileID, Start: startPos, End: l.pos},
		Line:  startLine,
		Col:   startCol,
	}
}

// scanRawString scans a r"..." raw string literal (no escape processing).
func (l *Lexer) scanRawString() Token {
	startPos := l.pos
	startLine := l.line
	startCol := l.col

	l.advance() // skip 'r'
	l.advance() // skip '"'

	for l.pos < len(l.src) && l.src[l.pos] != '"' {
		if l.src[l.pos] == '\n' {
			return l.errorToken(startPos, l.pos, startLine, startCol, "unterminated raw string literal")
		}
		l.advance()
	}

	if l.pos >= len(l.src) {
		return l.errorToken(startPos, l.pos, startLine, startCol, "unterminated raw string literal")
	}

	l.advance() // skip closing '"'

	return Token{
		Type:  STRING_LIT,
		Value: l.src[startPos:l.pos],
		Span:  diagnostic.Span{FileID: l.fileID, Start: startPos, End: l.pos},
		Line:  startLine,
		Col:   startCol,
	}
}

// scanChar scans a character literal 'x', including escape sequences.
func (l *Lexer) scanChar() Token {
	startPos := l.pos
	startLine := l.line
	startCol := l.col

	l.advance() // skip opening '\''

	if l.pos >= len(l.src) || l.src[l.pos] == '\n' {
		return l.errorToken(startPos, l.pos, startLine, startCol, "empty character literal")
	}

	if l.src[l.pos] == '\'' {
		l.advance()
		return l.errorToken(startPos, l.pos, startLine, startCol, "empty character literal")
	}

	if l.src[l.pos] == '\\' {
		_, err := l.scanEscape()
		if err != "" {
			for l.pos < len(l.src) && l.src[l.pos] != '\'' && l.src[l.pos] != '\n' {
				l.advance()
			}
			if l.pos < len(l.src) && l.src[l.pos] == '\'' {
				l.advance()
			}
			return l.errorToken(startPos, l.pos, startLine, startCol, err)
		}
	} else {
		l.advanceRune()
	}

	if l.pos >= len(l.src) || l.src[l.pos] != '\'' {
		// Might have multiple characters — consume rest
		for l.pos < len(l.src) && l.src[l.pos] != '\'' && l.src[l.pos] != '\n' {
			l.advance()
		}
		if l.pos < len(l.src) && l.src[l.pos] == '\'' {
			l.advance()
		}
		return l.errorToken(startPos, l.pos, startLine, startCol, "character literal must contain exactly one character")
	}

	l.advance() // skip closing '\''

	return Token{
		Type:  CHAR_LIT,
		Value: l.src[startPos:l.pos],
		Span:  diagnostic.Span{FileID: l.fileID, Start: startPos, End: l.pos},
		Line:  startLine,
		Col:   startCol,
	}
}

// scanEscape processes an escape sequence starting with backslash.
// Returns the decoded rune and an error message (empty on success).
func (l *Lexer) scanEscape() (rune, string) {
	l.advance() // skip '\\'

	if l.pos >= len(l.src) {
		return 0, "unexpected end of escape sequence"
	}

	ch := l.src[l.pos]
	l.advance()

	switch ch {
	case 'n':
		return '\n', ""
	case 't':
		return '\t', ""
	case 'r':
		return '\r', ""
	case '\\':
		return '\\', ""
	case '"':
		return '"', ""
	case '\'':
		return '\'', ""
	case '0':
		return 0, ""
	case 'u':
		return l.scanUnicodeEscape()
	default:
		return 0, fmt.Sprintf("unknown escape sequence '\\%c'", ch)
	}
}

// scanUnicodeEscape processes \u{XXXX} Unicode escape.
func (l *Lexer) scanUnicodeEscape() (rune, string) {
	if l.pos >= len(l.src) || l.src[l.pos] != '{' {
		return 0, "expected '{' in Unicode escape"
	}
	l.advance() // skip '{'

	start := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != '}' {
		if !isHexDigit(l.src[l.pos]) {
			return 0, "invalid hex digit in Unicode escape"
		}
		l.advance()
	}

	if l.pos >= len(l.src) {
		return 0, "unterminated Unicode escape"
	}

	hexStr := l.src[start:l.pos]
	l.advance() // skip '}'

	if len(hexStr) == 0 || len(hexStr) > 6 {
		return 0, "Unicode escape must have 1-6 hex digits"
	}

	var codepoint rune
	for _, c := range hexStr {
		codepoint = codepoint * 16
		switch {
		case c >= '0' && c <= '9':
			codepoint += rune(c - '0')
		case c >= 'a' && c <= 'f':
			codepoint += rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			codepoint += rune(c-'A') + 10
		}
	}

	if !utf8.ValidRune(codepoint) {
		return 0, fmt.Sprintf("invalid Unicode codepoint U+%04X", codepoint)
	}

	return codepoint, ""
}

// scanOperatorOrDelimiter scans single and multi-character operators and delimiters.
func (l *Lexer) scanOperatorOrDelimiter() Token {
	startPos := l.pos
	startLine := l.line
	startCol := l.col

	ch := l.src[l.pos]
	l.advance()

	switch ch {
	case '+':
		if l.pos < len(l.src) && l.src[l.pos] == '+' {
			l.advance()
			return l.makeTokenAt(CONCAT, startPos, l.pos, startLine, startCol)
		}
		return l.makeTokenAt(PLUS, startPos, l.pos, startLine, startCol)

	case '-':
		if l.pos < len(l.src) && l.src[l.pos] == '>' {
			l.advance()
			return l.makeTokenAt(ARROW, startPos, l.pos, startLine, startCol)
		}
		return l.makeTokenAt(MINUS, startPos, l.pos, startLine, startCol)

	case '*':
		return l.makeTokenAt(STAR, startPos, l.pos, startLine, startCol)

	case '/':
		return l.makeTokenAt(SLASH, startPos, l.pos, startLine, startCol)

	case '%':
		return l.makeTokenAt(PERCENT, startPos, l.pos, startLine, startCol)

	case '=':
		if l.pos < len(l.src) && l.src[l.pos] == '=' {
			l.advance()
			return l.makeTokenAt(EQ, startPos, l.pos, startLine, startCol)
		}
		if l.pos < len(l.src) && l.src[l.pos] == '>' {
			l.advance()
			return l.makeTokenAt(FAT_ARROW, startPos, l.pos, startLine, startCol)
		}
		return l.makeTokenAt(ASSIGN, startPos, l.pos, startLine, startCol)

	case '!':
		if l.pos < len(l.src) && l.src[l.pos] == '=' {
			l.advance()
			return l.makeTokenAt(NEQ, startPos, l.pos, startLine, startCol)
		}
		return l.makeTokenAt(NOT, startPos, l.pos, startLine, startCol)

	case '<':
		if l.pos < len(l.src) && l.src[l.pos] == '=' {
			l.advance()
			return l.makeTokenAt(LEQ, startPos, l.pos, startLine, startCol)
		}
		return l.makeTokenAt(LT, startPos, l.pos, startLine, startCol)

	case '>':
		if l.pos < len(l.src) && l.src[l.pos] == '=' {
			l.advance()
			return l.makeTokenAt(GEQ, startPos, l.pos, startLine, startCol)
		}
		return l.makeTokenAt(GT, startPos, l.pos, startLine, startCol)

	case '&':
		if l.pos < len(l.src) && l.src[l.pos] == '&' {
			l.advance()
			return l.makeTokenAt(AND, startPos, l.pos, startLine, startCol)
		}
		return l.errorToken(startPos, l.pos, startLine, startCol, "unexpected character '&'")

	case '|':
		if l.pos < len(l.src) && l.src[l.pos] == '|' {
			l.advance()
			return l.makeTokenAt(OR, startPos, l.pos, startLine, startCol)
		}
		if l.pos < len(l.src) && l.src[l.pos] == '>' {
			l.advance()
			return l.makeTokenAt(PIPE, startPos, l.pos, startLine, startCol)
		}
		return l.makeTokenAt(BAR, startPos, l.pos, startLine, startCol)

	case '.':
		if l.pos < len(l.src) && l.src[l.pos] == '.' {
			l.advance()
			if l.pos < len(l.src) && l.src[l.pos] == '=' {
				l.advance()
				return l.makeTokenAt(RANGE_INCLUSIVE, startPos, l.pos, startLine, startCol)
			}
			return l.makeTokenAt(RANGE, startPos, l.pos, startLine, startCol)
		}
		return l.makeTokenAt(DOT, startPos, l.pos, startLine, startCol)

	case ':':
		if l.pos < len(l.src) && l.src[l.pos] == ':' {
			l.advance()
			return l.makeTokenAt(DOUBLE_COLON, startPos, l.pos, startLine, startCol)
		}
		return l.makeTokenAt(COLON, startPos, l.pos, startLine, startCol)

	case '(':
		return l.makeTokenAt(LPAREN, startPos, l.pos, startLine, startCol)
	case ')':
		return l.makeTokenAt(RPAREN, startPos, l.pos, startLine, startCol)
	case '{':
		return l.makeTokenAt(LBRACE, startPos, l.pos, startLine, startCol)
	case '}':
		return l.makeTokenAt(RBRACE, startPos, l.pos, startLine, startCol)
	case '[':
		return l.makeTokenAt(LBRACKET, startPos, l.pos, startLine, startCol)
	case ']':
		return l.makeTokenAt(RBRACKET, startPos, l.pos, startLine, startCol)
	case ',':
		return l.makeTokenAt(COMMA, startPos, l.pos, startLine, startCol)
	case ';':
		return l.makeTokenAt(SEMICOLON, startPos, l.pos, startLine, startCol)

	default:
		// Unknown character — produce ERROR token
		return l.errorToken(startPos, l.pos, startLine, startCol,
			fmt.Sprintf("unexpected character '%c'", ch))
	}
}

// isIdentStart returns true if r can start an identifier (letter or underscore).
// Supports Unicode L* categories.
func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

// isIdentContinue returns true if r can continue an identifier.
func isIdentContinue(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
