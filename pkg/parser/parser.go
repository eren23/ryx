package parser

import (
	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/lexer"
)

// Parser is a recursive-descent + Pratt expression parser for Ryx.
type Parser struct {
	tokens     []lexer.Token
	pos        int
	errors     []*ParseError
	delimStack []delimInfo
}

// Parse tokenises src and parses it into an AST.
func Parse(src string, fileID int) *ParseResult {
	l := lexer.New(src, fileID)
	raw := l.Tokenize()

	// Filter out NEWLINE and ERROR tokens (lexer errors are separate).
	tokens := make([]lexer.Token, 0, len(raw))
	for _, tok := range raw {
		switch tok.Type {
		case lexer.NEWLINE, lexer.ERROR:
			continue
		default:
			tokens = append(tokens, tok)
		}
	}

	p := &Parser{tokens: tokens}
	prog := p.parseProgram()
	return &ParseResult{Program: prog, Errors: p.errors}
}

// ParseExpr is a convenience entry point that parses a single expression.
func ParseExpr(src string, fileID int) (Expr, []*ParseError) {
	l := lexer.New(src, fileID)
	raw := l.Tokenize()
	tokens := make([]lexer.Token, 0, len(raw))
	for _, tok := range raw {
		switch tok.Type {
		case lexer.NEWLINE, lexer.ERROR:
			continue
		default:
			tokens = append(tokens, tok)
		}
	}
	p := &Parser{tokens: tokens}
	expr := p.parseExpr()
	return expr, p.errors
}

// ---------------------------------------------------------------------------
// Token access helpers
// ---------------------------------------------------------------------------

