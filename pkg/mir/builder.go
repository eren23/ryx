package mir

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ryx-lang/ryx/pkg/hir"
	"github.com/ryx-lang/ryx/pkg/types"
)

// ---------------------------------------------------------------------------
// Builder — HIR to MIR SSA construction
//
// Uses the Braun et al. algorithm: variables are tracked per-block,
// and phi nodes are lazily inserted when a variable is read in a block
// where it wasn't defined.
// ---------------------------------------------------------------------------

// Build lowers an HIR program to MIR in SSA form.
func Build(prog *hir.Program) *Program {
	// Compile match expressions into decision trees before MIR lowering.
	hir.CompileMatches(prog)

	p := &Program{}

	// Copy struct/enum definitions.
	for _, s := range prog.Structs {
		sd := &StructDef{Name: s.Name}
		for _, f := range s.Fields {
			sd.Fields = append(sd.Fields, FieldDef{Name: f.Name, Type: f.Type})
		}
		p.Structs = append(p.Structs, sd)
	}
	for _, e := range prog.Enums {
		ed := &EnumDef{Name: e.Name}
		for _, v := range e.Variants {
			ed.Variants = append(ed.Variants, VariantDef{Name: v.Name, Fields: v.Fields})
		}
		p.Enums = append(p.Enums, ed)
	}

	// Lower each function, collecting lambda functions along the way.
	var lambdaFns []*Function
	for _, fn := range prog.Functions {
		mfn := buildFunction(fn, prog, &lambdaFns)
		p.Functions = append(p.Functions, mfn)
	}
	// Append lambda functions after all top-level functions.
	p.Functions = append(p.Functions, lambdaFns...)

	return p
}

// builder holds per-function lowering state.
type builder struct {
	fn       *Function
	prog     *hir.Program
	curBlock BlockID

	// SSA variable tracking: maps source variable name to its current
	// SSA local in each block (Braun et al. algorithm).
	currentDef map[string]map[BlockID]LocalID

	// sealed tracks whether all predecessors of a block are known.
	sealed map[BlockID]bool

	// incompletePhis tracks phi nodes that need operands added
	// when the block is sealed.
	incompletePhis map[BlockID][]incompletePhi

	// loopState for break/continue handling.
	loopStack []loopContext

	// lambdaCounter for generating unique lambda function names.
	lambdaCounter int

	// lambdaFns collects MIR functions created for lambda bodies.
	lambdaFns *[]*Function
}

type incompletePhi struct {
	name string
	phi  *Phi
}

type loopContext struct {
	header  BlockID // loop header (continue target)
	exit    BlockID // loop exit (break target)
	exitVal LocalID // local to store break value (NoLocal if none)
}

func buildFunction(hirFn *hir.Function, prog *hir.Program, lambdaFns *[]*Function) *Function {
	fn := &Function{
		Name:       hirFn.Name,
		ReturnType: hirFn.ReturnType,
		Entry:      0,
	}

	b := &builder{
		fn:             fn,
		prog:           prog,
		lambdaFns:      lambdaFns,
		currentDef:     make(map[string]map[BlockID]LocalID),
		sealed:         make(map[BlockID]bool),
		incompletePhis: make(map[BlockID][]incompletePhi),
	}

	// Create entry block.
	entry := fn.NewBlock("entry")
	b.curBlock = entry
	b.sealBlock(entry) // entry has no predecessors

	// Create locals for parameters.
	for _, p := range hirFn.Params {
		local := fn.NewLocal(p.Name, p.Type)
		fn.Params = append(fn.Params, local)
		b.writeVariable(p.Name, entry, local)
	}

	// Lower function body.
	if hirFn.Body != nil {
		val := b.lowerBlock(hirFn.Body)
		// If the current block has no terminator, add a return.
		cur := fn.Block(b.curBlock)
		if cur.Term == nil {
			if val != nil {
				cur.Term = &Return{Value: val}
			} else {
				cur.Term = &Return{Value: UnitConst()}
			}
		}
	}

	// Ensure all blocks have terminators.
	for _, blk := range fn.Blocks {
		if blk.Term == nil {
			blk.Term = &Unreachable{}
		}
	}

	return fn
}

// ---------------------------------------------------------------------------
// Braun et al. SSA construction
// ---------------------------------------------------------------------------

func (b *builder) writeVariable(name string, block BlockID, local LocalID) {
	if b.currentDef[name] == nil {
		b.currentDef[name] = make(map[BlockID]LocalID)
	}
	b.currentDef[name][block] = local
}

func (b *builder) readVariable(name string, block BlockID) Value {
	if defs, ok := b.currentDef[name]; ok {
		if local, ok := defs[block]; ok {
			return b.fn.LocalRef(local)
		}
	}
	return b.readVariableRecursive(name, block)
}

