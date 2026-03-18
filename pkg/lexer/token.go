package lexer

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
)

// TokenType identifies the kind of a lexical token.
type TokenType int

const (
	// Literals
	INT_LIT    TokenType = iota // 42, 0xFF, 0b1010, 0o77, 1_000_000
	FLOAT_LIT                   // 3.14, 1e10, 2.5e-3
	STRING_LIT                  // "hello\nworld"
	CHAR_LIT                    // 'a', '\n', '\u{1F600}'
	BOOL_LIT                    // true, false

	// Identifiers & Keywords (25 keywords)
	IDENT
	FN
	LET
	MUT
	IF
	ELSE
	MATCH
	FOR
	WHILE
	LOOP
	BREAK
	CONTINUE
	RETURN
	TYPE
	STRUCT
	TRAIT
	IMPL
	SPAWN
	CHANNEL
	IN
	AS
	PUB
	IMPORT
	MODULE
	SELF_KW   // self (value keyword)
	SELF_TYPE // Self (type keyword)

	// Operators (20)
	PLUS            // +
	MINUS           // -
	STAR            // *
	SLASH           // /
	PERCENT         // %
	EQ              // ==
	NEQ             // !=
	LT              // <
	GT              // >
	LEQ             // <=
	GEQ             // >=
	AND             // &&
	OR              // ||
	NOT             // !
	BAR             // |
	PIPE            // |>
	CONCAT          // ++
	RANGE           // ..
	RANGE_INCLUSIVE // ..=
	ASSIGN          // =
	ARROW           // ->

	// Delimiters
	LPAREN       // (
	RPAREN       // )
	LBRACE       // {
	RBRACE       // }
	LBRACKET     // [
	RBRACKET     // ]
	COMMA        // ,
	COLON        // :
	SEMICOLON    // ;
	DOT          // .
	DOUBLE_COLON // ::
	FAT_ARROW    // =>

	// Special
	NEWLINE
	EOF
	ERROR

	tokenTypeCount // sentinel for counting
)

var tokenTypeNames = [tokenTypeCount]string{
	INT_LIT:         "INT_LIT",
	FLOAT_LIT:       "FLOAT_LIT",
	STRING_LIT:      "STRING_LIT",
	CHAR_LIT:        "CHAR_LIT",
	BOOL_LIT:        "BOOL_LIT",
	IDENT:           "IDENT",
	FN:              "FN",
	LET:             "LET",
	MUT:             "MUT",
	IF:              "IF",
	ELSE:            "ELSE",
	MATCH:           "MATCH",
	FOR:             "FOR",
	WHILE:           "WHILE",
	LOOP:            "LOOP",
	BREAK:           "BREAK",
	CONTINUE:        "CONTINUE",
	RETURN:          "RETURN",
	TYPE:            "TYPE",
	STRUCT:          "STRUCT",
	TRAIT:           "TRAIT",
	IMPL:            "IMPL",
	SPAWN:           "SPAWN",
	CHANNEL:         "CHANNEL",
	IN:              "IN",
	AS:              "AS",
	PUB:             "PUB",
	IMPORT:          "IMPORT",
	MODULE:          "MODULE",
	SELF_KW:         "SELF_KW",
	SELF_TYPE:       "SELF_TYPE",
	PLUS:            "PLUS",
	MINUS:           "MINUS",
	STAR:            "STAR",
	SLASH:           "SLASH",
	PERCENT:         "PERCENT",
	EQ:              "EQ",
	NEQ:             "NEQ",
	LT:              "LT",
	GT:              "GT",
	LEQ:             "LEQ",
	GEQ:             "GEQ",
	AND:             "AND",
	OR:              "OR",
	NOT:             "NOT",
	BAR:             "BAR",
	PIPE:            "PIPE",
	CONCAT:          "CONCAT",
	RANGE:           "RANGE",
	RANGE_INCLUSIVE: "RANGE_INCLUSIVE",
	ASSIGN:          "ASSIGN",
	ARROW:           "ARROW",
	LPAREN:          "LPAREN",
	RPAREN:          "RPAREN",
	LBRACE:          "LBRACE",
	RBRACE:          "RBRACE",
	LBRACKET:        "LBRACKET",
	RBRACKET:        "RBRACKET",
	COMMA:           "COMMA",
	COLON:           "COLON",
	SEMICOLON:       "SEMICOLON",
	DOT:             "DOT",
	DOUBLE_COLON:    "DOUBLE_COLON",
	FAT_ARROW:       "FAT_ARROW",
	NEWLINE:         "NEWLINE",
	EOF:             "EOF",
	ERROR:           "ERROR",
}

func (t TokenType) String() string {
	if t >= 0 && t < tokenTypeCount {
		return tokenTypeNames[t]
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

// IsKeyword returns true if the token type is a keyword.
func (t TokenType) IsKeyword() bool {
	return t >= FN && t <= SELF_TYPE
}

// IsLiteral returns true if the token type is a literal.
func (t TokenType) IsLiteral() bool {
	return t >= INT_LIT && t <= BOOL_LIT
}

// IsOperator returns true if the token type is an operator.
func (t TokenType) IsOperator() bool {
	return t >= PLUS && t <= ARROW
}

// Token represents a single lexical token from the source.
type Token struct {
	Type  TokenType
	Value string          // raw text of the token
	Span  diagnostic.Span // source span (byte offsets + file ID)
	Line  int             // 1-based line number
	Col   int             // 1-based column number
}

func (tok Token) String() string {
	if tok.Value != "" {
		return fmt.Sprintf("%s(%q) at %d:%d", tok.Type, tok.Value, tok.Line, tok.Col)
	}
	return fmt.Sprintf("%s at %d:%d", tok.Type, tok.Line, tok.Col)
}

// keywords maps keyword strings to their TokenType.
var keywords = map[string]TokenType{
	"fn":       FN,
	"let":      LET,
	"mut":      MUT,
	"if":       IF,
	"else":     ELSE,
	"match":    MATCH,
	"for":      FOR,
	"while":    WHILE,
	"loop":     LOOP,
	"break":    BREAK,
	"continue": CONTINUE,
	"return":   RETURN,
	"type":     TYPE,
	"struct":   STRUCT,
	"trait":    TRAIT,
	"impl":     IMPL,
	"spawn":    SPAWN,
	"channel":  CHANNEL,
	"in":       IN,
	"as":       AS,
	"pub":      PUB,
	"import":   IMPORT,
	"module":   MODULE,
	"self":     SELF_KW,
	"Self":     SELF_TYPE,
	"true":     BOOL_LIT,
	"false":    BOOL_LIT,
}

// LookupKeyword returns the keyword TokenType for ident, or IDENT if not a keyword.
func LookupKeyword(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}

// KeywordList returns all keyword strings (useful for "did you mean?" suggestions).
func KeywordList() []string {
	list := make([]string, 0, len(keywords))
	for k := range keywords {
		list = append(list, k)
	}
	return list
}