func (p *Parser) peek() lexer.Token {
	if p.pos >= len(p.tokens) {
		// Synthesise EOF.
		if len(p.tokens) > 0 {
			last := p.tokens[len(p.tokens)-1]
			return lexer.Token{Type: lexer.EOF, Span: last.Span}
		}
		return lexer.Token{Type: lexer.EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekAt(offset int) lexer.Token {
	idx := p.pos + offset
	if idx >= len(p.tokens) || idx < 0 {
		return lexer.Token{Type: lexer.EOF}
	}
	return p.tokens[idx]
}

func (p *Parser) advance() lexer.Token {
	tok := p.peek()
	if tok.Type != lexer.EOF {
		p.pos++
	}
	return tok
}

func (p *Parser) prev() lexer.Token {
	if p.pos > 0 && p.pos-1 < len(p.tokens) {
		return p.tokens[p.pos-1]
	}
	return lexer.Token{Type: lexer.EOF}
}

func (p *Parser) check(t lexer.TokenType) bool {
	return p.peek().Type == t
}

func (p *Parser) atEnd() bool {
	return p.peek().Type == lexer.EOF
}

func (p *Parser) match(types ...lexer.TokenType) bool {
	for _, t := range types {
		if p.check(t) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *Parser) expect(t lexer.TokenType) lexer.Token {
	if p.check(t) {
		return p.advance()
	}
	p.errorExpected(describeTokenType(t))
	return p.peek()
}

func (p *Parser) expectSemicolon() {
	if p.check(lexer.SEMICOLON) {
		p.advance()
		return
	}
	// Auto-insert before `}` (the next token is RBRACE).
	if p.check(lexer.RBRACE) {
		return
	}
	p.errorExpected("`;`")
}

func describeTokenType(t lexer.TokenType) string {
	switch t {
	case lexer.IDENT:
		return "identifier"
	case lexer.SEMICOLON:
		return "`;`"
	case lexer.RPAREN:
		return "`)`"
	case lexer.RBRACE:
		return "`}`"
	case lexer.RBRACKET:
		return "`]`"
	case lexer.LBRACE:
		return "`{`"
	case lexer.LPAREN:
		return "`(`"
	case lexer.COLON:
		return "`:`"
	case lexer.COMMA:
		return "`,`"
	case lexer.ARROW:
		return "`->`"
	case lexer.FAT_ARROW:
		return "`=>`"
	case lexer.ASSIGN:
		return "`=`"
	default:
		return "`" + t.String() + "`"
	}
}

// makeSpan creates a span from start to the end of the most recently consumed token.
func (p *Parser) makeSpan(start diagnostic.Span) diagnostic.Span {
	prev := p.prev()
	if prev.Type == lexer.EOF && start.End > start.Start {
		return start
	}
	return start.Merge(prev.Span)
}

// ---------------------------------------------------------------------------
// Program
// ---------------------------------------------------------------------------

func (p *Parser) parseProgram() *Program {
	start := p.peek().Span
	var items []Item
	for !p.atEnd() {
		startPos := p.pos
		item := p.parseItem()
		if item != nil {
			items = append(items, item)
		} else if p.pos == startPos {
			// No progress — advance to avoid infinite loop.
			p.advance()
		}
	}
	return &Program{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Items:    items,
	}
}

// ---------------------------------------------------------------------------
// Items
// ---------------------------------------------------------------------------

func (p *Parser) parseItem() Item {
	pub := false
	pubStart := p.peek().Span
	if p.match(lexer.PUB) {
		pub = true
	}

	switch p.peek().Type {
	case lexer.FN:
		return p.parseFnDef(pub, pubStart)
	case lexer.TYPE:
		return p.parseTypeDef(pub, pubStart)
	case lexer.STRUCT:
		return p.parseStructDef(pub, pubStart)
	case lexer.TRAIT:
		return p.parseTraitDef(pub, pubStart)
	case lexer.IMPL:
		if pub {
			p.error(pubStart, "`pub` is not allowed on `impl` blocks")
		}
		return p.parseImplBlock()
	case lexer.IMPORT:
		if pub {
			p.error(pubStart, "`pub` is not allowed on `import`")
		}
		return p.parseImportDecl()
	case lexer.MODULE:
		if pub {
			p.error(pubStart, "`pub` is not allowed on `module`")
		}
		return p.parseModuleDecl()
	default:
		if pub {
			p.error(pubStart, "`pub` must be followed by `fn`, `type`, `struct`, or `trait`")
		}
		p.errorUnexpected()
		p.synchronize()
		return nil
	}
}

// ---------------------------------------------------------------------------
// fn
// ---------------------------------------------------------------------------

func (p *Parser) parseFnDef(pub bool, start diagnostic.Span) *FnDef {
	p.expect(lexer.FN)
	if !pub {
		start = p.prev().Span
	}

	name := p.expect(lexer.IDENT).Value

	var gp *GenericParams
	if p.check(lexer.LT) {
		gp = p.parseGenericParams()
	}

	p.expect(lexer.LPAREN)
	params := p.parseParamList()
	p.expect(lexer.RPAREN)

	var ret TypeExpr
	if p.match(lexer.ARROW) {
		ret = p.parseTypeExpr()
	}

	var body *Block
	if p.check(lexer.LBRACE) {
		body = p.parseBlock()
	} else {
		p.expectSemicolon()
	}

	return &FnDef{
		nodeBase:   nodeBase{pos: p.makeSpan(start)},
		Public:     pub,
		Name:       name,
		GenParams:  gp,
		Params:     params,
		ReturnType: ret,
		Body:       body,
	}
}

func (p *Parser) parseParamList() []*Param {
	var params []*Param
	for !p.check(lexer.RPAREN) && !p.atEnd() {
		params = append(params, p.parseParam())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	return params
}

func (p *Parser) parseParam() *Param {
	start := p.peek().Span

	// Handle `self` parameter in methods.
	if p.check(lexer.SELF_KW) {
		tok := p.advance()
		return &Param{
			nodeBase: nodeBase{pos: tok.Span},
			Name:     "self",
		}
	}

	name := p.expect(lexer.IDENT).Value

	var typ TypeExpr
	if p.match(lexer.COLON) {
		typ = p.parseTypeExpr()
	}

	return &Param{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Name:     name,
		Type:     typ,
	}
}

// ---------------------------------------------------------------------------
// type (ADT / enum)
// ---------------------------------------------------------------------------

func (p *Parser) parseTypeDef(pub bool, start diagnostic.Span) *TypeDef {
	p.expect(lexer.TYPE)
	if !pub {
		start = p.prev().Span
	}

	name := p.expect(lexer.IDENT).Value

	var gp *GenericParams
	if p.check(lexer.LT) {
		gp = p.parseGenericParams()
	}

	p.expect(lexer.LBRACE)
	p.pushDelim(p.prev(), "brace")
	var variants []*Variant
	for !p.check(lexer.RBRACE) && !p.atEnd() {
		variants = append(variants, p.parseVariant())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.RBRACE)
	p.popDelim()

	return &TypeDef{
		nodeBase:  nodeBase{pos: p.makeSpan(start)},
		Public:    pub,
		Name:      name,
		GenParams: gp,
		Variants:  variants,
	}
}

func (p *Parser) parseVariant() *Variant {
	start := p.peek().Span
	name := p.expect(lexer.IDENT).Value

	var fields []TypeExpr
	if p.match(lexer.LPAREN) {
		for !p.check(lexer.RPAREN) && !p.atEnd() {
			fields = append(fields, p.parseTypeExpr())
			if !p.match(lexer.COMMA) {
				break
			}
		}
		p.expect(lexer.RPAREN)
	}

	return &Variant{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Name:     name,
		Fields:   fields,
	}
}

// ---------------------------------------------------------------------------
// struct
// ---------------------------------------------------------------------------

func (p *Parser) parseStructDef(pub bool, start diagnostic.Span) *StructDef {
	p.expect(lexer.STRUCT)
	if !pub {
		start = p.prev().Span
	}

	name := p.expect(lexer.IDENT).Value

	var gp *GenericParams
	if p.check(lexer.LT) {
		gp = p.parseGenericParams()
	}

	p.expect(lexer.LBRACE)
	p.pushDelim(p.prev(), "brace")
	var fields []*Field
	for !p.check(lexer.RBRACE) && !p.atEnd() {
		fields = append(fields, p.parseField())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.RBRACE)
	p.popDelim()

	return &StructDef{
		nodeBase:  nodeBase{pos: p.makeSpan(start)},
		Public:    pub,
		Name:      name,
		GenParams: gp,
		Fields:    fields,
	}
}

func (p *Parser) parseField() *Field {
	start := p.peek().Span
	fpub := p.match(lexer.PUB)
	name := p.expect(lexer.IDENT).Value
	p.expect(lexer.COLON)
	typ := p.parseTypeExpr()
	return &Field{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Public:   fpub,
		Name:     name,
		Type:     typ,
	}
}

// ---------------------------------------------------------------------------
// trait
// ---------------------------------------------------------------------------

func (p *Parser) parseTraitDef(pub bool, start diagnostic.Span) *TraitDef {
	p.expect(lexer.TRAIT)
	if !pub {
		start = p.prev().Span
	}

	name := p.expect(lexer.IDENT).Value

	var gp *GenericParams
	if p.check(lexer.LT) {
		gp = p.parseGenericParams()
	}

	p.expect(lexer.LBRACE)
	p.pushDelim(p.prev(), "brace")
	var methods []*FnDef
	for !p.check(lexer.RBRACE) && !p.atEnd() {
		fn := p.parseFnDef(false, p.peek().Span)
		methods = append(methods, fn)
	}
	p.expect(lexer.RBRACE)
	p.popDelim()

	return &TraitDef{
		nodeBase:  nodeBase{pos: p.makeSpan(start)},
		Public:    pub,
		Name:      name,
		GenParams: gp,
		Methods:   methods,
	}
}

// ---------------------------------------------------------------------------
// impl
// ---------------------------------------------------------------------------

func (p *Parser) parseImplBlock() *ImplBlock {
	start := p.expect(lexer.IMPL).Span

	var gp *GenericParams
	if p.check(lexer.LT) {
		gp = p.parseGenericParams()
	}

	// Parse the first type — could be trait name or target type.
	firstType := p.parseTypeExpr()

	var traitName string
	var traitGens []TypeExpr
	var targetType TypeExpr

	if p.match(lexer.FOR) {
		// `impl Trait for Type`
		if nt, ok := firstType.(*NamedType); ok {
			traitName = nt.Name
			traitGens = nt.GenArgs
		} else {
			p.error(firstType.Span(), "expected trait name")
		}
		targetType = p.parseTypeExpr()
	} else {
		// Inherent impl: `impl Type`
		targetType = firstType
	}

	p.expect(lexer.LBRACE)
	p.pushDelim(p.prev(), "brace")
	var methods []*FnDef
	for !p.check(lexer.RBRACE) && !p.atEnd() {
		pub := false
		pubStart := p.peek().Span
		if p.match(lexer.PUB) {
			pub = true
		}
		fn := p.parseFnDef(pub, pubStart)
		methods = append(methods, fn)
	}
	p.expect(lexer.RBRACE)
	p.popDelim()

	return &ImplBlock{
		nodeBase:   nodeBase{pos: p.makeSpan(start)},
		GenParams:  gp,
		TraitName:  traitName,
		TraitGens:  traitGens,
		TargetType: targetType,
		Methods:    methods,
	}
}

// ---------------------------------------------------------------------------
// import / module
// ---------------------------------------------------------------------------

func (p *Parser) parseImportDecl() *ImportDecl {
	start := p.expect(lexer.IMPORT).Span
	var path []string
	path = append(path, p.expect(lexer.IDENT).Value)
	for p.match(lexer.DOUBLE_COLON) {
		path = append(path, p.expect(lexer.IDENT).Value)
	}
	alias := ""
	if p.match(lexer.AS) {
		alias = p.expect(lexer.IDENT).Value
	}
	p.expectSemicolon()
	return &ImportDecl{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Path:     path,
		Alias:    alias,
	}
}

func (p *Parser) parseModuleDecl() *ModuleDecl {
	start := p.expect(lexer.MODULE).Span
	name := p.expect(lexer.IDENT).Value
	p.expectSemicolon()
	return &ModuleDecl{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Name:     name,
	}
}

// ---------------------------------------------------------------------------
// Generics
// ---------------------------------------------------------------------------

func (p *Parser) parseGenericParams() *GenericParams {
	start := p.expect(lexer.LT).Span
	var params []*GenericParam
	for !p.check(lexer.GT) && !p.atEnd() {
		params = append(params, p.parseGenericParam())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.GT)
	return &GenericParams{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Params:   params,
	}
}

func (p *Parser) parseGenericParam() *GenericParam {
	start := p.peek().Span
	name := p.expect(lexer.IDENT).Value
	var bounds []TypeExpr
	if p.match(lexer.COLON) {
		bounds = append(bounds, p.parseTypeExpr())
		for p.match(lexer.PLUS) {
			bounds = append(bounds, p.parseTypeExpr())
		}
	}
	return &GenericParam{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Name:     name,
		Bounds:   bounds,
	}
}

func (p *Parser) parseGenericArgs() []TypeExpr {
	p.expect(lexer.LT)
	var args []TypeExpr
	for !p.check(lexer.GT) && !p.atEnd() {
		args = append(args, p.parseTypeExpr())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.GT)
	return args
}

// ---------------------------------------------------------------------------
// Block
// ---------------------------------------------------------------------------

func (p *Parser) parseBlock() *Block {
	start := p.expect(lexer.LBRACE).Span
	p.pushDelim(p.prev(), "brace")
	var stmts []Stmt
	var trailing Expr

	for !p.check(lexer.RBRACE) && !p.atEnd() {
		startPos := p.pos
		// Try to parse a statement.
		stmt, isTrailing := p.parseStmtOrTrailing()
		if isTrailing {
			// The last expression in the block without a semicolon.
			if es, ok := stmt.(*ExprStmt); ok {
				trailing = es.Expr
			}
			break
		}
		if stmt != nil {
			stmts = append(stmts, stmt)
		} else if p.pos == startPos {
			// No progress — advance to avoid infinite loop.
			p.advance()
		}
	}

	p.expect(lexer.RBRACE)
	p.popDelim()

	return &Block{
		nodeBase:     nodeBase{pos: p.makeSpan(start)},
		Stmts:        stmts,
		TrailingExpr: trailing,
	}
}

// parseStmtOrTrailing parses a statement. If the statement is an expression
// without a trailing semicolon (and next token is `}`), isTrailing is true.
func (p *Parser) parseStmtOrTrailing() (Stmt, bool) {
	switch p.peek().Type {
	case lexer.LET:
		return p.parseLetStmt(), false
	case lexer.RETURN:
		return p.parseReturnStmt(), false
	default:
		return p.parseExprStmtOrTrailing()
	}
}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

func (p *Parser) parseLetStmt() *LetStmt {
	start := p.expect(lexer.LET).Span
	mut := p.match(lexer.MUT)
	name := p.expect(lexer.IDENT).Value

	var typ TypeExpr
	if p.match(lexer.COLON) {
		typ = p.parseTypeExpr()
	}

	var val Expr
	if p.match(lexer.ASSIGN) {
		val = p.parseExpr()
	}

	p.expectSemicolon()

	return &LetStmt{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Mutable:  mut,
		Name:     name,
		Type:     typ,
		Value:    val,
	}
}

func (p *Parser) parseReturnStmt() *ReturnStmt {
	start := p.expect(lexer.RETURN).Span
	var val Expr
	if !p.check(lexer.SEMICOLON) && !p.check(lexer.RBRACE) && !p.atEnd() {
		val = p.parseExpr()
	}
	p.expectSemicolon()
	return &ReturnStmt{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Value:    val,
	}
}

func (p *Parser) parseExprStmtOrTrailing() (Stmt, bool) {
	expr := p.parseExpr()
	if expr == nil {
		p.errorUnexpected()
		p.synchronize()
		return nil, false
	}

	// If the next token is `}`, this is a trailing expression.
	if p.check(lexer.RBRACE) {
		stmt := &ExprStmt{
			nodeBase: nodeBase{pos: expr.Span()},
			Expr:     expr,
		}
		return stmt, true
	}

	p.expectSemicolon()
	return &ExprStmt{
		nodeBase: nodeBase{pos: expr.Span()},
		Expr:     expr,
	}, false
}

// ---------------------------------------------------------------------------
// Expressions — Pratt parser
// ---------------------------------------------------------------------------

func (p *Parser) parseExpr() Expr {
	return p.parsePratt(PrecAssign)
}

func (p *Parser) parsePratt(minPrec int) Expr {
	left := p.parsePrefixExpr()
	if left == nil {
		return nil
	}

	for {
		tok := p.peek()
		prec := infixPrec(tok.Type)
		if prec < minPrec {
			break
		}
		if prec == PrecNone {
			break
		}
		left = p.parseInfixExpr(left, prec, tok.Type)
	}

	return left
}

// ---------------------------------------------------------------------------
// Prefix (NUD) expressions
// ---------------------------------------------------------------------------

func (p *Parser) parsePrefixExpr() Expr {
	tok := p.peek()
	switch tok.Type {
	case lexer.INT_LIT:
		p.advance()
		return &IntLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value}
	case lexer.FLOAT_LIT:
		p.advance()
		return &FloatLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value}
	case lexer.STRING_LIT:
		p.advance()
		return &StringLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value}
	case lexer.CHAR_LIT:
		p.advance()
		return &CharLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value}
	case lexer.BOOL_LIT:
		p.advance()
		return &BoolLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value == "true"}

	case lexer.IDENT:
		return p.parseIdentOrStructLit()

	case lexer.SELF_KW:
		p.advance()
		return &SelfExpr{nodeBase: nodeBase{pos: tok.Span}}

	case lexer.NOT, lexer.MINUS:
		return p.parseUnaryExpr()

	case lexer.LPAREN:
		return p.parseGroupOrTuple()

	case lexer.LBRACKET:
		return p.parseArrayLit()

	case lexer.LBRACE:
		return p.parseBlock()

	case lexer.IF:
		return p.parseIfExpr()

	case lexer.MATCH:
		return p.parseMatchExpr()

	case lexer.FOR:
		return p.parseForExpr()

	case lexer.WHILE:
		return p.parseWhileExpr()

	case lexer.LOOP:
		return p.parseLoopExpr()

	case lexer.BAR:
		return p.parseLambdaExpr()

	case lexer.OR:
		// || is zero-param lambda shorthand: || body
		return p.parseZeroParamLambda()

	case lexer.PIPE:
		// |> cannot start an expression
		return nil

	case lexer.SPAWN:
		return p.parseSpawnExpr()

	case lexer.CHANNEL:
		return p.parseChannelExpr()

	case lexer.BREAK:
		p.advance()
		return &BreakExpr{nodeBase: nodeBase{pos: tok.Span}}

	case lexer.CONTINUE:
		p.advance()
		return &ContinueExpr{nodeBase: nodeBase{pos: tok.Span}}

	case lexer.RETURN:
		return p.parseReturnExpr()

	default:
		return nil
	}
}