func (b *builder) readVariableRecursive(name string, block BlockID) Value {
	var val Value

	if !b.sealed[block] {
		// Block not yet sealed — create incomplete phi.
		typ := b.varType(name)
		dest := b.fn.NewLocal(name, typ)
		phi := &Phi{
			Dest: dest,
			Type: typ,
			Args: make(map[BlockID]Value),
		}
		b.fn.Block(block).Phis = append(b.fn.Block(block).Phis, phi)
		b.incompletePhis[block] = append(b.incompletePhis[block], incompletePhi{
			name: name,
			phi:  phi,
		})
		val = b.fn.LocalRef(dest)
	} else {
		preds := b.fn.Block(block).Preds
		if len(preds) == 1 {
			// Single predecessor — no phi needed.
			val = b.readVariable(name, preds[0])
		} else if len(preds) == 0 {
			// No predecessors (entry block) — variable not defined.
			// Return a zero-value constant for the type.
			val = b.zeroValue(b.varType(name))
		} else {
			// Multiple predecessors — insert phi.
			typ := b.varType(name)
			dest := b.fn.NewLocal(name, typ)
			phi := &Phi{
				Dest: dest,
				Type: typ,
				Args: make(map[BlockID]Value),
			}
			b.fn.Block(block).Phis = append(b.fn.Block(block).Phis, phi)
			b.writeVariable(name, block, dest)
			// Add phi operands (must be done after writeVariable to break cycles).
			for _, pred := range preds {
				phi.Args[pred] = b.readVariable(name, pred)
			}
			// Propagate UpvalueAlias if all non-trivial phi operands agree on the same alias.
			commonAlias := -1
			aliasConsistent := true
			for _, arg := range phi.Args {
				if ref, ok := arg.(*Local); ok {
					if ref.ID == dest {
						continue // skip self-reference
					}
					a := b.fn.Locals[int(ref.ID)].UpvalueAlias
					if a >= 0 {
						if commonAlias == -1 {
							commonAlias = a
						} else if commonAlias != a {
							aliasConsistent = false
							break
						}
					}
				}
			}
			if aliasConsistent && commonAlias >= 0 {
				b.fn.Locals[int(dest)].UpvalueAlias = commonAlias
			}
			val = b.tryRemoveTrivialPhi(phi, block)
		}
	}

	if local, ok := val.(*Local); ok {
		b.writeVariable(name, block, local.ID)
	}
	return val
}

func (b *builder) tryRemoveTrivialPhi(phi *Phi, block BlockID) Value {
	var same Value
	for _, v := range phi.Args {
		if valEqual(v, same) {
			continue // unique value or self-reference
		}
		if local, ok := v.(*Local); ok && local.ID == phi.Dest {
			continue // self-reference
		}
		if same != nil {
			return b.fn.LocalRef(phi.Dest) // non-trivial phi
		}
		same = v
	}
	if same == nil {
		same = UnitConst() // unreachable or undefined
	}

	// Phi is trivial — remove it.
	blk := b.fn.Block(block)
	for i, p := range blk.Phis {
		if p == phi {
			blk.Phis = append(blk.Phis[:i], blk.Phis[i+1:]...)
			break
		}
	}

	// Replace all references to the removed phi's dest in other phis' args.
	// Without this, other phis may hold stale references to the dead local,
	// causing incorrect values at runtime (Braun et al. SSA construction
	// requires propagating trivial phi removal to all users).
	removedID := phi.Dest
	for _, bb := range b.fn.Blocks {
		for _, p := range bb.Phis {
			for predID, arg := range p.Args {
				if local, ok := arg.(*Local); ok && local.ID == removedID {
					p.Args[predID] = same
				}
			}
		}
	}

	return same
}

func (b *builder) sealBlock(block BlockID) {
	for _, ip := range b.incompletePhis[block] {
		for _, pred := range b.fn.Block(block).Preds {
			ip.phi.Args[pred] = b.readVariable(ip.name, pred)
		}
	}
	delete(b.incompletePhis, block)
	b.sealed[block] = true
}

func (b *builder) varType(name string) types.Type {
	// Search all blocks for the variable type via the local definition.
	if defs, ok := b.currentDef[name]; ok {
		for _, localID := range defs {
			return b.fn.Locals[int(localID)].Type
		}
	}
	return types.TypUnit
}

// isGlobal returns true if name refers to a top-level function (either
// user-defined in the HIR program or a known built-in) that should be
// emitted as a Global reference rather than an SSA local lookup.
func (b *builder) isGlobal(name string) bool {
	// Not global if the variable is defined in any SSA block (e.g. a
	// function parameter defined in the entry block must shadow a
	// builtin of the same name in all subsequent blocks).
	if defs, ok := b.currentDef[name]; ok && len(defs) > 0 {
		return false
	}
	// Check user-defined functions in the HIR program.
	for _, fn := range b.prog.Functions {
		if fn.Name == name {
			return true
		}
	}
	// Check known built-in function names.
	return isBuiltinName(name)
}

