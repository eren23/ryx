package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// DeadCodePass removes unused definitions and unreachable blocks.
type DeadCodePass struct{}

func (*DeadCodePass) Name() string { return "dead-code" }

func (*DeadCodePass) Run(fn *mir.Function) {
	removeUnreachableBlocks(fn)
	removeDeadStmts(fn)
}

// removeUnreachableBlocks removes blocks not reachable from the entry block.
func removeUnreachableBlocks(fn *mir.Function) {
	reachable := make(map[mir.BlockID]bool)
	var walk func(mir.BlockID)
	walk = func(id mir.BlockID) {
		if reachable[id] {
			return
		}
		reachable[id] = true
		blk := fn.Block(id)
		if blk.Term != nil {
			for _, succ := range blk.Term.Successors() {
				walk(succ)
			}
		}
	}
	walk(fn.Entry)

	// Remap block IDs.
	oldToNew := make(map[mir.BlockID]mir.BlockID)
	var newBlocks []*mir.BasicBlock
	for _, blk := range fn.Blocks {
		if reachable[blk.ID] {
			newID := mir.BlockID(len(newBlocks))
			oldToNew[blk.ID] = newID
			blk.ID = newID
			newBlocks = append(newBlocks, blk)
		}
	}

	if len(newBlocks) == len(fn.Blocks) {
		return // no blocks removed
	}

	fn.Blocks = newBlocks
	fn.Entry = oldToNew[fn.Entry]

	// Rewrite all block references.
	for _, blk := range fn.Blocks {
		// Rewrite preds/succs.
		blk.Preds = remapBlockIDs(blk.Preds, oldToNew)
		blk.Succs = remapBlockIDs(blk.Succs, oldToNew)

		// Rewrite phi args.
		for _, phi := range blk.Phis {
			newArgs := make(map[mir.BlockID]mir.Value)
			for bid, val := range phi.Args {
				if newBid, ok := oldToNew[bid]; ok {
					newArgs[newBid] = val
				}
			}
			phi.Args = newArgs
		}

		// Rewrite terminators.
		rewriteTermBlockIDs(blk, oldToNew)
	}
}

func remapBlockIDs(ids []mir.BlockID, m map[mir.BlockID]mir.BlockID) []mir.BlockID {
	result := make([]mir.BlockID, 0, len(ids))
	for _, id := range ids {
		if newID, ok := m[id]; ok {
			result = append(result, newID)
		}
	}
	return result
}

func rewriteTermBlockIDs(blk *mir.BasicBlock, m map[mir.BlockID]mir.BlockID) {
	switch t := blk.Term.(type) {
	case *mir.Goto:
		if newID, ok := m[t.Target]; ok {
			t.Target = newID
		}
	case *mir.Branch:
		if newID, ok := m[t.Then]; ok {
			t.Then = newID
		}
		if newID, ok := m[t.Else]; ok {
			t.Else = newID
		}
	case *mir.Switch:
		for i := range t.Cases {
			if newID, ok := m[t.Cases[i].Target]; ok {
				t.Cases[i].Target = newID
			}
		}
		if newID, ok := m[t.Default]; ok {
			t.Default = newID
		}
	}
}

// removeDeadStmts removes statements whose results are never used.
func removeDeadStmts(fn *mir.Function) {
	// Collect all used locals.
	used := make(map[mir.LocalID]bool)

	// Mark params as used.
	for _, p := range fn.Params {
		used[p] = true
	}

	var markValue func(mir.Value)
	markValue = func(v mir.Value) {
		if v == nil {
			return
		}
		if local, ok := v.(*mir.Local); ok {
			used[local.ID] = true
		}
	}

	markStmtOperands := func(stmt mir.Stmt) {
		switch s := stmt.(type) {
		case *mir.Assign:
			markValue(s.Src)
		case *mir.CallStmt:
			markValue(s.Func)
			for _, a := range s.Args {
				markValue(a)
			}
		case *mir.BinaryOpStmt:
			markValue(s.Left)
			markValue(s.Right)
		case *mir.UnaryOpStmt:
			markValue(s.Operand)
		case *mir.FieldAccessStmt:
			markValue(s.Object)
		case *mir.IndexAccessStmt:
			markValue(s.Object)
			markValue(s.Index)
		case *mir.ArrayAllocStmt:
			for _, e := range s.Elems {
				markValue(e)
			}
		case *mir.StructAllocStmt:
			for _, f := range s.Fields {
				markValue(f.Value)
			}
		case *mir.EnumAllocStmt:
			for _, a := range s.Args {
				markValue(a)
			}
		case *mir.ClosureAllocStmt:
			for _, c := range s.Captures {
				markValue(c)
			}
		case *mir.ChannelCreateStmt:
			markValue(s.BufSize)
		case *mir.ChannelSendStmt:
			markValue(s.Chan)
			markValue(s.SendVal)
		case *mir.ChannelRecvStmt:
			markValue(s.Chan)
		case *mir.ChannelCloseStmt:
			markValue(s.Chan)
		case *mir.SpawnStmt:
			markValue(s.Func)
			for _, a := range s.Args {
				markValue(a)
			}
		case *mir.CastStmt:
			markValue(s.Src)
		case *mir.TagCheckStmt: // [CLAUDE-FIX]
			markValue(s.Object)
		}
	}

	// Iterate to fixed point.
	for {
		prevCount := len(used)

		for _, blk := range fn.Blocks {
			for _, phi := range blk.Phis {
				if used[phi.Dest] {
					for _, v := range phi.Args {
						markValue(v)
					}
				}
			}
			for _, stmt := range blk.Stmts {
				dest := stmt.DestLocal()
				if dest == mir.NoLocal || used[dest] || hasSideEffect(stmt) {
					markStmtOperands(stmt)
					if dest != mir.NoLocal {
						used[dest] = true
					}
				}
			}
			// Mark values used by terminators.
			switch t := blk.Term.(type) {
			case *mir.Branch:
				markValue(t.Cond)
			case *mir.Switch:
				markValue(t.Scrutinee)
			case *mir.Return:
				markValue(t.Value)
			}
		}

		if len(used) == prevCount {
			break
		}
	}

	// Remove unused statements.
	for _, blk := range fn.Blocks {
		var newStmts []mir.Stmt
		for _, stmt := range blk.Stmts {
			dest := stmt.DestLocal()
			if dest == mir.NoLocal || used[dest] || hasSideEffect(stmt) {
				newStmts = append(newStmts, stmt)
			}
		}
		blk.Stmts = newStmts

		// Remove unused phi nodes.
		var newPhis []*mir.Phi
		for _, phi := range blk.Phis {
			if used[phi.Dest] {
				newPhis = append(newPhis, phi)
			}
		}
		blk.Phis = newPhis
	}
}

func hasSideEffect(stmt mir.Stmt) bool {
	switch stmt.(type) {
	case *mir.CallStmt, *mir.ChannelSendStmt, *mir.ChannelRecvStmt, *mir.ChannelCloseStmt, *mir.SpawnStmt, *mir.ChannelCreateStmt:
		return true
	default:
		return false
	}
}
