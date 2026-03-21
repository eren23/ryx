package resolver

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/diagnostic"
	"github.com/ryx-lang/ryx/pkg/parser"
)

// ResolveResult contains the output of name resolution.
type ResolveResult struct {
	RootScope   *Scope
	Diagnostics []diagnostic.Diagnostic

	// Resolutions maps identifier spans to their resolved symbols.
	Resolutions map[diagnostic.Span]*Symbol
	// TypeDefs maps type names to their definitions.
	TypeDefs map[string]*parser.TypeDef
	// StructDefs maps struct names to their definitions.
	StructDefs map[string]*parser.StructDef
	// TraitDefs maps trait names to their definitions.
	TraitDefs map[string]*parser.TraitDef
	// ImplBlocks stores all impl blocks.
	ImplBlocks []*parser.ImplBlock
}

// Resolver performs two-pass name resolution on a parsed AST.
type Resolver struct {
	collector *diagnostic.Collector
	registry  *diagnostic.SourceRegistry

	root        *Scope
	current     *Scope
	resolutions map[diagnostic.Span]*Symbol

	// Top-level definitions collected during hoisting.
	typeDefs   map[string]*parser.TypeDef
	structDefs map[string]*parser.StructDef
	traitDefs  map[string]*parser.TraitDef
	implBlocks []*parser.ImplBlock

	// childIndex tracks the next child scope to enter during pass 2.
	childIdx map[*Scope]int
}

// Resolve performs name resolution on the given program AST.
func Resolve(program *parser.Program, registry *diagnostic.SourceRegistry) *ResolveResult {
	collector := diagnostic.NewCollector(registry, 50, 50)
	r := &Resolver{
		collector:   collector,
		registry:    registry,
		resolutions: make(map[diagnostic.Span]*Symbol),
		typeDefs:    make(map[string]*parser.TypeDef),
		structDefs:  make(map[string]*parser.StructDef),
		traitDefs:   make(map[string]*parser.TraitDef),
		childIdx:    make(map[*Scope]int),
	}

	// [CLAUDE-FIX] Create prelude scope so builtins can be shadowed by user code
	prelude := NewScope(ModuleScope, nil)
	r.root = NewScope(ModuleScope, prelude)
	r.current = r.root

	// Register built-in functions and types so user code can reference them.
	r.registerBuiltins()

	// Pass 1: Build scope tree — hoist functions/types first, then walk.
	r.pass1Hoist(program)
	r.pass1Walk(program)

	// Pass 2: Resolve identifiers and check semantics.
	r.current = r.root
	r.pass2Walk(program)

	// Post-pass: detect unused symbols.
	r.checkUnused(r.root)

	return &ResolveResult{
		RootScope:   r.root,
		Diagnostics: collector.Diagnostics(),
		Resolutions: r.resolutions,
		TypeDefs:    r.typeDefs,
		StructDefs:  r.structDefs,
		TraitDefs:   r.traitDefs,
		ImplBlocks:  r.implBlocks,
	}
}

// ---------------------------------------------------------------------------
// Built-in registration
// ---------------------------------------------------------------------------