// isBuiltinName returns true if name is a known built-in function.
func isBuiltinName(name string) bool {
	switch name {
	case "print", "println", "read_line",
		"int_to_float", "float_to_int", "int_to_string", "float_to_string",
		"parse_int", "parse_float", "bool_to_string",
		"string_to_int", "string_to_float", "char_to_int", "int_to_char",
		"string_len", "array_len",
		"assert", "assert_eq", "panic",
		"eq", "neq", "compare", "to_string", "default", "clone", "hash",
		"abs", "min", "max", "sqrt", "pow", "floor", "ceil", "round",
		"sin", "cos", "tan", "asin", "acos", "atan", "atan2",
		"log", "log2", "log10", "exp", "pi", "e", "gcd", "lcm", "clamp",
		"random_int", "random_float",
		"time_now_ms", "sleep_ms", "random_seed", "random_shuffle", "random_choice",
		"string_chars", "string_contains", "string_split", "string_trim",
		"string_replace", "string_starts_with", "string_ends_with",
		"string_to_upper", "string_to_lower", "string_repeat", "string_reverse",
		"string_slice", "string_index_of", "string_pad_left", "string_pad_right",
		"string_bytes", "string_join", "char_to_string",
		"array_push", "array_pop", "array_slice", "array_reverse",
		"array_contains", "array_map", "array_filter", "array_fold",
		"array_sort", "array_flatten", "array_zip", "array_enumerate",
		"array_flat_map", "array_find", "array_any", "array_all",
		"array_sum", "array_min", "array_max", "array_take", "array_drop",
		"array_chunk", "array_unique", "array_join",
		"read_file", "write_file", "file_exists", "dir_list", "dir_create",
		"path_join", "path_dirname", "path_basename", "path_extension", "file_size",
		"map_new", "map_get", "map_set", "map_delete", "map_contains",
		"map_len", "map_keys", "map_values", "map_entries",
		"map_merge", "map_filter", "map_map",
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
		"gfx_image_width", "gfx_image_height":
		return true
	}
	return false
}

func (b *builder) zeroValue(typ types.Type) Value {
	switch typ.(type) {
	case *types.IntType:
		return IntConst(0)
	case *types.FloatType:
		return FloatConst(0)
	case *types.BoolType:
		return BoolConst(false)
	case *types.StringType:
		return StringConst("")
	default:
		return UnitConst()
	}
}

// ---------------------------------------------------------------------------
// Lowering: HIR -> MIR
// ---------------------------------------------------------------------------

func (b *builder) lowerBlock(block *hir.Block) Value {
	var val Value
	for _, stmt := range block.Stmts {
		b.lowerStmt(stmt)
	}
	if block.TrailingExpr != nil {
		val = b.lowerExpr(block.TrailingExpr)
	}
	return val
}

func (b *builder) lowerStmt(stmt hir.Stmt) {
	switch s := stmt.(type) {
	case *hir.LetStmt:
		val := b.lowerExpr(s.Value)
		local := b.fn.NewLocal(s.Name, s.Type)
		b.fn.Locals[int(local)].Mutable = s.Mutable
		b.emit(&Assign{Dest: local, Src: val, Type: s.Type})
		b.writeVariable(s.Name, b.curBlock, local)

	case *hir.ExprStmt:
		b.lowerExpr(s.Expr)

	case *hir.ReturnStmt:
		if s.Value != nil {
			val := b.lowerExpr(s.Value)
			b.fn.Block(b.curBlock).Term = &Return{Value: val}
		} else {
			b.fn.Block(b.curBlock).Term = &Return{Value: UnitConst()}
		}
		// Create a new unreachable block for any dead code after return.
		dead := b.fn.NewBlock("dead")
		b.sealBlock(dead)
		b.curBlock = dead
	}
}

