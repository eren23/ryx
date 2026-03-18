package lexer

import (
	"testing"
)

// helper to lex a string and return all tokens
func lex(src string) []Token {
	l := New(src, 0)
	return l.Tokenize()
}

// expectTokens checks the sequence of token types (excluding EOF)
func expectTokens(t *testing.T, src string, expected []TokenType) {
	t.Helper()
	tokens := lex(src)
	// Remove EOF for comparison
	got := make([]TokenType, 0, len(tokens)-1)
	for _, tok := range tokens {
		if tok.Type != EOF {
			got = append(got, tok.Type)
		}
	}
	if len(got) != len(expected) {
		t.Fatalf("token count mismatch for %q:\ngot  %d tokens: %v\nwant %d tokens: %v",
			src, len(got), got, len(expected), expected)
	}
	for i := range got {
		if got[i] != expected[i] {
			t.Errorf("token[%d] for %q: got %v, want %v", i, src, got[i], expected[i])
		}
	}
}

// expectSingle checks a single token (first non-EOF) has the right type and value.
func expectSingle(t *testing.T, src string, typ TokenType, value string) {
	t.Helper()
	tokens := lex(src)
	if len(tokens) < 1 {
		t.Fatalf("no tokens for %q", src)
	}
	tok := tokens[0]
	if tok.Type != typ {
		t.Errorf("for %q: got type %v, want %v", src, tok.Type, typ)
	}
	if tok.Value != value {
		t.Errorf("for %q: got value %q, want %q", src, tok.Value, value)
	}
}

// --- Integer Literals ---

func TestIntLiterals(t *testing.T) {
	tests := []struct {
		src   string
		value string
	}{
		{"0", "0"},
		{"42", "42"},
		{"1_000_000", "1_000_000"},
		{"0xFF", "0xFF"},
		{"0xff", "0xff"},
		{"0xFF_FF", "0xFF_FF"},
		{"0b1010", "0b1010"},
		{"0b1111_0000", "0b1111_0000"},
		{"0o77", "0o77"},
		{"0o77_77", "0o77_77"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, INT_LIT, tt.value)
		})
	}
}

func TestIntLiteralErrors(t *testing.T) {
	tests := []struct {
		src string
		msg string
	}{
		{"0x", "expected hex digit after 0x"},
		{"0b", "expected binary digit after 0b"},
		{"0o", "expected octal digit after 0o"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			tokens := lex(tt.src)
			if tokens[0].Type != ERROR {
				t.Errorf("expected ERROR token for %q, got %v", tt.src, tokens[0].Type)
			}
			if tokens[0].Value != tt.msg {
				t.Errorf("expected error %q, got %q", tt.msg, tokens[0].Value)
			}
		})
	}
}

// --- Float Literals ---

func TestFloatLiterals(t *testing.T) {
	tests := []struct {
		src   string
		value string
	}{
		{"3.14", "3.14"},
		{"0.5", "0.5"},
		{"1e10", "1e10"},
		{"2.5e-3", "2.5e-3"},
		{"1E+5", "1E+5"},
		{"1_000.5", "1_000.5"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, FLOAT_LIT, tt.value)
		})
	}
}

// --- String Literals ---

func TestStringLiterals(t *testing.T) {
	tests := []struct {
		src   string
		value string
	}{
		{`"hello"`, `"hello"`},
		{`""`, `""`},
		{`"hello\nworld"`, `"hello\nworld"`},
		{`"tab\there"`, `"tab\there"`},
		{`"escaped\\"`, `"escaped\\"`},
		{`"quote\""`, `"quote\""`},
		{`"\0"`, `"\0"`},
		{`"\r"`, `"\r"`},
		{`"\u{0041}"`, `"\u{0041}"`},
		{`"\u{1F600}"`, `"\u{1F600}"`},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, STRING_LIT, tt.value)
		})
	}
}