// builtinFunctions lists all built-in function names that should be available
// in every Ryx program without explicit imports.
var builtinFunctions = []string{
	// I/O
	"print", "println", "read_line",
	// Type conversions
	"int_to_float", "float_to_int", "int_to_string", "float_to_string",
	"parse_int", "parse_float", "bool_to_string",
	"string_to_int", "string_to_float", "char_to_int", "int_to_char",
	// Assertions
	"assert", "assert_eq", "panic",
	// String operations
	"string_len", "string_chars", "string_contains", "string_split",
	"string_trim", "string_replace", "string_starts_with", "string_ends_with",
	"string_to_upper", "string_to_lower", "string_repeat", "string_reverse",
	"string_slice", "string_index_of", "string_pad_left", "string_pad_right",
	"string_bytes", "string_join", "char_to_string",
	// Array operations
	"array_len", "array_push", "array_pop", "array_slice", "array_reverse",
	"array_contains", "array_map", "array_filter", "array_fold",
	"array_sort", "array_flatten", "array_zip", "array_enumerate",
	"array_flat_map", "array_find", "array_any", "array_all",
	"array_sum", "array_min", "array_max", "array_take", "array_drop",
	"array_chunk", "array_unique", "array_join",
	// Math
	"abs", "min", "max", "sqrt", "pow", "floor", "ceil", "round",
	"sin", "cos", "tan", "asin", "acos", "atan", "atan2",
	"log", "log2", "log10", "exp", "pi", "e", "gcd", "lcm", "clamp",
	"random_int", "random_float",
	// Time/Random
	"time_now_ms", "sleep_ms", "random_seed", "random_shuffle", "random_choice",
	// File I/O
	"read_file", "write_file", "file_exists", "dir_list", "dir_create",
	"path_join", "path_dirname", "path_basename", "path_extension", "file_size",
	// Map operations
	"map_new", "map_get", "map_set", "map_delete", "map_contains",
	"map_len", "map_keys", "map_values", "map_entries",
	"map_merge", "map_filter", "map_map",
	// Hash / comparison (trait methods)
	"eq", "neq", "compare", "to_string", "default", "clone", "hash",
	// Graphics — window
	"gfx_init", "gfx_run", "gfx_quit", "gfx_set_title",
	// Graphics — draw
	"gfx_clear", "gfx_set_color", "gfx_pixel",
	"gfx_line", "gfx_rect", "gfx_fill_rect",
	"gfx_circle", "gfx_fill_circle", "gfx_text",
	// Graphics — colors
	"gfx_rgb", "gfx_rgba",
	"COLOR_BLACK", "COLOR_WHITE", "COLOR_RED",
	"COLOR_GREEN", "COLOR_BLUE", "COLOR_YELLOW",
	// Graphics — input
	"gfx_key_pressed", "gfx_key_just_pressed",
	"gfx_mouse_x", "gfx_mouse_y", "gfx_mouse_pressed",
	"KEY_UP", "KEY_DOWN", "KEY_LEFT", "KEY_RIGHT",
	"KEY_SPACE", "KEY_ESCAPE", "KEY_ENTER",
	"KEY_W", "KEY_A", "KEY_S", "KEY_D",
	"KEY_EQUAL", "KEY_MINUS",
	// Graphics — bridge
	"gfx_width", "gfx_height", "gfx_fps", "gfx_delta_time",
	// Graphics — image
	"gfx_load_image", "gfx_draw_image", "gfx_draw_image_scaled",
	"gfx_image_width", "gfx_image_height",
}

// builtinTypes lists built-in type names.
var builtinTypes = []string{
	"Int", "Float", "Bool", "Char", "String", "Unit",
	"Option", "Result",
}

// registerBuiltins pre-populates the root scope with built-in function
// and type symbols so that user code can reference them without imports.
func (r *Resolver) registerBuiltins() {
	prelude := r.root.Parent // [CLAUDE-FIX] Register builtins in prelude scope so user code can shadow them
	for _, name := range builtinFunctions {
		sym := &Symbol{
			Name:      name,
			Kind:      FunctionSymbol,
			Public:    true,
			UsedCount: 1, // Prevent "unused" warnings.
		}
		prelude.Define(sym)
	}
	for _, name := range builtinTypes {
		sym := &Symbol{
			Name:      name,
			Kind:      TypeSymbol,
			Public:    true,
			UsedCount: 1,
		}
		prelude.Define(sym)
	}
}

// ---------------------------------------------------------------------------
// Pass 1: Hoisting — register top-level functions and types first
// ---------------------------------------------------------------------------

func (r *Resolver) pass1Hoist(program *parser.Program) {
	for _, item := range program.Items {
		switch it := item.(type) {
		case *parser.FnDef:
			r.defineSymbol(it.Name, FunctionSymbol, it.Span(), it.Public, false)
		case *parser.TypeDef:
			r.typeDefs[it.Name] = it
			sym := r.defineSymbol(it.Name, TypeSymbol, it.Span(), it.Public, false)
			if sym != nil {
				var variantNames []string
				for _, v := range it.Variants {
					variantNames = append(variantNames, v.Name)
				}
				sym.Variants = variantNames
			}
			// Register variant constructors at module scope.
			for i, v := range it.Variants {
				vs := r.defineSymbol(v.Name, VariantSymbol, v.Span(), it.Public, false)
				if vs != nil {
					vs.ParentType = it.Name
					vs.VariantIndex = i
					vs.FieldCount = len(v.Fields)
				}
			}
		case *parser.StructDef:
			r.structDefs[it.Name] = it
			r.defineSymbol(it.Name, TypeSymbol, it.Span(), it.Public, false)
		case *parser.TraitDef:
			r.traitDefs[it.Name] = it
			r.defineSymbol(it.Name, TraitSymbol, it.Span(), it.Public, false)
		case *parser.ImportDecl:
			name := it.Alias
			if name == "" && len(it.Path) > 0 {
				name = it.Path[len(it.Path)-1]
			}
			r.defineSymbol(name, ImportSymbol, it.Span(), false, false)
		}
	}
}