func (b *builder) lowerExpr(expr hir.Expr) Value {
	switch e := expr.(type) {
	case *hir.IntLit:
		// Use base 0 to auto-detect hex (0x), octal (0o), binary (0b) literals.
		cleaned := strings.ReplaceAll(e.Value, "_", "")
		v, _ := strconv.ParseInt(cleaned, 0, 64)
		return IntConst(v)

	case *hir.FloatLit:
		v, _ := strconv.ParseFloat(e.Value, 64)
		return FloatConst(v)

	case *hir.StringLit:
		return StringConst(e.Value)

	case *hir.CharLit:
		return &Const{Kind: ConstChar, Str: e.Value, Type: types.TypChar}

	case *hir.BoolLit:
		return BoolConst(e.Value)

	case *hir.UnitLit:
		return UnitConst()

	case *hir.VarRef:
		// If the variable is not defined as a local in the current SSA scope,
		// check whether it's a top-level function or a built-in name and
		// emit a Global reference.
		if b.isGlobal(e.Name) {
			return &Global{Name: e.Name, Type: e.ExprType()}
		}
		return b.readVariable(e.Name, b.curBlock)

	case *hir.PathRef:
		name := ""
		for i, seg := range e.Segments {
			if i > 0 {
				name += "::"
			}
			name += seg
		}
		return &Global{Name: name, Type: e.ExprType()}

	case *hir.Block:
		return b.lowerBlock(e)

	case *hir.IfExpr:
		return b.lowerIf(e)

	case *hir.WhileExpr:
		return b.lowerWhile(e)

	case *hir.LoopExpr:
		return b.lowerLoop(e)

	case *hir.Call:
		return b.lowerCall(e)

	case *hir.StaticCall:
		return b.lowerStaticCall(e)

	case *hir.BinaryOp:
		return b.lowerBinaryOp(e)

	case *hir.UnaryOp:
		return b.lowerUnaryOp(e)

	case *hir.FieldAccess:
		obj := b.lowerExpr(e.Object)
		dest := b.fn.NewLocal("field", e.ExprType())
		b.emit(&FieldAccessStmt{Dest: dest, Object: obj, Field: e.Field, Type: e.ExprType()})
		return b.fn.LocalRef(dest)

	case *hir.Index:
		obj := b.lowerExpr(e.Object)
		idx := b.lowerExpr(e.Idx)
		dest := b.fn.NewLocal("idx", e.ExprType())
		b.emit(&IndexAccessStmt{Dest: dest, Object: obj, Index: idx, Type: e.ExprType()})
		return b.fn.LocalRef(dest)

	case *hir.ArrayLiteral:
		elems := make([]Value, len(e.Elems))
		for i, el := range e.Elems {
			elems[i] = b.lowerExpr(el)
		}
		dest := b.fn.NewLocal("arr", e.ExprType())
		b.emit(&ArrayAllocStmt{Dest: dest, Elems: elems, Type: e.ExprType()})
		return b.fn.LocalRef(dest)

	case *hir.TupleLiteral:
		// Lower tuples as struct allocs with numeric field names.
		fields := make([]FieldValue, len(e.Elems))
		for i, el := range e.Elems {
			fields[i] = FieldValue{
				Name:  fmt.Sprintf("%d", i),
				Value: b.lowerExpr(el),
			}
		}
		dest := b.fn.NewLocal("tuple", e.ExprType())
		b.emit(&StructAllocStmt{Dest: dest, Name: "tuple", Fields: fields, Type: e.ExprType()})
		return b.fn.LocalRef(dest)

	case *hir.StructLiteral:
		fields := make([]FieldValue, len(e.Fields))
		for i, f := range e.Fields {
			fields[i] = FieldValue{
				Name:  f.Name,
				Value: b.lowerExpr(f.Value),
			}
		}
		dest := b.fn.NewLocal(e.Name, e.ExprType())
		b.emit(&StructAllocStmt{Dest: dest, Name: e.Name, Fields: fields, Type: e.ExprType()})
		return b.fn.LocalRef(dest)

	case *hir.Assign: // [CLAUDE-FIX] Handle variable assignment
		// Look up current definition before creating new local, to propagate UpvalueAlias.
		curDef := b.readVariable(e.Name, b.curBlock)
		val := b.lowerExpr(e.Value)
		local := b.fn.NewLocal(e.Name, e.Value.ExprType())
		// Propagate UpvalueAlias from the current definition so mutations
		// inside closures go through the upvalue cell, not ephemeral stack slots.
		if ref, ok := curDef.(*Local); ok {
			if alias := b.fn.Locals[int(ref.ID)].UpvalueAlias; alias >= 0 {
				b.fn.Locals[int(local)].UpvalueAlias = alias
			}
		}
		b.emit(&Assign{Dest: local, Src: val, Type: e.Value.ExprType()})
		b.writeVariable(e.Name, b.curBlock, local)
		return UnitConst()

	case *hir.FieldAssign: // [CLAUDE-FIX] Handle field assignment
		obj := b.lowerExpr(e.Object)
		val := b.lowerExpr(e.Value)
		fieldIdx := e.Field
		b.emit(&FieldSetStmt{Object: obj, Field: fieldIdx, Value: val})
		return UnitConst()

	case *hir.IndexAssign: // [CLAUDE-FIX] Handle index assignment
		obj := b.lowerExpr(e.Object)
		idx := b.lowerExpr(e.Index)
		val := b.lowerExpr(e.Value)
		b.emit(&IndexSetStmt{Object: obj, Index: idx, Value: val})
		return UnitConst()

	case *hir.EnumConstruct: // [CLAUDE-FIX] Handle enum variant constructor calls
		args := make([]Value, len(e.Args))
		for i, a := range e.Args {
			args[i] = b.lowerExpr(a)
		}
		dest := b.fn.NewLocal("enum", e.ExprType())
		b.emit(&EnumAllocStmt{
			Dest:     dest,
			EnumName: e.EnumName,
			Variant:  e.Variant,
			Args:     args,
			Type:     e.ExprType(),
		})
		return b.fn.LocalRef(dest)

	case *hir.ChannelCreate:
		var bufSize Value
		if e.BufSize != nil {
			bufSize = b.lowerExpr(e.BufSize)
		}
		dest := b.fn.NewLocal("chan", e.ExprType())
		b.emit(&ChannelCreateStmt{Dest: dest, ElemType: e.ElemType, BufSize: bufSize, Type: e.ExprType()})
		return b.fn.LocalRef(dest)

	case *hir.Spawn:
		return b.lowerSpawn(e)

	case *hir.BreakExpr:
		if len(b.loopStack) > 0 {
			ctx := b.loopStack[len(b.loopStack)-1]
			b.fn.Block(b.curBlock).Term = &Goto{Target: ctx.exit}
			b.fn.AddEdge(b.curBlock, ctx.exit)
			dead := b.fn.NewBlock("post.break")
			b.sealBlock(dead)
			b.curBlock = dead
		}
		return UnitConst()

	case *hir.ContinueExpr:
		if len(b.loopStack) > 0 {
			ctx := b.loopStack[len(b.loopStack)-1]
			b.fn.Block(b.curBlock).Term = &Goto{Target: ctx.header}
			b.fn.AddEdge(b.curBlock, ctx.header)
			dead := b.fn.NewBlock("post.continue")
			b.sealBlock(dead)
			b.curBlock = dead
		}
		return UnitConst()

	case *hir.ReturnExpr:
		if e.Value != nil {
			val := b.lowerExpr(e.Value)
			b.fn.Block(b.curBlock).Term = &Return{Value: val}
		} else {
			b.fn.Block(b.curBlock).Term = &Return{Value: UnitConst()}
		}
		dead := b.fn.NewBlock("post.return")
		b.sealBlock(dead)
		b.curBlock = dead
		return UnitConst()

	case *hir.Cast:
		src := b.lowerExpr(e.Expr)
		dest := b.fn.NewLocal("cast", e.Target)
		b.emit(&CastStmt{Dest: dest, Src: src, Target: e.Target, Type: e.Target})
		return b.fn.LocalRef(dest)

	case *hir.Lambda:
		return b.lowerLambda(e)

	case *hir.MatchExpr:
		return b.lowerMatch(e)

	default:
		return UnitConst()
	}
}