// parseIdentOrStructLit handles plain identifiers, path expressions (a::b),
// and struct literals (Name { ... }).
func (p *Parser) parseIdentOrStructLit() Expr {
	tok := p.advance() // consume IDENT
	name := tok.Value

	// Check for struct literal: IDENT { field: value, ... }
	// We need to disambiguate from a block expression. Heuristic: if the
	// token after `{` is `IDENT :`, treat it as a struct literal.
	if p.check(lexer.LBRACE) && p.isStructLitAhead() {
		return p.parseStructLitBody(name, tok.Span)
	}

	return &Ident{nodeBase: nodeBase{pos: tok.Span}, Name: name}
}

// isStructLitAhead peeks ahead to determine if `{ IDENT : ` or `{ }` follows.
func (p *Parser) isStructLitAhead() bool {
	// `{ }` → empty struct literal.
	if p.peekAt(1).Type == lexer.RBRACE {
		return true
	}
	// `{ IDENT : ` → struct literal.
	if p.peekAt(1).Type == lexer.IDENT && p.peekAt(2).Type == lexer.COLON {
		return true
	}
	return false
}

func (p *Parser) parseStructLitBody(name string, start diagnostic.Span) *StructLit {
	p.expect(lexer.LBRACE)
	p.pushDelim(p.prev(), "brace")
	var fields []*FieldInit
	for !p.check(lexer.RBRACE) && !p.atEnd() {
		fStart := p.peek().Span
		fname := p.expect(lexer.IDENT).Value
		p.expect(lexer.COLON)
		fval := p.parseExpr()
		fields = append(fields, &FieldInit{
			nodeBase: nodeBase{pos: p.makeSpan(fStart)},
			Name:     fname,
			Value:    fval,
		})
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.RBRACE)
	p.popDelim()
	return &StructLit{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Name:     name,
		Fields:   fields,
	}
}