// ---------------------------------------------------------------------------
// Pass 1: Walk — build scope tree for function bodies, impl blocks, etc.
// ---------------------------------------------------------------------------

func (r *Resolver) pass1Walk(program *parser.Program) {
	for _, item := range program.Items {
		r.pass1Item(item)
	}
}

func (r *Resolver) pass1Item(item parser.Item) {
	switch it := item.(type) {
	case *parser.FnDef:
		r.pass1FnDef(it)
	case *parser.ImplBlock:
		r.implBlocks = append(r.implBlocks, it)
		r.pass1ImplBlock(it)
	case *parser.TraitDef:
		r.pass1TraitDef(it)
	case *parser.TypeDef:
		if it.GenParams != nil {
			s := r.pushScope(BlockScope)
			_ = s
			r.defineGenericParams(it.GenParams)
			r.popScope()
		}
	case *parser.StructDef:
		if it.GenParams != nil {
			s := r.pushScope(BlockScope)
			_ = s
			r.defineGenericParams(it.GenParams)
			r.popScope()
		}
	}
}

func (r *Resolver) pass1FnDef(fn *parser.FnDef) {
	if fn.Body == nil {
		return
	}
	fnScope := r.pushScope(FunctionScope)
	fnScope.FnName = fn.Name

	// Generic type params scoped to the function.
	r.defineGenericParams(fn.GenParams)

	// Parameters.
	for _, p := range fn.Params {
		r.defineSymbol(p.Name, VariableSymbol, p.Span(), false, false)
	}

	r.pass1Block(fn.Body)
	r.popScope()
}

func (r *Resolver) pass1ImplBlock(impl *parser.ImplBlock) {
	implScope := r.pushScope(ImplScope)
	if nt, ok := impl.TargetType.(*parser.NamedType); ok {
		implScope.ImplTarget = nt.Name
	}

	r.defineGenericParams(impl.GenParams)

	for _, method := range impl.Methods {
		r.defineSymbol(method.Name, FunctionSymbol, method.Span(), method.Public, false)
		r.pass1FnDef(method)
	}
	r.popScope()
}

func (r *Resolver) pass1TraitDef(trait *parser.TraitDef) {
	traitScope := r.pushScope(TraitScope)
	traitScope.TraitName = trait.Name

	r.defineGenericParams(trait.GenParams)

	for _, method := range trait.Methods {
		r.defineSymbol(method.Name, FunctionSymbol, method.Span(), true, false)
		if method.Body != nil {
			r.pass1FnDef(method)
		}
	}
	r.popScope()
}

func (r *Resolver) pass1Block(block *parser.Block) {
	if block == nil {
		return
	}
	blockScope := r.pushScope(BlockScope)
	_ = blockScope

	for _, stmt := range block.Stmts {
		r.pass1Stmt(stmt)
	}
	if block.TrailingExpr != nil {
		r.pass1Expr(block.TrailingExpr)
	}
	r.popScope()
}

func (r *Resolver) pass1Stmt(stmt parser.Stmt) {
	switch s := stmt.(type) {
	case *parser.LetStmt:
		r.defineSymbol(s.Name, VariableSymbol, s.Span(), false, s.Mutable)
		if s.Value != nil {
			r.pass1Expr(s.Value)
		}
	case *parser.ExprStmt:
		r.pass1Expr(s.Expr)
	case *parser.ReturnStmt:
		if s.Value != nil {
			r.pass1Expr(s.Value)
		}
	}
}

