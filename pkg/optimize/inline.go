package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// InlinePass inlines small, non-recursive functions.
type InlinePass struct {
	MaxStmts int // maximum statement count to inline (default 8)
}

func (*InlinePass) Name() string { return "inline" }

func (p *InlinePass) Run(fn *mir.Function) {
	if p.MaxStmts == 0 {
		p.MaxStmts = 8
	}

	// This pass operates on a function with access to the program context.
	// Since we don't have the program in scope, we inline within the function:
	// look for call sites to functions defined in the same program.
	// For now, we handle the simpler case of inlining calls to functions
	// that are passed via a global lookup.

	// We need program context for cross-function inlining. For single-function
	// inlining within a program, use InlinePassWithProgram.
}

// InlinePassWithProgram inlines small functions with program context.
type InlinePassWithProgram struct {
	MaxStmts int
	Program  *mir.Program
}

func (*InlinePassWithProgram) Name() string { return "inline" }

func (p *InlinePassWithProgram) Run(fn *mir.Function) {
	if p.MaxStmts == 0 {
		p.MaxStmts = 8
	}

	// Build function map.
	funcMap := make(map[string]*mir.Function)
	for _, f := range p.Program.Functions {
		funcMap[f.Name] = f
	}

	// Find call sites eligible for inlining.
	changed := true
	for changed {
		changed = false
		for _, blk := range fn.Blocks {
			for i, stmt := range blk.Stmts {
				call, ok := stmt.(*mir.CallStmt)
				if !ok {
					continue
				}
				glob, ok := call.Func.(*mir.Global)
				if !ok {
					continue
				}

				callee, ok := funcMap[glob.Name]
				if !ok {
					continue
				}

				// Don't inline recursive calls.
				if callee.Name == fn.Name {
					continue
				}

				// Check size limit.
				totalStmts := 0
				for _, cb := range callee.Blocks {
					totalStmts += len(cb.Stmts)
				}
				if totalStmts > p.MaxStmts {
					continue
				}

				// Don't inline functions with complex control flow (multiple blocks).
				// Only inline single-block functions for simplicity.
				reachableBlocks := countReachableBlocks(callee)
				if reachableBlocks > 1 {
					continue
				}

				// Inline the callee.
				inlined := inlineCall(fn, blk, i, call, callee)
				if inlined {
					changed = true
					break // restart scanning this block
				}
			}
		}
	}
}

func countReachableBlocks(fn *mir.Function) int {
	visited := make(map[mir.BlockID]bool)
	var walk func(mir.BlockID)
	walk = func(id mir.BlockID) {
		if visited[id] {
			return
		}
		visited[id] = true
		blk := fn.Block(id)
		if blk.Term != nil {
			for _, succ := range blk.Term.Successors() {
				walk(succ)
			}
		}
	}
	walk(fn.Entry)
	return len(visited)
}

// inlineCall replaces a call statement with the callee's body (single-block case).
func inlineCall(
	caller *mir.Function,
	callBlock *mir.BasicBlock,
	callIdx int,
	call *mir.CallStmt,
	callee *mir.Function,
) bool {
	entry := callee.Block(callee.Entry)

	// Map callee locals to fresh locals in the caller.
	localMap := make(map[mir.LocalID]mir.LocalID)
	for _, def := range callee.Locals {
		newID := caller.NewLocal("inline."+def.Name, def.Type)
		localMap[def.ID] = newID
	}

	// Map parameters to argument values.
	paramAssigns := make([]mir.Stmt, 0, len(callee.Params))
	for i, pid := range callee.Params {
		if i < len(call.Args) {
			newPID := localMap[pid]
			paramAssigns = append(paramAssigns, &mir.Assign{
				Dest: newPID,
				Src:  call.Args[i],
				Type: callee.Locals[int(pid)].Type,
			})
		}
	}

	// Remap values in callee statements.
	remapVal := func(v mir.Value) mir.Value {
		if v == nil {
			return v
		}
		if local, ok := v.(*mir.Local); ok {
			if newID, ok := localMap[local.ID]; ok {
				return &mir.Local{ID: newID, Type: local.Type}
			}
		}
		return v
	}

	// Copy callee statements with remapped locals.
	var inlinedStmts []mir.Stmt
	inlinedStmts = append(inlinedStmts, paramAssigns...)

	for _, stmt := range entry.Stmts {
		copied := copyStmt(stmt, localMap)
		replaceStmtOperands(copied, remapVal)
		inlinedStmts = append(inlinedStmts, copied)
	}

	// If callee returns a value, map it to the call's destination.
	if ret, ok := entry.Term.(*mir.Return); ok && ret.Value != nil {
		retVal := remapVal(ret.Value)
		inlinedStmts = append(inlinedStmts, &mir.Assign{
			Dest: call.Dest,
			Src:  retVal,
			Type: call.Type,
		})
	}

	// Replace the call statement with the inlined statements.
	newStmts := make([]mir.Stmt, 0, len(callBlock.Stmts)+len(inlinedStmts)-1)
	newStmts = append(newStmts, callBlock.Stmts[:callIdx]...)
	newStmts = append(newStmts, inlinedStmts...)
	newStmts = append(newStmts, callBlock.Stmts[callIdx+1:]...)
	callBlock.Stmts = newStmts

	return true
}