func (p *Parser) parseUnaryExpr() Expr {
	tok := p.advance() // ! or -
	operand := p.parsePratt(PrecUnary)
	if operand == nil {
		p.errorExpected("expression")
		return &UnaryExpr{nodeBase: nodeBase{pos: tok.Span}, Op: tok.Type}
	}
	return &UnaryExpr{
		nodeBase: nodeBase{pos: tok.Span.Merge(operand.Span())},
		Op:       tok.Type,
		Operand:  operand,
	}
}

func (p *Parser) parseGroupOrTuple() Expr {
	start := p.expect(lexer.LPAREN).Span
	p.pushDelim(p.prev(), "parenthesis")

	// Empty tuple: ()
	if p.check(lexer.RPAREN) {
		p.advance()
		p.popDelim()
		return &TupleLit{
			nodeBase: nodeBase{pos: p.makeSpan(start)},
		}
	}

	first := p.parseExpr()

	// Tuple: (a, b, ...)
	if p.match(lexer.COMMA) {
		elems := []Expr{first}
		for !p.check(lexer.RPAREN) && !p.atEnd() {
			elems = append(elems, p.parseExpr())
			if !p.match(lexer.COMMA) {
				break
			}
		}
		p.expect(lexer.RPAREN)
		p.popDelim()
		return &TupleLit{
			nodeBase: nodeBase{pos: p.makeSpan(start)},
			Elems:    elems,
		}
	}

	// Grouped expression: (a)
	p.expect(lexer.RPAREN)
	p.popDelim()
	return &GroupExpr{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Inner:    first,
	}
}