// ---------------------------------------------------------------------------
// Control flow lowering
// ---------------------------------------------------------------------------

func (b *builder) lowerIf(e *hir.IfExpr) Value {
	cond := b.lowerExpr(e.Cond)
	resultType := e.ExprType()

	thenBlock := b.fn.NewBlock("if.then")
	elseBlock := b.fn.NewBlock("if.else")
	mergeBlock := b.fn.NewBlock("if.merge")

	b.fn.Block(b.curBlock).Term = &Branch{Cond: cond, Then: thenBlock, Else: elseBlock}
	b.fn.AddEdge(b.curBlock, thenBlock)
	b.fn.AddEdge(b.curBlock, elseBlock)
	b.sealBlock(thenBlock)
	b.sealBlock(elseBlock)

	// Lower then branch.
	b.curBlock = thenBlock
	thenVal := b.lowerBlock(e.Then)
	if thenVal == nil {
		thenVal = UnitConst()
	}
	thenExit := b.curBlock
	if b.fn.Block(thenExit).Term == nil {
		b.fn.Block(thenExit).Term = &Goto{Target: mergeBlock}
		b.fn.AddEdge(thenExit, mergeBlock)
	}

	// Lower else branch.
	b.curBlock = elseBlock
	var elseVal Value
	if e.Else != nil {
		switch elseExpr := e.Else.(type) {
		case *hir.Block:
			elseVal = b.lowerBlock(elseExpr)
		default:
			elseVal = b.lowerExpr(e.Else)
		}
	}
	if elseVal == nil {
		elseVal = UnitConst()
	}
	elseExit := b.curBlock
	if b.fn.Block(elseExit).Term == nil {
		b.fn.Block(elseExit).Term = &Goto{Target: mergeBlock}
		b.fn.AddEdge(elseExit, mergeBlock)
	}

	b.sealBlock(mergeBlock)
	b.curBlock = mergeBlock

	// If both branches produce values and merge block has predecessors,
	// insert a phi node.
	if len(b.fn.Block(mergeBlock).Preds) > 0 && resultType != nil && !resultType.Equal(types.TypUnit) {
		dest := b.fn.NewLocal("if.val", resultType)
		phi := &Phi{
			Dest: dest,
			Type: resultType,
			Args: make(map[BlockID]Value),
		}
		phi.Args[thenExit] = thenVal
		phi.Args[elseExit] = elseVal
		b.fn.Block(mergeBlock).Phis = append(b.fn.Block(mergeBlock).Phis, phi)
		return b.fn.LocalRef(dest)
	}

	return UnitConst()
}

func (b *builder) lowerWhile(e *hir.WhileExpr) Value {
	headerBlock := b.fn.NewBlock("while.header")
	bodyBlock := b.fn.NewBlock("while.body")
	exitBlock := b.fn.NewBlock("while.exit")

	// Jump to header.
	b.fn.Block(b.curBlock).Term = &Goto{Target: headerBlock}
	b.fn.AddEdge(b.curBlock, headerBlock)

	// Header: evaluate condition.
	b.curBlock = headerBlock
	// Don't seal header yet — back edge from body not yet added.

	b.loopStack = append(b.loopStack, loopContext{
		header: headerBlock,
		exit:   exitBlock,
	})

	cond := b.lowerExpr(e.Cond)
	b.fn.Block(b.curBlock).Term = &Branch{Cond: cond, Then: bodyBlock, Else: exitBlock}
	condExit := b.curBlock
	b.fn.AddEdge(condExit, bodyBlock)
	b.fn.AddEdge(condExit, exitBlock)
	b.sealBlock(bodyBlock)

	// Body.
	b.curBlock = bodyBlock
	b.lowerBlock(e.Body)
	if b.fn.Block(b.curBlock).Term == nil {
		b.fn.Block(b.curBlock).Term = &Goto{Target: headerBlock}
		b.fn.AddEdge(b.curBlock, headerBlock)
	}

	// Now seal header (all predecessors known: entry + back edge).
	b.sealBlock(headerBlock)

	b.loopStack = b.loopStack[:len(b.loopStack)-1]

	b.sealBlock(exitBlock)
	b.curBlock = exitBlock
	return UnitConst()
}