// copyStmt creates a shallow copy of a statement with remapped destination.
func copyStmt(stmt mir.Stmt, localMap map[mir.LocalID]mir.LocalID) mir.Stmt {
	remap := func(id mir.LocalID) mir.LocalID {
		if newID, ok := localMap[id]; ok {
			return newID
		}
		return id
	}

	switch s := stmt.(type) {
	case *mir.Assign:
		return &mir.Assign{Dest: remap(s.Dest), Src: s.Src, Type: s.Type}
	case *mir.CallStmt:
		args := make([]mir.Value, len(s.Args))
		copy(args, s.Args)
		return &mir.CallStmt{Dest: remap(s.Dest), Func: s.Func, Args: args, Type: s.Type}
	case *mir.BinaryOpStmt:
		return &mir.BinaryOpStmt{Dest: remap(s.Dest), Op: s.Op, Left: s.Left, Right: s.Right, Type: s.Type}
	case *mir.UnaryOpStmt:
		return &mir.UnaryOpStmt{Dest: remap(s.Dest), Op: s.Op, Operand: s.Operand, Type: s.Type}
	case *mir.FieldAccessStmt:
		return &mir.FieldAccessStmt{Dest: remap(s.Dest), Object: s.Object, Field: s.Field, Type: s.Type}
	case *mir.IndexAccessStmt:
		return &mir.IndexAccessStmt{Dest: remap(s.Dest), Object: s.Object, Index: s.Index, Type: s.Type}
	case *mir.ArrayAllocStmt:
		elems := make([]mir.Value, len(s.Elems))
		copy(elems, s.Elems)
		return &mir.ArrayAllocStmt{Dest: remap(s.Dest), Elems: elems, Type: s.Type}
	case *mir.StructAllocStmt:
		fields := make([]mir.FieldValue, len(s.Fields))
		copy(fields, s.Fields)
		return &mir.StructAllocStmt{Dest: remap(s.Dest), Name: s.Name, Fields: fields, Type: s.Type}
	case *mir.EnumAllocStmt:
		args := make([]mir.Value, len(s.Args))
		copy(args, s.Args)
		return &mir.EnumAllocStmt{Dest: remap(s.Dest), EnumName: s.EnumName, Variant: s.Variant, Args: args, Type: s.Type}
	case *mir.ClosureAllocStmt:
		caps := make([]mir.Value, len(s.Captures))
		copy(caps, s.Captures)
		return &mir.ClosureAllocStmt{Dest: remap(s.Dest), FuncName: s.FuncName, Captures: caps, Type: s.Type}
	case *mir.ChannelCreateStmt:
		return &mir.ChannelCreateStmt{Dest: remap(s.Dest), ElemType: s.ElemType, BufSize: s.BufSize, Type: s.Type}
	case *mir.ChannelSendStmt:
		return &mir.ChannelSendStmt{Chan: s.Chan, SendVal: s.SendVal}
	case *mir.SpawnStmt:
		args := make([]mir.Value, len(s.Args))
		copy(args, s.Args)
		return &mir.SpawnStmt{Dest: remap(s.Dest), Func: s.Func, Args: args, Type: s.Type}
	case *mir.CastStmt:
		return &mir.CastStmt{Dest: remap(s.Dest), Src: s.Src, Target: s.Target, Type: s.Type}
	case *mir.TagCheckStmt: // [CLAUDE-FIX]
		return &mir.TagCheckStmt{Dest: remap(s.Dest), Object: s.Object, EnumName: s.EnumName, Variant: s.Variant, Type: s.Type}
	default:
		return stmt
	}
}