func (p *Parser) parseArrayLit() Expr {
	start := p.expect(lexer.LBRACKET).Span
	p.pushDelim(p.prev(), "bracket")
	var elems []Expr
	for !p.check(lexer.RBRACKET) && !p.atEnd() {
		elems = append(elems, p.parseExpr())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.RBRACKET)
	p.popDelim()
	return &ArrayLit{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Elems:    elems,
	}
}

func (p *Parser) parseIfExpr() Expr {
	start := p.expect(lexer.IF).Span
	cond := p.parseExpr()
	then := p.parseBlock()

	var elseExpr Expr
	if p.match(lexer.ELSE) {
		if p.check(lexer.IF) {
			elseExpr = p.parseIfExpr()
		} else {
			elseExpr = p.parseBlock()
		}
	}

	return &IfExpr{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Cond:     cond,
		Then:     then,
		Else:     elseExpr,
	}
}

func (p *Parser) parseMatchExpr() Expr {
	start := p.expect(lexer.MATCH).Span
	scrutinee := p.parseExpr()

	p.expect(lexer.LBRACE)
	p.pushDelim(p.prev(), "brace")
	var arms []*MatchArm
	for !p.check(lexer.RBRACE) && !p.atEnd() {
		startPos := p.pos
		arms = append(arms, p.parseMatchArm())
		if !p.match(lexer.COMMA) {
			break
		}
		if p.pos == startPos {
			p.advance()
		}
	}
	p.expect(lexer.RBRACE)
	p.popDelim()

	return &MatchExpr{
		nodeBase:  nodeBase{pos: p.makeSpan(start)},
		Scrutinee: scrutinee,
		Arms:      arms,
	}
}

func (p *Parser) parseMatchArm() *MatchArm {
	start := p.peek().Span
	pat := p.parsePattern()

	var guard Expr
	if p.match(lexer.IF) {
		guard = p.parseExpr()
	}

	p.expect(lexer.FAT_ARROW)
	body := p.parseExpr()

	return &MatchArm{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Pattern:  pat,
		Guard:    guard,
		Body:     body,
	}
}

func (p *Parser) parseForExpr() Expr {
	start := p.expect(lexer.FOR).Span
	binding := p.expect(lexer.IDENT).Value
	p.expect(lexer.IN)
	iter := p.parseExpr()
	body := p.parseBlock()
	return &ForExpr{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Binding:  binding,
		Iter:     iter,
		Body:     body,
	}
}

func (p *Parser) parseWhileExpr() Expr {
	start := p.expect(lexer.WHILE).Span
	cond := p.parseExpr()
	body := p.parseBlock()
	return &WhileExpr{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Cond:     cond,
		Body:     body,
	}
}

func (p *Parser) parseLoopExpr() Expr {
	start := p.expect(lexer.LOOP).Span
	body := p.parseBlock()
	return &LoopExpr{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Body:     body,
	}
}

func (p *Parser) parseSpawnExpr() Expr {
	start := p.expect(lexer.SPAWN).Span
	body := p.parseBlock()
	return &SpawnExpr{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Body:     body,
	}
}

func (p *Parser) parseChannelExpr() Expr {
	start := p.expect(lexer.CHANNEL).Span

	// channel<Type>(size)
	var elemType TypeExpr
	if p.check(lexer.LT) {
		p.advance()
		elemType = p.parseTypeExpr()
		p.expect(lexer.GT)
	}

	var size Expr
	if p.match(lexer.LPAREN) {
		if !p.check(lexer.RPAREN) {
			size = p.parseExpr()
		}
		p.expect(lexer.RPAREN)
	}

	return &ChannelExpr{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		ElemType: elemType,
		Size:     size,
	}
}

func (p *Parser) parseReturnExpr() Expr {
	start := p.expect(lexer.RETURN).Span
	var val Expr
	if !p.check(lexer.SEMICOLON) && !p.check(lexer.RBRACE) && !p.atEnd() {
		val = p.parseExpr()
	}
	return &ReturnExpr{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Value:    val,
	}
}

// ---------------------------------------------------------------------------
// Lambda expressions: |params| body  or  |params| -> RetType body
// ---------------------------------------------------------------------------

func (p *Parser) parseLambdaExpr() Expr {
	start := p.expect(lexer.BAR).Span

	var params []*Param
	if !p.check(lexer.BAR) {
		params = p.parseLambdaParams()
	}
	p.expect(lexer.BAR)

	var ret TypeExpr
	if p.match(lexer.ARROW) {
		ret = p.parseTypeExpr()
	}

	body := p.parseExpr()

	return &LambdaExpr{
		nodeBase:   nodeBase{pos: p.makeSpan(start)},
		Params:     params,
		ReturnType: ret,
		Body:       body,
	}
}