func (r *Resolver) pass1Expr(expr parser.Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *parser.Block:
		r.pass1Block(e)
	case *parser.IfExpr:
		r.pass1Expr(e.Cond)
		r.pass1Block(e.Then)
		if e.Else != nil {
			r.pass1Expr(e.Else)
		}
	case *parser.MatchExpr:
		r.pass1Expr(e.Scrutinee)
		for _, arm := range e.Arms {
			armScope := r.pushScope(BlockScope)
			_ = armScope
			r.pass1Pattern(arm.Pattern)
			if arm.Guard != nil {
				r.pass1Expr(arm.Guard)
			}
			r.pass1Expr(arm.Body)
			r.popScope()
		}
	case *parser.ForExpr:
		loopScope := r.pushScope(BlockScope)
		loopScope.InLoop = true
		r.defineSymbol(e.Binding, VariableSymbol, e.Span(), false, false)
		r.pass1Expr(e.Iter)
		r.pass1BlockInLoop(e.Body)
		r.popScope()
	case *parser.WhileExpr:
		r.pass1Expr(e.Cond)
		loopScope := r.pushScope(BlockScope)
		loopScope.InLoop = true
		r.pass1BlockInLoop(e.Body)
		r.popScope()
	case *parser.LoopExpr:
		loopScope := r.pushScope(BlockScope)
		loopScope.InLoop = true
		r.pass1BlockInLoop(e.Body)
		r.popScope()
	case *parser.LambdaExpr:
		fnScope := r.pushScope(FunctionScope)
		fnScope.FnName = "<lambda>"
		for _, p := range e.Params {
			r.defineSymbol(p.Name, VariableSymbol, p.Span(), false, false)
		}
		r.pass1Expr(e.Body)
		r.popScope()
	case *parser.BinaryExpr:
		r.pass1Expr(e.Left)
		r.pass1Expr(e.Right)
	case *parser.UnaryExpr:
		r.pass1Expr(e.Operand)
	case *parser.CallExpr:
		r.pass1Expr(e.Func)
		for _, arg := range e.Args {
			r.pass1Expr(arg)
		}
	case *parser.FieldExpr:
		r.pass1Expr(e.Object)
	case *parser.IndexExpr:
		r.pass1Expr(e.Object)
		r.pass1Expr(e.Index)
	case *parser.ArrayLit:
		for _, elem := range e.Elems {
			r.pass1Expr(elem)
		}
	case *parser.TupleLit:
		for _, elem := range e.Elems {
			r.pass1Expr(elem)
		}
	case *parser.StructLit:
		for _, f := range e.Fields {
			r.pass1Expr(f.Value)
		}
	case *parser.GroupExpr:
		r.pass1Expr(e.Inner)
	case *parser.ReturnExpr:
		if e.Value != nil {
			r.pass1Expr(e.Value)
		}
	case *parser.AsExpr:
		r.pass1Expr(e.Expr)
	case *parser.SpawnExpr:
		r.pass1Block(e.Body)
	}
}

func (r *Resolver) pass1BlockInLoop(block *parser.Block) {
	if block == nil {
		return
	}
	// When entering a loop body, we don't push a new scope here since
	// the caller already pushed a loop scope. We just walk the statements.
	for _, stmt := range block.Stmts {
		r.pass1Stmt(stmt)
	}
	if block.TrailingExpr != nil {
		r.pass1Expr(block.TrailingExpr)
	}
}

func (r *Resolver) pass1Pattern(pat parser.Pattern) {
	if pat == nil {
		return
	}
	switch p := pat.(type) {
	case *parser.BindingPat:
		r.defineSymbol(p.Name, VariableSymbol, p.Span(), false, false)
	case *parser.TuplePat:
		for _, elem := range p.Elems {
			r.pass1Pattern(elem)
		}
	case *parser.VariantPat:
		for _, field := range p.Fields {
			r.pass1Pattern(field)
		}
	case *parser.StructPat:
		for _, f := range p.Fields {
			if f.Pattern != nil {
				r.pass1Pattern(f.Pattern)
			} else {
				// Shorthand: field name is also a binding.
				r.defineSymbol(f.Name, VariableSymbol, f.Span(), false, false)
			}
		}
	case *parser.OrPat:
		// All alternatives must bind the same names. Just walk the first.
		if len(p.Alts) > 0 {
			r.pass1Pattern(p.Alts[0])
		}
	}
}