func TestRawStringLiterals(t *testing.T) {
	tests := []struct {
		src   string
		value string
	}{
		{`r"hello"`, `r"hello"`},
		{`r"no\nescape"`, `r"no\nescape"`},
		{`r""`, `r""`},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, STRING_LIT, tt.value)
		})
	}
}

func TestStringErrors(t *testing.T) {
	tests := []struct {
		src string
		msg string
	}{
		{`"unterminated`, "unterminated string literal"},
		{"\"newline\n\"", "unterminated string literal"},
		{`"\q"`, "unknown escape sequence '\\q'"},
		{`"\u{ZZZZ}"`, "invalid hex digit in Unicode escape"},
		{`"\u{}"`, "Unicode escape must have 1-6 hex digits"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			tokens := lex(tt.src)
			if tokens[0].Type != ERROR {
				t.Errorf("expected ERROR for %q, got %v (%q)", tt.src, tokens[0].Type, tokens[0].Value)
			}
			if tokens[0].Value != tt.msg {
				t.Errorf("error message for %q:\ngot  %q\nwant %q", tt.src, tokens[0].Value, tt.msg)
			}
		})
	}
}

// --- Char Literals ---

func TestCharLiterals(t *testing.T) {
	tests := []struct {
		src   string
		value string
	}{
		{"'a'", "'a'"},
		{"'Z'", "'Z'"},
		{`'\n'`, `'\n'`},
		{`'\t'`, `'\t'`},
		{`'\\'`, `'\\'`},
		{`'\0'`, `'\0'`},
		{`'\u{0041}'`, `'\u{0041}'`},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, CHAR_LIT, tt.value)
		})
	}
}

func TestCharErrors(t *testing.T) {
	tests := []struct {
		src string
		msg string
	}{
		{"''", "empty character literal"},
		{"'ab'", "character literal must contain exactly one character"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			tokens := lex(tt.src)
			if tokens[0].Type != ERROR {
				t.Errorf("expected ERROR for %q, got %v", tt.src, tokens[0].Type)
			}
			if tokens[0].Value != tt.msg {
				t.Errorf("error message: got %q, want %q", tokens[0].Value, tt.msg)
			}
		})
	}
}

// --- Bool Literals ---

func TestBoolLiterals(t *testing.T) {
	expectSingle(t, "true", BOOL_LIT, "true")
	expectSingle(t, "false", BOOL_LIT, "false")
}

// --- Keywords ---

func TestKeywords(t *testing.T) {
	tests := []struct {
		src string
		typ TokenType
	}{
		{"fn", FN},
		{"let", LET},
		{"mut", MUT},
		{"if", IF},
		{"else", ELSE},
		{"match", MATCH},
		{"for", FOR},
		{"while", WHILE},
		{"loop", LOOP},
		{"break", BREAK},
		{"continue", CONTINUE},
		{"return", RETURN},
		{"type", TYPE},
		{"struct", STRUCT},
		{"trait", TRAIT},
		{"impl", IMPL},
		{"spawn", SPAWN},
		{"channel", CHANNEL},
		{"in", IN},
		{"as", AS},
		{"pub", PUB},
		{"import", IMPORT},
		{"module", MODULE},
		{"self", SELF_KW},
		{"Self", SELF_TYPE},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, tt.typ, tt.src)
		})
	}
}

// --- Identifiers ---

func TestIdentifiers(t *testing.T) {
	tests := []struct {
		src   string
		value string
	}{
		{"x", "x"},
		{"foo", "foo"},
		{"_bar", "_bar"},
		{"camelCase", "camelCase"},
		{"snake_case", "snake_case"},
		{"_", "_"},
		{"__init__", "__init__"},
		{"x1", "x1"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, IDENT, tt.value)
		})
	}
}

func TestUnicodeIdentifiers(t *testing.T) {
	tests := []struct {
		src   string
		value string
	}{
		{"日本語", "日本語"},
		{"π", "π"},
		{"αβγ", "αβγ"},
		{"名前", "名前"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, IDENT, tt.value)
		})
	}
}

// --- Operators ---