func (p *Parser) parseZeroParamLambda() Expr {
	start := p.advance().Span // consume ||

	var ret TypeExpr
	if p.match(lexer.ARROW) {
		ret = p.parseTypeExpr()
	}

	body := p.parseExpr()

	return &LambdaExpr{
		nodeBase:   nodeBase{pos: p.makeSpan(start)},
		ReturnType: ret,
		Body:       body,
	}
}

func (p *Parser) parseLambdaParams() []*Param {
	var params []*Param
	for !p.check(lexer.BAR) && !p.atEnd() {
		params = append(params, p.parseLambdaParam())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	return params
}

func (p *Parser) parseLambdaParam() *Param {
	start := p.peek().Span
	name := p.expect(lexer.IDENT).Value
	var typ TypeExpr
	if p.match(lexer.COLON) {
		typ = p.parseTypeExpr()
	}
	return &Param{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Name:     name,
		Type:     typ,
	}
}

// ---------------------------------------------------------------------------
// Infix (LED) expressions
// ---------------------------------------------------------------------------

func (p *Parser) parseInfixExpr(left Expr, prec int, op lexer.TokenType) Expr {
	switch op {
	case lexer.LPAREN:
		return p.parseCallExpr(left)
	case lexer.DOT:
		return p.parseFieldExpr(left)
	case lexer.LBRACKET:
		return p.parseIndexExprInfix(left)
	case lexer.DOUBLE_COLON:
		return p.parsePathExpr(left)
	case lexer.AS:
		return p.parseAsExpr(left)
	default:
		return p.parseBinaryExpr(left, prec, op)
	}
}

func (p *Parser) parseBinaryExpr(left Expr, prec int, op lexer.TokenType) Expr {
	p.advance() // consume operator

	nextPrec := prec + 1 // left-associative
	if isRightAssoc(op) {
		nextPrec = prec // right-associative
	}
	if isNonAssoc(op) {
		nextPrec = prec + 1 // prevent chaining
	}

	right := p.parsePratt(nextPrec)
	if right == nil {
		p.errorExpected("expression after operator")
		return left
	}

	return &BinaryExpr{
		nodeBase: nodeBase{pos: left.Span().Merge(right.Span())},
		Left:     left,
		Op:       op,
		Right:    right,
	}
}

func (p *Parser) parseCallExpr(fn Expr) Expr {
	p.advance() // consume (
	p.pushDelim(p.prev(), "parenthesis")
	var args []Expr
	for !p.check(lexer.RPAREN) && !p.atEnd() {
		args = append(args, p.parseExpr())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.RPAREN)
	p.popDelim()
	return &CallExpr{
		nodeBase: nodeBase{pos: fn.Span().Merge(p.prev().Span)},
		Func:     fn,
		Args:     args,
	}
}

func (p *Parser) parseFieldExpr(object Expr) Expr {
	p.advance() // consume .
	fieldTok := p.expect(lexer.IDENT)
	return &FieldExpr{
		nodeBase: nodeBase{pos: object.Span().Merge(fieldTok.Span)},
		Object:   object,
		Field:    fieldTok.Value,
	}
}

func (p *Parser) parseIndexExprInfix(object Expr) Expr {
	p.advance() // consume [
	p.pushDelim(p.prev(), "bracket")
	index := p.parseExpr()
	p.expect(lexer.RBRACKET)
	p.popDelim()
	return &IndexExpr{
		nodeBase: nodeBase{pos: object.Span().Merge(p.prev().Span)},
		Object:   object,
		Index:    index,
	}
}

func (p *Parser) parsePathExpr(left Expr) Expr {
	// left must be an Ident (or PathExpr already).
	var segments []string

	switch l := left.(type) {
	case *Ident:
		segments = append(segments, l.Name)
	case *PathExpr:
		segments = append(segments, l.Segments...)
	default:
		p.error(left.Span(), "left side of `::` must be an identifier")
		segments = append(segments, "<error>")
	}

	for p.match(lexer.DOUBLE_COLON) {
		segments = append(segments, p.expect(lexer.IDENT).Value)
	}

	return &PathExpr{
		nodeBase: nodeBase{pos: left.Span().Merge(p.prev().Span)},
		Segments: segments,
	}
}

func (p *Parser) parseAsExpr(left Expr) Expr {
	p.advance() // consume `as`
	typ := p.parseTypeExpr()
	return &AsExpr{
		nodeBase: nodeBase{pos: left.Span().Merge(typ.Span())},
		Expr:     left,
		Type:     typ,
	}
}

// ---------------------------------------------------------------------------
// Patterns
// ---------------------------------------------------------------------------

func (p *Parser) parsePattern() Pattern {
	return p.parseOrPattern()
}

func (p *Parser) parseOrPattern() Pattern {
	left := p.parseSinglePattern()

	if !p.check(lexer.BAR) {
		return left
	}

	alts := []Pattern{left}
	for p.match(lexer.BAR) {
		alts = append(alts, p.parseSinglePattern())
	}

	return &OrPat{
		nodeBase: nodeBase{pos: alts[0].Span().Merge(alts[len(alts)-1].Span())},
		Alts:     alts,
	}
}

func (p *Parser) parseSinglePattern() Pattern {
	tok := p.peek()

	switch tok.Type {
	case lexer.IDENT:
		// Could be a binding, variant constructor, or struct pattern.
		name := tok.Value
		p.advance()

		// Variant pattern: Name(pat, pat, ...)
		if p.check(lexer.LPAREN) {
			return p.parseVariantPatBody(name, tok.Span)
		}
		// Struct pattern: Name { field: pat, ... }
		if p.check(lexer.LBRACE) && p.isStructPatAhead() {
			return p.parseStructPatBody(name, tok.Span)
		}
		// Check if it's an uppercase identifier (conventionally a variant with no args)
		// or a binding.
		if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
			// Could be a zero-arg variant or a type binding. Treat as variant.
			return &VariantPat{
				nodeBase: nodeBase{pos: tok.Span},
				Name:     name,
			}
		}
		// Wildcard: _
		if name == "_" {
			return &WildcardPat{nodeBase: nodeBase{pos: tok.Span}}
		}
		return &BindingPat{
			nodeBase: nodeBase{pos: tok.Span},
			Name:     name,
		}

	case lexer.INT_LIT:
		p.advance()
		lit := &IntLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value}
		// Check for range pattern: 1..5 or 1..=5.
		if p.check(lexer.RANGE) || p.check(lexer.RANGE_INCLUSIVE) {
			return p.parseRangePat(lit, tok.Span)
		}
		return &LiteralPat{nodeBase: nodeBase{pos: tok.Span}, Value: lit}

	case lexer.FLOAT_LIT:
		p.advance()
		lit := &FloatLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value}
		return &LiteralPat{nodeBase: nodeBase{pos: tok.Span}, Value: lit}

	case lexer.STRING_LIT:
		p.advance()
		lit := &StringLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value}
		return &LiteralPat{nodeBase: nodeBase{pos: tok.Span}, Value: lit}

	case lexer.CHAR_LIT:
		p.advance()
		lit := &CharLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value}
		if p.check(lexer.RANGE) || p.check(lexer.RANGE_INCLUSIVE) {
			return p.parseRangePat(lit, tok.Span)
		}
		return &LiteralPat{nodeBase: nodeBase{pos: tok.Span}, Value: lit}

	case lexer.BOOL_LIT:
		p.advance()
		lit := &BoolLit{nodeBase: nodeBase{pos: tok.Span}, Value: tok.Value == "true"}
		return &LiteralPat{nodeBase: nodeBase{pos: tok.Span}, Value: lit}

	case lexer.MINUS:
		// Negative literal pattern: -42
		p.advance()
		numTok := p.peek()
		if numTok.Type == lexer.INT_LIT || numTok.Type == lexer.FLOAT_LIT {
			p.advance()
			var lit Expr
			if numTok.Type == lexer.INT_LIT {
				lit = &IntLit{nodeBase: nodeBase{pos: tok.Span.Merge(numTok.Span)}, Value: "-" + numTok.Value}
			} else {
				lit = &FloatLit{nodeBase: nodeBase{pos: tok.Span.Merge(numTok.Span)}, Value: "-" + numTok.Value}
			}
			return &LiteralPat{nodeBase: nodeBase{pos: tok.Span.Merge(numTok.Span)}, Value: lit}
		}
		p.errorExpected("numeric literal after `-` in pattern")
		return &WildcardPat{nodeBase: nodeBase{pos: tok.Span}}

	case lexer.LPAREN:
		// Tuple pattern or grouped pattern.
		return p.parseTupleOrGroupedPat()

	default:
		p.errorExpected("pattern")
		return &WildcardPat{nodeBase: nodeBase{pos: tok.Span}}
	}
}