// ---------------------------------------------------------------------------
// Pass 2: Resolve identifiers, check semantics
// ---------------------------------------------------------------------------

func (r *Resolver) pass2Walk(program *parser.Program) {
	for _, item := range program.Items {
		r.pass2Item(item)
	}
}

func (r *Resolver) pass2Item(item parser.Item) {
	switch it := item.(type) {
	case *parser.FnDef:
		r.pass2FnDef(it)
	case *parser.ImplBlock:
		r.pass2ImplBlock(it)
	case *parser.TraitDef:
		r.pass2TraitDef(it)
	case *parser.TypeDef:
		r.pass2TypeDef(it)
	case *parser.StructDef:
		r.pass2StructDef(it)
	}
}

func (r *Resolver) pass2FnDef(fn *parser.FnDef) {
	if fn.Body == nil {
		return
	}
	r.enterChildScope()

	// Resolve parameter types.
	for _, p := range fn.Params {
		r.resolveTypeExpr(p.Type)
	}
	r.resolveTypeExpr(fn.ReturnType)

	r.pass2Block(fn.Body)
	r.exitChildScope()
}

func (r *Resolver) pass2ImplBlock(impl *parser.ImplBlock) {
	r.enterChildScope()

	r.resolveTypeExpr(impl.TargetType)
	for _, tg := range impl.TraitGens {
		r.resolveTypeExpr(tg)
	}

	// Resolve trait name.
	if impl.TraitName != "" {
		sym := r.current.Lookup(impl.TraitName)
		if sym == nil {
			r.error("E010", fmt.Sprintf("undefined trait `%s`", impl.TraitName), impl.Span())
		} else if sym.Kind != TraitSymbol {
			r.error("E011", fmt.Sprintf("`%s` is not a trait", impl.TraitName), impl.Span())
		} else {
			sym.UsedCount++
		}
	}

	for _, method := range impl.Methods {
		r.pass2FnDef(method)
	}
	r.exitChildScope()
}

func (r *Resolver) pass2TraitDef(trait *parser.TraitDef) {
	r.enterChildScope()
	for _, method := range trait.Methods {
		if method.Body != nil {
			r.pass2FnDef(method)
		} else {
			// Just resolve the signature types.
			for _, p := range method.Params {
				r.resolveTypeExpr(p.Type)
			}
			r.resolveTypeExpr(method.ReturnType)
		}
	}
	r.exitChildScope()
}

func (r *Resolver) pass2TypeDef(td *parser.TypeDef) {
	if td.GenParams != nil {
		r.enterChildScope()
		for _, v := range td.Variants {
			for _, ft := range v.Fields {
				r.resolveTypeExpr(ft)
			}
		}
		r.exitChildScope()
	} else {
		for _, v := range td.Variants {
			for _, ft := range v.Fields {
				r.resolveTypeExpr(ft)
			}
		}
	}
}

func (r *Resolver) pass2StructDef(sd *parser.StructDef) {
	if sd.GenParams != nil {
		r.enterChildScope()
		for _, f := range sd.Fields {
			r.resolveTypeExpr(f.Type)
		}
		r.exitChildScope()
	} else {
		for _, f := range sd.Fields {
			r.resolveTypeExpr(f.Type)
		}
	}
}

func (r *Resolver) pass2Block(block *parser.Block) {
	if block == nil {
		return
	}
	r.enterChildScope()
	for _, stmt := range block.Stmts {
		r.pass2Stmt(stmt)
	}
	if block.TrailingExpr != nil {
		r.pass2Expr(block.TrailingExpr)
	}
	r.exitChildScope()
}

func (r *Resolver) pass2Stmt(stmt parser.Stmt) {
	switch s := stmt.(type) {
	case *parser.LetStmt:
		// Resolve value first (before the binding is visible).
		if s.Value != nil {
			r.pass2Expr(s.Value)
		}
		r.resolveTypeExpr(s.Type)
	case *parser.ExprStmt:
		r.pass2Expr(s.Expr)
	case *parser.ReturnStmt:
		if s.Value != nil {
			r.pass2Expr(s.Value)
		}
	}
}