func (b *builder) lowerLoop(e *hir.LoopExpr) Value {
	headerBlock := b.fn.NewBlock("loop.header")
	exitBlock := b.fn.NewBlock("loop.exit")

	b.fn.Block(b.curBlock).Term = &Goto{Target: headerBlock}
	b.fn.AddEdge(b.curBlock, headerBlock)

	b.loopStack = append(b.loopStack, loopContext{
		header: headerBlock,
		exit:   exitBlock,
	})

	b.curBlock = headerBlock
	b.lowerBlock(e.Body)
	if b.fn.Block(b.curBlock).Term == nil {
		b.fn.Block(b.curBlock).Term = &Goto{Target: headerBlock}
		b.fn.AddEdge(b.curBlock, headerBlock)
	}

	b.sealBlock(headerBlock)
	b.loopStack = b.loopStack[:len(b.loopStack)-1]

	b.sealBlock(exitBlock)
	b.curBlock = exitBlock
	return UnitConst()
}

// ---------------------------------------------------------------------------
// Call lowering
// ---------------------------------------------------------------------------

func (b *builder) lowerCall(e *hir.Call) Value {
	fn := b.lowerExpr(e.Func)
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		args[i] = b.lowerExpr(a)
	}
	dest := b.fn.NewLocal("call", e.ExprType())
	b.emit(&CallStmt{Dest: dest, Func: fn, Args: args, Type: e.ExprType()})
	return b.fn.LocalRef(dest)
}

func (b *builder) lowerStaticCall(e *hir.StaticCall) Value {
	// [CLAUDE-FIX] Handle channel::send and channel::recv as builtin MIR statements
	if e.TypeName == "channel" {
		switch e.Method {
		case "send":
			if len(e.Args) >= 2 {
				ch := b.lowerExpr(e.Args[0])
				val := b.lowerExpr(e.Args[1])
				b.emit(&ChannelSendStmt{Chan: ch, SendVal: val})
				return UnitConst()
			}
		case "recv":
			if len(e.Args) >= 1 {
				ch := b.lowerExpr(e.Args[0])
				dest := b.fn.NewLocal("recv", e.ExprType())
				b.emit(&ChannelRecvStmt{Dest: dest, Chan: ch, Type: e.ExprType()})
				return b.fn.LocalRef(dest)
			}
		case "close":
			if len(e.Args) >= 1 {
				ch := b.lowerExpr(e.Args[0])
				b.emit(&ChannelCloseStmt{Chan: ch})
				return UnitConst()
			}
		}
	}

	name := e.TypeName + "::" + e.Method
	fn := &Global{Name: name, Type: e.ExprType()}
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		args[i] = b.lowerExpr(a)
	}
	dest := b.fn.NewLocal("call", e.ExprType())
	b.emit(&CallStmt{Dest: dest, Func: fn, Args: args, Type: e.ExprType()})
	return b.fn.LocalRef(dest)
}

func (b *builder) lowerBinaryOp(e *hir.BinaryOp) Value {
	// Handle short-circuit operators.
	if e.Op == "&&" || e.Op == "||" {
		return b.lowerShortCircuit(e)
	}
	left := b.lowerExpr(e.Left)
	right := b.lowerExpr(e.Right)
	dest := b.fn.NewLocal("binop", e.ExprType())
	b.emit(&BinaryOpStmt{Dest: dest, Op: e.Op, Left: left, Right: right, Type: e.ExprType()})
	return b.fn.LocalRef(dest)
}

func (b *builder) lowerShortCircuit(e *hir.BinaryOp) Value {
	left := b.lowerExpr(e.Left)
	leftBlock := b.curBlock

	rhsBlock := b.fn.NewBlock("sc.rhs")
	mergeBlock := b.fn.NewBlock("sc.merge")

	if e.Op == "&&" {
		// If left is false, short-circuit to false.
		b.fn.Block(b.curBlock).Term = &Branch{Cond: left, Then: rhsBlock, Else: mergeBlock}
	} else {
		// If left is true, short-circuit to true.
		b.fn.Block(b.curBlock).Term = &Branch{Cond: left, Then: mergeBlock, Else: rhsBlock}
	}
	b.fn.AddEdge(b.curBlock, rhsBlock)
	b.fn.AddEdge(b.curBlock, mergeBlock)
	b.sealBlock(rhsBlock)

	b.curBlock = rhsBlock
	right := b.lowerExpr(e.Right)
	rhsExit := b.curBlock
	b.fn.Block(rhsExit).Term = &Goto{Target: mergeBlock}
	b.fn.AddEdge(rhsExit, mergeBlock)

	b.sealBlock(mergeBlock)
	b.curBlock = mergeBlock

	dest := b.fn.NewLocal("sc", types.TypBool)
	phi := &Phi{
		Dest: dest,
		Type: types.TypBool,
		Args: make(map[BlockID]Value),
	}
	phi.Args[leftBlock] = left
	phi.Args[rhsExit] = right
	b.fn.Block(mergeBlock).Phis = append(b.fn.Block(mergeBlock).Phis, phi)
	return b.fn.LocalRef(dest)
}

func (b *builder) lowerUnaryOp(e *hir.UnaryOp) Value {
	operand := b.lowerExpr(e.Operand)
	dest := b.fn.NewLocal("unop", e.ExprType())
	b.emit(&UnaryOpStmt{Dest: dest, Op: e.Op, Operand: operand, Type: e.ExprType()})
	return b.fn.LocalRef(dest)
}

