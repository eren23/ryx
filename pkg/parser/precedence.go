package parser

import "github.com/ryx-lang/ryx/pkg/lexer"

// Precedence levels for Pratt expression parsing (14 levels).
const (
	PrecNone       = 0  // not an operator
	PrecAssign     = 1  // =                (right-associative)
	PrecPipe       = 2  // |>               (left-associative)
	PrecOr         = 3  // ||               (left-associative)
	PrecAnd        = 4  // &&               (left-associative)
	PrecEquality   = 5  // == !=            (left-associative)
	PrecComparison = 6  // < > <= >=        (left-associative)
	PrecRange      = 7  // .. ..=           (non-associative)
	PrecConcat     = 8  // ++               (left-associative)
	PrecAddition   = 9  // + -              (left-associative)
	PrecMultiply   = 10 // * / %            (left-associative)
	PrecUnary      = 11 // ! - (prefix)
	PrecCall       = 12 // () . [] ::       (left-associative, postfix)
	PrecAs         = 13 // as               (left-associative, postfix)
	PrecPrimary    = 14 // literals, idents, grouped
)

// infixPrec returns the precedence of a token when it appears as an
// infix (or postfix) operator. Returns PrecNone if the token does not
// start an infix expression.
func infixPrec(tok lexer.TokenType) int {
	switch tok {
	case lexer.ASSIGN:
		return PrecAssign
	case lexer.PIPE:
		return PrecPipe
	case lexer.OR:
		return PrecOr
	case lexer.AND:
		return PrecAnd
	case lexer.EQ, lexer.NEQ:
		return PrecEquality
	case lexer.LT, lexer.GT, lexer.LEQ, lexer.GEQ:
		return PrecComparison
	case lexer.RANGE, lexer.RANGE_INCLUSIVE:
		return PrecRange
	case lexer.CONCAT:
		return PrecConcat
	case lexer.PLUS, lexer.MINUS:
		return PrecAddition
	case lexer.STAR, lexer.SLASH, lexer.PERCENT:
		return PrecMultiply
	case lexer.LPAREN, lexer.DOT, lexer.LBRACKET, lexer.DOUBLE_COLON:
		return PrecCall
	case lexer.AS:
		return PrecAs
	default:
		return PrecNone
	}
}

// isRightAssoc returns true for right-associative operators.
func isRightAssoc(tok lexer.TokenType) bool {
	return tok == lexer.ASSIGN
}

// isNonAssoc returns true for non-associative operators.
func isNonAssoc(tok lexer.TokenType) bool {
	return tok == lexer.RANGE || tok == lexer.RANGE_INCLUSIVE
}