func (r *Resolver) pass2Expr(expr parser.Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *parser.Ident:
		r.resolveIdent(e)
	case *parser.SelfExpr:
		if !r.current.IsInsideImpl() {
			r.error("E020", "`self` is only valid inside `impl` blocks", e.Span())
		}
	case *parser.PathExpr:
		r.resolvePathExpr(e)
	case *parser.Block:
		r.pass2Block(e)
	case *parser.IfExpr:
		r.pass2Expr(e.Cond)
		r.pass2Block(e.Then)
		if e.Else != nil {
			r.pass2Expr(e.Else)
		}
	case *parser.MatchExpr:
		r.pass2Expr(e.Scrutinee)
		for _, arm := range e.Arms {
			r.enterChildScope()
			r.pass2Pattern(arm.Pattern)
			if arm.Guard != nil {
				r.pass2Expr(arm.Guard)
			}
			r.pass2Expr(arm.Body)
			r.exitChildScope()
		}
		r.checkExhaustiveness(e)
	case *parser.ForExpr:
		r.enterChildScope()
		r.pass2Expr(e.Iter)
		r.pass2BlockInScope(e.Body)
		r.exitChildScope()
	case *parser.WhileExpr:
		r.pass2Expr(e.Cond)
		r.enterChildScope()
		r.pass2BlockInScope(e.Body)
		r.exitChildScope()
	case *parser.LoopExpr:
		r.enterChildScope()
		r.pass2BlockInScope(e.Body)
		r.exitChildScope()
	case *parser.LambdaExpr:
		r.enterChildScope()
		for _, p := range e.Params {
			r.resolveTypeExpr(p.Type)
		}
		r.resolveTypeExpr(e.ReturnType)
		r.pass2Expr(e.Body)
		r.exitChildScope()
	case *parser.BinaryExpr:
		r.pass2Expr(e.Left)
		r.pass2Expr(e.Right)
	case *parser.UnaryExpr:
		r.pass2Expr(e.Operand)
	case *parser.CallExpr:
		r.pass2Expr(e.Func)
		for _, arg := range e.Args {
			r.pass2Expr(arg)
		}
	case *parser.FieldExpr:
		r.pass2Expr(e.Object)
	case *parser.IndexExpr:
		r.pass2Expr(e.Object)
		r.pass2Expr(e.Index)
	case *parser.ArrayLit:
		for _, elem := range e.Elems {
			r.pass2Expr(elem)
		}
	case *parser.TupleLit:
		for _, elem := range e.Elems {
			r.pass2Expr(elem)
		}
	case *parser.StructLit:
		// Resolve struct type name.
		sym := r.current.Lookup(e.Name)
		if sym != nil {
			sym.UsedCount++
			r.resolutions[e.Span()] = sym
		} else {
			r.error("E005", fmt.Sprintf("undefined struct `%s`", e.Name), e.Span())
		}
		for _, f := range e.Fields {
			r.pass2Expr(f.Value)
		}
	case *parser.GroupExpr:
		r.pass2Expr(e.Inner)
	case *parser.BreakExpr:
		if !r.current.InLoop {
			r.error("E030", "`break` outside of loop", e.Span())
		}
	case *parser.ContinueExpr:
		if !r.current.InLoop {
			r.error("E031", "`continue` outside of loop", e.Span())
		}
	case *parser.ReturnExpr:
		if r.current.EnclosingFunction() == nil {
			r.error("E032", "`return` outside of function", e.Span())
		}
		if e.Value != nil {
			r.pass2Expr(e.Value)
		}
	case *parser.AsExpr:
		r.pass2Expr(e.Expr)
		r.resolveTypeExpr(e.Type)
	case *parser.SpawnExpr:
		r.pass2Block(e.Body)
	case *parser.ChannelExpr:
		r.resolveTypeExpr(e.ElemType)
		if e.Size != nil {
			r.pass2Expr(e.Size)
		}
	}
}

func (r *Resolver) pass2BlockInScope(block *parser.Block) {
	if block == nil {
		return
	}
	for _, stmt := range block.Stmts {
		r.pass2Stmt(stmt)
	}
	if block.TrailingExpr != nil {
		r.pass2Expr(block.TrailingExpr)
	}
}

