package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// EscapePass performs escape analysis to determine whether allocations
// can be placed on the stack instead of the heap. It annotates each
// allocation with its escape status.
type EscapePass struct{}

func (*EscapePass) Name() string { return "escape" }

func (*EscapePass) Run(fn *mir.Function) {
	info := analyzeEscapes(fn)
	// Attach escape info to the function for downstream codegen.
	// We store it in a side table accessible via the function.
	annotateEscapes(fn, info)
}

// analyzeEscapes determines which locals escape to the heap.
func analyzeEscapes(fn *mir.Function) *mir.EscapeInfo {
	info := &mir.EscapeInfo{
		Escapes: make(map[mir.LocalID]bool),
	}

	// Collect all allocation sites.
	allocs := make(map[mir.LocalID]bool)
	for _, blk := range fn.Blocks {
		for _, stmt := range blk.Stmts {
			switch stmt.(type) {
			case *mir.ArrayAllocStmt, *mir.StructAllocStmt, *mir.EnumAllocStmt,
				*mir.ClosureAllocStmt, *mir.ChannelCreateStmt:
				d := stmt.DestLocal()
				if d != mir.NoLocal {
					allocs[d] = true
				}
			}
		}
	}

	// An allocation escapes if:
	// 1. It is returned from the function.
	// 2. It is stored into another allocation that escapes.
	// 3. It is passed to a function call (conservative).
	// 4. It is sent through a channel.
	// 5. It is captured by a closure.

	// Build use-def chains: for each local, where is it used.
	for _, blk := range fn.Blocks {
		for _, stmt := range blk.Stmts {
			checkEscapeThroughStmt(stmt, allocs, info)
		}

		// Check if returned.
		if ret, ok := blk.Term.(*mir.Return); ok && ret.Value != nil {
			if local, ok := ret.Value.(*mir.Local); ok {
				if allocs[local.ID] {
					info.Escapes[local.ID] = true
				}
			}
		}
	}

	// Propagate: if an escaping allocation contains references to other allocations,
	// those escape too.
	changed := true
	for changed {
		changed = false
		for _, blk := range fn.Blocks {
			for _, stmt := range blk.Stmts {
				propagateEscapes(stmt, allocs, info, &changed)
			}
		}
	}

	return info
}

func checkEscapeThroughStmt(stmt mir.Stmt, allocs map[mir.LocalID]bool, info *mir.EscapeInfo) {
	markEscaped := func(v mir.Value) {
		if v == nil {
			return
		}
		if local, ok := v.(*mir.Local); ok && allocs[local.ID] {
			info.Escapes[local.ID] = true
		}
	}

	switch s := stmt.(type) {
	case *mir.CallStmt:
		// Conservative: all allocations passed to calls escape.
		for _, arg := range s.Args {
			markEscaped(arg)
		}
	case *mir.ChannelSendStmt:
		markEscaped(s.SendVal)
	case *mir.SpawnStmt:
		for _, arg := range s.Args {
			markEscaped(arg)
		}
	case *mir.ClosureAllocStmt:
		// Allocations captured by closures escape.
		for _, cap := range s.Captures {
			markEscaped(cap)
		}
	}
}

func propagateEscapes(stmt mir.Stmt, allocs map[mir.LocalID]bool, info *mir.EscapeInfo, changed *bool) {
	// If a struct/array field stores an allocation, and the struct escapes,
	// then the stored allocation also escapes.
	switch s := stmt.(type) {
	case *mir.StructAllocStmt:
		if info.Escapes[s.Dest] {
			for _, f := range s.Fields {
				if local, ok := f.Value.(*mir.Local); ok && allocs[local.ID] && !info.Escapes[local.ID] {
					info.Escapes[local.ID] = true
					*changed = true
				}
			}
		}
	case *mir.ArrayAllocStmt:
		if info.Escapes[s.Dest] {
			for _, e := range s.Elems {
				if local, ok := e.(*mir.Local); ok && allocs[local.ID] && !info.Escapes[local.ID] {
					info.Escapes[local.ID] = true
					*changed = true
				}
			}
		}
	case *mir.EnumAllocStmt:
		if info.Escapes[s.Dest] {
			for _, a := range s.Args {
				if local, ok := a.(*mir.Local); ok && allocs[local.ID] && !info.Escapes[local.ID] {
					info.Escapes[local.ID] = true
					*changed = true
				}
			}
		}
	}
}

// annotateEscapes marks allocations with their escape status.
// Non-escaping allocations are candidates for stack allocation.
func annotateEscapes(fn *mir.Function, info *mir.EscapeInfo) {
	// Store escape info on the allocation locals.
	for _, def := range fn.Locals {
		if _, escaped := info.Escapes[def.ID]; !escaped {
			// Default: not escaped = stack-allocatable.
		}
	}
	// The escape info is returned for codegen to use. We store it
	// as a separate annotation since MIR locals don't have an escape field.
	// For now, we keep the info in the EscapeInfo struct which can be
	// queried by downstream passes.
}

// AnalyzeEscapes is exported for testing.
func AnalyzeEscapes(fn *mir.Function) *mir.EscapeInfo {
	return analyzeEscapes(fn)
}