// ---------------------------------------------------------------------------
// Spawn / Lambda / Match lowering
// ---------------------------------------------------------------------------

func (b *builder) lowerSpawn(e *hir.Spawn) Value {
	// [CLAUDE-FIX] The spawn body has been extracted into a top-level function
	// by extractSpawnBodies. The body is now a block containing a single call
	// to the extracted function. Lower the call args and emit SpawnStmt.
	if e.Body != nil && len(e.Body.Stmts) > 0 {
		if exprStmt, ok := e.Body.Stmts[0].(*hir.ExprStmt); ok {
			if call, ok := exprStmt.Expr.(*hir.Call); ok {
				fn := b.lowerExpr(call.Func)
				args := make([]Value, len(call.Args))
				for i, a := range call.Args {
					args[i] = b.lowerExpr(a)
				}
				dest := b.fn.NewLocal("spawn", e.ExprType())
				b.emit(&SpawnStmt{Dest: dest, Func: fn, Args: args, Type: e.ExprType()})
				return b.fn.LocalRef(dest)
			}
		}
	}
	// Fallback for non-extracted spawns.
	lambdaName := fmt.Sprintf("%s$spawn%d", b.fn.Name, b.lambdaCounter)
	b.lambdaCounter++
	spawnFunc := &Global{Name: lambdaName, Type: types.TypUnit}
	dest := b.fn.NewLocal("spawn", e.ExprType())
	b.emit(&SpawnStmt{Dest: dest, Func: spawnFunc, Type: e.ExprType()})
	return b.fn.LocalRef(dest)
}

func (b *builder) lowerLambda(e *hir.Lambda) Value {
	lambdaName := fmt.Sprintf("%s$lambda%d", b.fn.Name, b.lambdaCounter)
	b.lambdaCounter++

	// Build a MIR function for the lambda body.
	lambdaFn := b.buildLambdaFunction(lambdaName, e)
	*b.lambdaFns = append(*b.lambdaFns, lambdaFn)

	captures := make([]Value, len(e.Captures))
	for i, cap := range e.Captures {
		captures[i] = b.readVariable(cap.Name, b.curBlock)
	}

	dest := b.fn.NewLocal("closure", e.ExprType())
	b.emit(&ClosureAllocStmt{
		Dest:     dest,
		FuncName: lambdaName,
		Captures: captures,
		Type:     e.ExprType(),
	})
	return b.fn.LocalRef(dest)
}

// buildLambdaFunction creates a MIR function from a lambda's body.
// Captured variables are accessed via Upvalue references; lambda params
// become regular function parameters.
func (b *builder) buildLambdaFunction(name string, lambda *hir.Lambda) *Function {
	// Determine return type from the lambda's function type.
	var retType types.Type = types.TypUnit
	if ft, ok := lambda.ExprType().(*types.FnType); ok {
		retType = ft.Return
	}

	fn := &Function{
		Name:         name,
		ReturnType:   retType,
		Entry:        0,
		UpvalueCount: len(lambda.Captures),
	}

	lb := &builder{
		fn:             fn,
		prog:           b.prog,
		lambdaFns:      b.lambdaFns, // support nested lambdas
		currentDef:     make(map[string]map[BlockID]LocalID),
		sealed:         make(map[BlockID]bool),
		incompletePhis: make(map[BlockID][]incompletePhi),
	}

	entry := fn.NewBlock("entry")
	lb.curBlock = entry
	lb.sealBlock(entry)

	// Load captured variables from upvalues into locals.
	for i, cap := range lambda.Captures {
		local := fn.NewLocal(cap.Name, cap.Type)
		fn.Locals[int(local)].UpvalueAlias = i
		lb.emit(&Assign{
			Dest: local,
			Src:  &Upvalue{Index: i, Type: cap.Type},
			Type: cap.Type,
		})
		lb.writeVariable(cap.Name, entry, local)
	}

	// Create locals for lambda parameters.
	for _, p := range lambda.Params {
		local := fn.NewLocal(p.Name, p.Type)
		fn.Params = append(fn.Params, local)
		lb.writeVariable(p.Name, entry, local)
	}

	// Lower the lambda body.
	val := lb.lowerExpr(lambda.Body)

	// Add return terminator.
	cur := fn.Block(lb.curBlock)
	if cur.Term == nil {
		if val != nil {
			cur.Term = &Return{Value: val}
		} else {
			cur.Term = &Return{Value: UnitConst()}
		}
	}

	// Ensure all blocks have terminators.
	for _, blk := range fn.Blocks {
		if blk.Term == nil {
			blk.Term = &Unreachable{}
		}
	}

	return fn
}

type armResult struct {
	val   Value
	block BlockID
}