func (p *Parser) parseVariantPatBody(name string, start diagnostic.Span) *VariantPat {
	p.expect(lexer.LPAREN)
	var fields []Pattern
	for !p.check(lexer.RPAREN) && !p.atEnd() {
		fields = append(fields, p.parsePattern())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.RPAREN)
	return &VariantPat{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Name:     name,
		Fields:   fields,
	}
}

func (p *Parser) isStructPatAhead() bool {
	if p.peekAt(1).Type == lexer.RBRACE {
		return true
	}
	if p.peekAt(1).Type == lexer.IDENT {
		// { IDENT : ... } — explicit field pattern
		if p.peekAt(2).Type == lexer.COLON {
			return true
		}
		// { IDENT , ... } or { IDENT } — shorthand field pattern
		if p.peekAt(2).Type == lexer.COMMA || p.peekAt(2).Type == lexer.RBRACE {
			return true
		}
	}
	return false
}

func (p *Parser) parseStructPatBody(name string, start diagnostic.Span) *StructPat {
	p.expect(lexer.LBRACE)
	p.pushDelim(p.prev(), "brace")
	var fields []*StructPatField
	for !p.check(lexer.RBRACE) && !p.atEnd() {
		fStart := p.peek().Span
		fname := p.expect(lexer.IDENT).Value
		var pat Pattern
		if p.match(lexer.COLON) {
			pat = p.parsePattern()
		}
		fields = append(fields, &StructPatField{
			nodeBase: nodeBase{pos: p.makeSpan(fStart)},
			Name:     fname,
			Pattern:  pat,
		})
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.RBRACE)
	p.popDelim()
	return &StructPat{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Name:     name,
		Fields:   fields,
	}
}

func (p *Parser) parseRangePat(start Expr, startSpan diagnostic.Span) *RangePat {
	inclusive := p.peek().Type == lexer.RANGE_INCLUSIVE
	p.advance() // consume .. or ..=

	// Parse end of range.
	endTok := p.peek()
	var end Expr
	switch endTok.Type {
	case lexer.INT_LIT:
		p.advance()
		end = &IntLit{nodeBase: nodeBase{pos: endTok.Span}, Value: endTok.Value}
	case lexer.CHAR_LIT:
		p.advance()
		end = &CharLit{nodeBase: nodeBase{pos: endTok.Span}, Value: endTok.Value}
	default:
		p.errorExpected("integer or char literal in range pattern")
		end = &IntLit{nodeBase: nodeBase{pos: endTok.Span}, Value: "0"}
	}

	return &RangePat{
		nodeBase:  nodeBase{pos: p.makeSpan(startSpan)},
		Start:     start,
		End:       end,
		Inclusive: inclusive,
	}
}