func (r *Resolver) pass2Pattern(pat parser.Pattern) {
	if pat == nil {
		return
	}
	switch p := pat.(type) {
	case *parser.VariantPat:
		// Resolve variant constructor name.
		sym := r.current.Lookup(p.Name)
		if sym != nil {
			if sym.Kind != VariantSymbol {
				r.error("E012", fmt.Sprintf("`%s` is not a variant constructor", p.Name), p.Span())
			} else {
				sym.UsedCount++
				r.resolutions[p.Span()] = sym
			}
		} else {
			r.error("E005", fmt.Sprintf("undefined variant `%s`", p.Name), p.Span())
		}
		for _, field := range p.Fields {
			r.pass2Pattern(field)
		}
	case *parser.StructPat:
		sym := r.current.Lookup(p.Name)
		if sym != nil {
			sym.UsedCount++
			r.resolutions[p.Span()] = sym
		} else {
			r.error("E005", fmt.Sprintf("undefined struct `%s`", p.Name), p.Span())
		}
		for _, f := range p.Fields {
			if f.Pattern != nil {
				r.pass2Pattern(f.Pattern)
			}
		}
	case *parser.TuplePat:
		for _, elem := range p.Elems {
			r.pass2Pattern(elem)
		}
	case *parser.OrPat:
		for _, alt := range p.Alts {
			r.pass2Pattern(alt)
		}
	case *parser.LiteralPat:
		r.pass2Expr(p.Value)
	}
}

// ---------------------------------------------------------------------------
// Resolution helpers
// ---------------------------------------------------------------------------

func (r *Resolver) resolveIdent(id *parser.Ident) {
	sym := r.current.Lookup(id.Name)
	if sym == nil {
		hint := diagnostic.SuggestHint(id.Name, r.current.AllSymbolNames())
		if hint != "" {
			r.errorWithHint("E005", fmt.Sprintf("undefined variable `%s`", id.Name), id.Span(), hint)
		} else {
			r.error("E005", fmt.Sprintf("undefined variable `%s`", id.Name), id.Span())
		}
		return
	}
	sym.UsedCount++
	r.resolutions[id.Span()] = sym
}

func (r *Resolver) resolvePathExpr(pe *parser.PathExpr) {
	if len(pe.Segments) == 0 {
		return
	}
	// Resolve the first segment.
	first := pe.Segments[0]
	sym := r.current.Lookup(first)
	if sym != nil {
		sym.UsedCount++
		r.resolutions[pe.Span()] = sym
	} else {
		r.error("E005", fmt.Sprintf("undefined name `%s`", first), pe.Span())
	}
}

func (r *Resolver) resolveTypeExpr(te parser.TypeExpr) {
	if te == nil {
		return
	}
	switch t := te.(type) {
	case *parser.NamedType:
		r.resolveTypeName(t)
		for _, ga := range t.GenArgs {
			r.resolveTypeExpr(ga)
		}
	case *parser.FnType:
		for _, p := range t.Params {
			r.resolveTypeExpr(p)
		}
		r.resolveTypeExpr(t.Return)
	case *parser.TupleType:
		for _, elem := range t.Elems {
			r.resolveTypeExpr(elem)
		}
	case *parser.ArrayType:
		r.resolveTypeExpr(t.Elem)
	case *parser.SliceType:
		r.resolveTypeExpr(t.Elem)
	case *parser.ReferenceType:
		r.resolveTypeExpr(t.Elem)
	case *parser.SelfType:
		if !r.current.IsInsideTraitOrImpl() {
			r.error("E021", "`Self` type is only valid inside `trait` or `impl` blocks", t.Span())
		}
	}
}

// resolveTypeName resolves a named type (e.g., Int, Vec, Option).
// Built-in types are always valid. User types must be defined.
func (r *Resolver) resolveTypeName(nt *parser.NamedType) {
	// Built-in primitive types.
	switch nt.Name {
	case "Int", "Float", "Bool", "Char", "String", "Unit":
		return
	}

	sym := r.current.Lookup(nt.Name)
	if sym == nil {
		r.error("E006", fmt.Sprintf("undefined type `%s`", nt.Name), nt.Span())
		return
	}
	if sym.Kind != TypeSymbol && sym.Kind != TraitSymbol && sym.Kind != TypeParamSymbol {
		r.error("E007", fmt.Sprintf("`%s` is not a type", nt.Name), nt.Span())
		return
	}
	sym.UsedCount++
	r.resolutions[nt.Span()] = sym
}