func TestOperators(t *testing.T) {
	tests := []struct {
		src string
		typ TokenType
	}{
		{"+", PLUS},
		{"-", MINUS},
		{"*", STAR},
		{"/", SLASH},
		{"%", PERCENT},
		{"==", EQ},
		{"!=", NEQ},
		{"<", LT},
		{">", GT},
		{"<=", LEQ},
		{">=", GEQ},
		{"&&", AND},
		{"||", OR},
		{"!", NOT},
		{"|>", PIPE},
		{"++", CONCAT},
		{"..", RANGE},
		{"..=", RANGE_INCLUSIVE},
		{"=", ASSIGN},
		{"->", ARROW},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, tt.typ, tt.src)
		})
	}
}

// --- Delimiters ---

func TestDelimiters(t *testing.T) {
	tests := []struct {
		src string
		typ TokenType
	}{
		{"(", LPAREN},
		{")", RPAREN},
		{"{", LBRACE},
		{"}", RBRACE},
		{"[", LBRACKET},
		{"]", RBRACKET},
		{",", COMMA},
		{":", COLON},
		{";", SEMICOLON},
		{".", DOT},
		{"::", DOUBLE_COLON},
		{"=>", FAT_ARROW},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expectSingle(t, tt.src, tt.typ, tt.src)
		})
	}
}

// --- Comments ---

func TestLineComments(t *testing.T) {
	tokens := lex("x // comment\ny")
	types := tokenTypes(tokens)
	expected := []TokenType{IDENT, NEWLINE, IDENT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("got %v, want %v", types, expected)
	}
}

func TestBlockComments(t *testing.T) {
	tokens := lex("x /* comment */ y")
	types := tokenTypes(tokens)
	expected := []TokenType{IDENT, IDENT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("got %v, want %v", types, expected)
	}
}

func TestNestedBlockComments(t *testing.T) {
	tokens := lex("x /* outer /* inner */ still comment */ y")
	types := tokenTypes(tokens)
	expected := []TokenType{IDENT, IDENT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("got %v, want %v", types, expected)
	}
}

func TestBlockCommentWithNewlines(t *testing.T) {
	tokens := lex("x /* comment\nspanning\nlines */ y")
	types := tokenTypes(tokens)
	expected := []TokenType{IDENT, IDENT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("got %v, want %v", types, expected)
	}
}

// --- Newlines ---

func TestNewlines(t *testing.T) {
	tokens := lex("a\nb\nc")
	types := tokenTypes(tokens)
	expected := []TokenType{IDENT, NEWLINE, IDENT, NEWLINE, IDENT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("got %v, want %v", types, expected)
	}
}

// --- Position Tracking ---

func TestPositionTracking(t *testing.T) {
	tokens := lex("let x = 42")
	// let: line 1, col 1
	if tokens[0].Line != 1 || tokens[0].Col != 1 {
		t.Errorf("let: got %d:%d, want 1:1", tokens[0].Line, tokens[0].Col)
	}
	// x: line 1, col 5
	if tokens[1].Line != 1 || tokens[1].Col != 5 {
		t.Errorf("x: got %d:%d, want 1:5", tokens[1].Line, tokens[1].Col)
	}
	// =: line 1, col 7
	if tokens[2].Line != 1 || tokens[2].Col != 7 {
		t.Errorf("=: got %d:%d, want 1:7", tokens[2].Line, tokens[2].Col)
	}
	// 42: line 1, col 9
	if tokens[3].Line != 1 || tokens[3].Col != 9 {
		t.Errorf("42: got %d:%d, want 1:9", tokens[3].Line, tokens[3].Col)
	}
}