func (b *builder) lowerMatch(e *hir.MatchExpr) Value {
	scrutinee := b.lowerExpr(e.Scrutinee)
	resultType := e.ExprType()
	mergeBlock := b.fn.NewBlock("match.merge")

	var results []armResult

	if e.Decision != nil {
		b.lowerDecision(e.Decision, scrutinee, resultType, mergeBlock, &results)
	}

	b.sealBlock(mergeBlock)
	b.curBlock = mergeBlock

	if len(results) > 0 && resultType != nil && !resultType.Equal(types.TypUnit) {
		dest := b.fn.NewLocal("match.val", resultType)
		phi := &Phi{
			Dest: dest,
			Type: resultType,
			Args: make(map[BlockID]Value),
		}
		for _, r := range results {
			phi.Args[r.block] = r.val
		}
		b.fn.Block(mergeBlock).Phis = append(b.fn.Block(mergeBlock).Phis, phi)
		return b.fn.LocalRef(dest)
	}

	return UnitConst()
}

func (b *builder) lowerDecision(
	dec hir.Decision,
	scrutinee Value,
	resultType types.Type,
	mergeBlock BlockID,
	results *[]armResult,
) {
	switch d := dec.(type) {
	case *hir.DecisionLeaf:
		// Bind pattern variables.
		for _, bind := range d.Bindings {
			val := b.lowerExpr(bind.Expr)
			local := b.fn.NewLocal(bind.Name, bind.Type)
			b.emit(&Assign{Dest: local, Src: val, Type: bind.Type})
			b.writeVariable(bind.Name, b.curBlock, local)
		}
		bodyVal := b.lowerExpr(d.Body)
		if bodyVal == nil {
			bodyVal = UnitConst()
		}
		exitBlock := b.curBlock
		if b.fn.Block(exitBlock).Term == nil {
			b.fn.Block(exitBlock).Term = &Goto{Target: mergeBlock}
			b.fn.AddEdge(exitBlock, mergeBlock)
		}
		*results = append(*results, armResult{val: bodyVal, block: exitBlock})

	case *hir.DecisionSwitch:
		// Lower the HIR scrutinee for this switch level. For nested switches
		// (e.g., checking a destructured field), this differs from the parent
		// scrutinee MIR value.
		switchVal := b.lowerExpr(d.Scrutinee)

		enumName := ""
		if switchVal.ValueType() != nil {
			if et, ok := switchVal.ValueType().(*types.EnumType); ok {
				enumName = et.Name
			}
		}

		// Build chain of tag checks: if variant matches, go to case block; else try next.
		for _, c := range d.Cases {
			caseBlock := b.fn.NewBlock("match.case")
			nextBlock := b.fn.NewBlock("match.next")

			if enumName != "" {
				// Emit tag check: does scrutinee's variant == c.Constructor?
				tagResult := b.fn.NewLocal("tag.eq", types.TypBool)
				b.emit(&TagCheckStmt{
					Dest:     tagResult,
					Object:   switchVal,
					EnumName: enumName,
					Variant:  c.Constructor,
					Type:     types.TypBool,
				})
				b.fn.Block(b.curBlock).Term = &Branch{
					Cond: b.fn.LocalRef(tagResult),
					Then: caseBlock,
					Else: nextBlock,
				}
			} else {
				// Non-enum switch (literal patterns): fall through for now
				b.fn.Block(b.curBlock).Term = &Goto{Target: caseBlock}
			}
			b.fn.AddEdge(b.curBlock, caseBlock)
			b.fn.AddEdge(b.curBlock, nextBlock)
			b.sealBlock(caseBlock)
			b.curBlock = caseBlock

			// Field bindings are handled by DecisionLeaf via the match
			// compiler's FieldAccess nodes. Pass the original scrutinee
			// for phi/result tracking but the switch dispatches on switchVal.
			b.lowerDecision(c.Body, scrutinee, resultType, mergeBlock, results)

			// Continue from the "next" block for the next case check.
			b.sealBlock(nextBlock)
			b.curBlock = nextBlock
		}

		// After all cases: handle default or unreachable.
		if d.Default != nil {
			b.lowerDecision(d.Default, scrutinee, resultType, mergeBlock, results)
		} else {
			b.fn.Block(b.curBlock).Term = &Unreachable{}
		}

	case *hir.DecisionGuard:
		cond := b.lowerExpr(d.Condition)
		thenBlock := b.fn.NewBlock("guard.then")
		elseBlock := b.fn.NewBlock("guard.else")
		b.fn.Block(b.curBlock).Term = &Branch{Cond: cond, Then: thenBlock, Else: elseBlock}
		b.fn.AddEdge(b.curBlock, thenBlock)
		b.fn.AddEdge(b.curBlock, elseBlock)
		b.sealBlock(thenBlock)
		b.sealBlock(elseBlock)

		b.curBlock = thenBlock
		b.lowerDecision(d.Then, scrutinee, resultType, mergeBlock, results)

		b.curBlock = elseBlock
		b.lowerDecision(d.Else, scrutinee, resultType, mergeBlock, results)

	case *hir.DecisionFail:
		b.fn.Block(b.curBlock).Term = &Unreachable{}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (b *builder) emit(stmt Stmt) {
	blk := b.fn.Block(b.curBlock)
	blk.Stmts = append(blk.Stmts, stmt)
}

func valEqual(a, b Value) bool {
	if a == nil || b == nil {
		return a == b
	}
	la, aOk := a.(*Local)
	lb, bOk := b.(*Local)
	if aOk && bOk {
		return la.ID == lb.ID
	}
	return false
}