// ---------------------------------------------------------------------------
// Exhaustiveness checking (delegates to exhaustiveness.go)
// ---------------------------------------------------------------------------

func (r *Resolver) checkExhaustiveness(me *parser.MatchExpr) {
	result := CheckExhaustiveness(me, r.typeDefs)
	for _, d := range result.Diagnostics {
		r.collector.Add(d)
	}
}

// ---------------------------------------------------------------------------
// Unused detection
// ---------------------------------------------------------------------------

func (r *Resolver) checkUnused(scope *Scope) {
	for _, sym := range scope.Symbols {
		if sym.UsedCount > 0 {
			continue
		}
		switch sym.Kind {
		case VariableSymbol:
			if sym.Name == "_" || sym.Name == "self" {
				continue
			}
			if sym.Mutable {
				r.collector.WarningWithHint("W002",
					fmt.Sprintf("variable `%s` does not need to be mutable", sym.Name),
					sym.Span, "remove `mut`")
			}
			r.collector.Warning("W001",
				fmt.Sprintf("unused variable `%s`", sym.Name),
				sym.Span)
		case ImportSymbol:
			r.collector.Warning("W003",
				fmt.Sprintf("unused import `%s`", sym.Name),
				sym.Span)
		case FunctionSymbol:
			// Only warn for non-public, non-main functions at module level.
			if !sym.Public && sym.Name != "main" && scope.Kind == ModuleScope {
				r.collector.Warning("W004",
					fmt.Sprintf("unused function `%s`", sym.Name),
					sym.Span)
			}
		}
	}
	for _, child := range scope.Children {
		r.checkUnused(child)
	}
}

// ---------------------------------------------------------------------------
// Scope management helpers
// ---------------------------------------------------------------------------

func (r *Resolver) pushScope(kind ScopeKind) *Scope {
	s := NewScope(kind, r.current)
	r.current = s
	return s
}

func (r *Resolver) popScope() {
	if r.current.Parent != nil {
		r.current = r.current.Parent
	}
}

// enterChildScope advances to the next child scope for pass 2.
// Pass 2 walks the same scope tree built by pass 1.
func (r *Resolver) enterChildScope() {
	idx := r.childIdx[r.current]
	if idx < len(r.current.Children) {
		r.current = r.current.Children[idx]
		r.childIdx[r.current.Parent] = idx + 1
	}
}

func (r *Resolver) exitChildScope() {
	if r.current.Parent != nil {
		r.current = r.current.Parent
	}
}

func (r *Resolver) defineSymbol(name string, kind SymbolKind, span diagnostic.Span, public, mutable bool) *Symbol {
	sym := &Symbol{
		Name:    name,
		Kind:    kind,
		Span:    span,
		Public:  public,
		Mutable: mutable,
	}

	existing := r.current.Define(sym)
	if existing != nil {
		// Same-scope shadowing is an error.
		r.collector.Error("E001",
			fmt.Sprintf("`%s` is already defined in this scope", name),
			span)
		return existing
	}

	// Check for nested shadowing (warning).
	if r.current.Parent != nil {
		outer := r.current.Parent.Lookup(name)
		if outer != nil && name != "_" {
			r.collector.Warning("W010",
				fmt.Sprintf("`%s` shadows a previous definition", name),
				span)
		}
	}

	return sym
}

func (r *Resolver) defineGenericParams(gp *parser.GenericParams) {
	if gp == nil {
		return
	}
	for _, p := range gp.Params {
		r.defineSymbol(p.Name, TypeParamSymbol, p.Span(), false, false)
	}
}

// ---------------------------------------------------------------------------
// Diagnostic helpers
// ---------------------------------------------------------------------------

func (r *Resolver) error(code, message string, span diagnostic.Span) {
	r.collector.Error(code, message, span)
}

func (r *Resolver) errorWithHint(code, message string, span diagnostic.Span, hint string) {
	r.collector.ErrorWithHint(code, message, span, hint)
}