func TestMultilinePositionTracking(t *testing.T) {
	tokens := lex("a\nb\nc")
	// a: line 1, col 1
	if tokens[0].Line != 1 || tokens[0].Col != 1 {
		t.Errorf("a: got %d:%d, want 1:1", tokens[0].Line, tokens[0].Col)
	}
	// \n: line 1
	if tokens[1].Type != NEWLINE || tokens[1].Line != 1 {
		t.Errorf("newline1: got line %d, want 1", tokens[1].Line)
	}
	// b: line 2, col 1
	if tokens[2].Line != 2 || tokens[2].Col != 1 {
		t.Errorf("b: got %d:%d, want 2:1", tokens[2].Line, tokens[2].Col)
	}
	// c: line 3, col 1
	if tokens[4].Line != 3 || tokens[4].Col != 1 {
		t.Errorf("c: got %d:%d, want 3:1", tokens[4].Line, tokens[4].Col)
	}
}

func TestByteOffsets(t *testing.T) {
	tokens := lex("a + b")
	// a: offset 0-1
	if tokens[0].Span.Start != 0 || tokens[0].Span.End != 1 {
		t.Errorf("a span: got %d-%d, want 0-1", tokens[0].Span.Start, tokens[0].Span.End)
	}
	// +: offset 2-3
	if tokens[1].Span.Start != 2 || tokens[1].Span.End != 3 {
		t.Errorf("+ span: got %d-%d, want 2-3", tokens[1].Span.Start, tokens[1].Span.End)
	}
	// b: offset 4-5
	if tokens[2].Span.Start != 4 || tokens[2].Span.End != 5 {
		t.Errorf("b span: got %d-%d, want 4-5", tokens[2].Span.Start, tokens[2].Span.End)
	}
}

// --- EOF ---

func TestEmptyInput(t *testing.T) {
	tokens := lex("")
	if len(tokens) != 1 || tokens[0].Type != EOF {
		t.Errorf("empty input: got %v, want [EOF]", tokens)
	}
}

func TestEOFPosition(t *testing.T) {
	tokens := lex("x")
	eof := tokens[len(tokens)-1]
	if eof.Type != EOF {
		t.Errorf("last token should be EOF, got %v", eof.Type)
	}
}

// --- Error Recovery ---

func TestErrorRecovery(t *testing.T) {
	// Lexer should produce ERROR token for '@' and continue
	tokens := lex("x @ y")
	types := tokenTypes(tokens)
	expected := []TokenType{IDENT, ERROR, IDENT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("got %v, want %v", types, expected)
	}
}

func TestMultipleErrors(t *testing.T) {
	tokens := lex("@ # $")
	errorCount := 0
	for _, tok := range tokens {
		if tok.Type == ERROR {
			errorCount++
		}
	}
	if errorCount != 3 {
		t.Errorf("expected 3 ERROR tokens, got %d", errorCount)
	}
}

// --- Complex Expressions ---

func TestComplexExpression(t *testing.T) {
	src := "fn add(x: Int, y: Int) -> Int { return x + y }"
	tokens := lex(src)
	expected := []TokenType{
		FN, IDENT, LPAREN, IDENT, COLON, IDENT, COMMA, IDENT, COLON, IDENT, RPAREN,
		ARROW, IDENT, LBRACE, RETURN, IDENT, PLUS, IDENT, RBRACE, EOF,
	}
	types := tokenTypes(tokens)
	if !tokenTypesEqual(types, expected) {
		t.Errorf("complex expr:\ngot  %v\nwant %v", types, expected)
	}
}

func TestMatchExpression(t *testing.T) {
	src := `match x {
  1 => "one",
  _ => "other",
}`
	tokens := lex(src)
	// Verify it contains expected tokens
	found := map[TokenType]bool{}
	for _, tok := range tokens {
		found[tok.Type] = true
	}
	for _, want := range []TokenType{MATCH, IDENT, LBRACE, INT_LIT, FAT_ARROW, STRING_LIT, COMMA, RBRACE} {
		if !found[want] {
			t.Errorf("missing token type %v in match expression", want)
		}
	}
}

// --- Range vs Float ---