func (p *Parser) parseTupleOrGroupedPat() Pattern {
	start := p.expect(lexer.LPAREN).Span

	if p.check(lexer.RPAREN) {
		p.advance()
		return &TuplePat{nodeBase: nodeBase{pos: p.makeSpan(start)}}
	}

	first := p.parsePattern()

	if p.match(lexer.COMMA) {
		elems := []Pattern{first}
		for !p.check(lexer.RPAREN) && !p.atEnd() {
			elems = append(elems, p.parsePattern())
			if !p.match(lexer.COMMA) {
				break
			}
		}
		p.expect(lexer.RPAREN)
		return &TuplePat{
			nodeBase: nodeBase{pos: p.makeSpan(start)},
			Elems:    elems,
		}
	}

	p.expect(lexer.RPAREN)
	// Grouped pattern — just return the inner pattern.
	return first
}

// ---------------------------------------------------------------------------
// Type Expressions
// ---------------------------------------------------------------------------

func (p *Parser) parseTypeExpr() TypeExpr {
	return p.parseTypeExprInner()
}

func (p *Parser) parseTypeExprInner() TypeExpr {
	tok := p.peek()

	switch tok.Type {
	case lexer.IDENT:
		return p.parseNamedType()

	case lexer.SELF_TYPE:
		p.advance()
		return &SelfType{nodeBase: nodeBase{pos: tok.Span}}

	case lexer.LPAREN:
		return p.parseTupleOrFnType()

	case lexer.LBRACKET:
		return p.parseArrayOrSliceType()

	case lexer.FN:
		return p.parseFnTypeExpr()

	case lexer.CHANNEL: // [CLAUDE-FIX] Parse channel<T> in type positions
		return p.parseChannelType()

	default:
		p.errorExpected("type")
		return &NamedType{nodeBase: nodeBase{pos: tok.Span}, Name: "<error>"}
	}
}

func (p *Parser) parseNamedType() TypeExpr {
	tok := p.advance()
	name := tok.Value

	var genArgs []TypeExpr
	// Only parse `<` as generic args if the next token after `<` looks like a type.
	if p.check(lexer.LT) && p.looksLikeGenericArgs() {
		genArgs = p.parseGenericArgs()
	}

	return &NamedType{
		nodeBase: nodeBase{pos: p.makeSpan(tok.Span)},
		Name:     name,
		GenArgs:  genArgs,
	}
}

// [CLAUDE-FIX] Parse channel<T> in type positions
func (p *Parser) parseChannelType() TypeExpr {
	tok := p.advance() // consume 'channel'
	p.expect(lexer.LT)
	elem := p.parseTypeExpr()
	p.expect(lexer.GT)
	return &ChannelType{
		nodeBase: nodeBase{pos: p.makeSpan(tok.Span)},
		Elem:     elem,
	}
}

// looksLikeGenericArgs uses lookahead to decide if `<` starts generic args
// rather than a comparison. Heuristic: if the token after `<` is a type-like
// token (uppercase IDENT, keyword like Self, tuple start), treat as generic args.
func (p *Parser) looksLikeGenericArgs() bool {
	next := p.peekAt(1)
	switch next.Type {
	case lexer.IDENT:
		return true // could be a type name
	case lexer.SELF_TYPE:
		return true
	case lexer.LPAREN:
		return true
	case lexer.LBRACKET:
		return true
	case lexer.FN:
		return true
	case lexer.CHANNEL: // [CLAUDE-FIX]
		return true
	case lexer.GT:
		return true // empty: < >
	default:
		return false
	}
}

func (p *Parser) parseTupleOrFnType() TypeExpr {
	start := p.expect(lexer.LPAREN).Span

	// Empty tuple type: ()
	if p.check(lexer.RPAREN) {
		p.advance()
		// Check for function type: () -> ReturnType
		if p.match(lexer.ARROW) {
			ret := p.parseTypeExpr()
			return &FnType{
				nodeBase: nodeBase{pos: p.makeSpan(start)},
				Return:   ret,
			}
		}
		return &TupleType{nodeBase: nodeBase{pos: p.makeSpan(start)}}
	}

	first := p.parseTypeExpr()

	if p.match(lexer.COMMA) {
		// Tuple type: (A, B, ...)
		elems := []TypeExpr{first}
		for !p.check(lexer.RPAREN) && !p.atEnd() {
			elems = append(elems, p.parseTypeExpr())
			if !p.match(lexer.COMMA) {
				break
			}
		}
		p.expect(lexer.RPAREN)

		// Check for function type: (A, B) -> Ret
		if p.match(lexer.ARROW) {
			ret := p.parseTypeExpr()
			return &FnType{
				nodeBase: nodeBase{pos: p.makeSpan(start)},
				Params:   elems,
				Return:   ret,
			}
		}

		return &TupleType{
			nodeBase: nodeBase{pos: p.makeSpan(start)},
			Elems:    elems,
		}
	}

	p.expect(lexer.RPAREN)

	// Check for function type: (A) -> Ret
	if p.match(lexer.ARROW) {
		ret := p.parseTypeExpr()
		return &FnType{
			nodeBase: nodeBase{pos: p.makeSpan(start)},
			Params:   []TypeExpr{first},
			Return:   ret,
		}
	}

	// Single-element parenthesized type — return the inner type.
	return first
}

func (p *Parser) parseArrayOrSliceType() TypeExpr {
	start := p.expect(lexer.LBRACKET).Span
	elem := p.parseTypeExpr()
	p.expect(lexer.RBRACKET)
	return &ArrayType{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Elem:     elem,
	}
}

func (p *Parser) parseFnTypeExpr() TypeExpr {
	start := p.expect(lexer.FN).Span
	p.expect(lexer.LPAREN)
	var params []TypeExpr
	for !p.check(lexer.RPAREN) && !p.atEnd() {
		params = append(params, p.parseTypeExpr())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.RPAREN)

	var ret TypeExpr
	if p.match(lexer.ARROW) {
		ret = p.parseTypeExpr()
	}

	return &FnType{
		nodeBase: nodeBase{pos: p.makeSpan(start)},
		Params:   params,
		Return:   ret,
	}
}