func TestRangeVsFloat(t *testing.T) {
	// 1..2 should be INT RANGE INT
	tokens := lex("1..2")
	types := tokenTypes(tokens)
	expected := []TokenType{INT_LIT, RANGE, INT_LIT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("1..2: got %v, want %v", types, expected)
	}

	// 1..=2 should be INT RANGE_INCLUSIVE INT
	tokens = lex("1..=2")
	types = tokenTypes(tokens)
	expected = []TokenType{INT_LIT, RANGE_INCLUSIVE, INT_LIT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("1..=2: got %v, want %v", types, expected)
	}
}

// --- Self keyword ---

func TestSelfKeyword(t *testing.T) {
	expectSingle(t, "self", SELF_KW, "self")
	expectSingle(t, "Self", SELF_TYPE, "Self")
	// self_foo should be an identifier, not a keyword
	expectSingle(t, "self_foo", IDENT, "self_foo")
}

// --- Operator disambiguation ---

func TestOperatorDisambiguation(t *testing.T) {
	// Ensure ++ is CONCAT, not two PLUS
	tokens := lex("a ++ b")
	types := tokenTypes(tokens)
	expected := []TokenType{IDENT, CONCAT, IDENT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("a ++ b: got %v, want %v", types, expected)
	}

	// Ensure => is FAT_ARROW, not ASSIGN + GT
	tokens = lex("x => y")
	types = tokenTypes(tokens)
	expected = []TokenType{IDENT, FAT_ARROW, IDENT, EOF}
	if !tokenTypesEqual(types, expected) {
		t.Errorf("x => y: got %v, want %v", types, expected)
	}
}

// --- Whitespace handling ---

func TestWhitespaceOnly(t *testing.T) {
	tokens := lex("   \t  \r  ")
	if len(tokens) != 1 || tokens[0].Type != EOF {
		t.Errorf("whitespace only: got %v", tokenTypes(tokens))
	}
}

// --- Spans ---

func TestSpanFileID(t *testing.T) {
	l := New("x", 42)
	tokens := l.Tokenize()
	if tokens[0].Span.FileID != 42 {
		t.Errorf("expected FileID 42, got %d", tokens[0].Span.FileID)
	}
}

// --- All 52 token types ---

func TestAll52TokenTypes(t *testing.T) {
	// Verify we can produce every non-special token type
	// (ERROR is tested elsewhere, NEWLINE and EOF are implicit)
	produced := map[TokenType]bool{}

	// Literals
	for _, src := range []string{"42", "3.14", `"hello"`, "'a'", "true", "false"} {
		tok := lex(src)[0]
		produced[tok.Type] = true
	}

	// Keywords
	for _, kw := range []string{
		"fn", "let", "mut", "if", "else", "match", "for", "while", "loop",
		"break", "continue", "return", "type", "struct", "trait", "impl",
		"spawn", "channel", "in", "as", "pub", "import", "module", "self", "Self",
	} {
		tok := lex(kw)[0]
		produced[tok.Type] = true
	}

	// Identifiers
	produced[IDENT] = true // tested above

	// Operators
	for _, op := range []string{
		"+", "-", "*", "/", "%", "==", "!=", "<", ">", "<=", ">=",
		"&&", "||", "!", "|", "|>", "++", "..", "..=", "=", "->",
	} {
		tok := lex(op)[0]
		produced[tok.Type] = true
	}

	// Delimiters
	for _, del := range []string{"(", ")", "{", "}", "[", "]", ",", ":", ";", ".", "::", "=>"} {
		tok := lex(del)[0]
		produced[tok.Type] = true
	}

	// Special
	produced[NEWLINE] = true
	produced[EOF] = true
	produced[ERROR] = true

	// Check all 52
	for i := TokenType(0); i < tokenTypeCount; i++ {
		if !produced[i] {
			t.Errorf("token type %v (%d) was never produced", i, i)
		}
	}
}

// --- helpers ---

func tokenTypes(tokens []Token) []TokenType {
	types := make([]TokenType, len(tokens))
	for i, tok := range tokens {
		types[i] = tok.Type
	}
	return types
}

func tokenTypesEqual(a, b []TokenType) bool {
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
