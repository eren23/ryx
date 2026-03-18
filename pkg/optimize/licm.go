package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// LICMPass performs loop-invariant code motion.
// It identifies computations inside loops that don't depend on the loop
// iteration and hoists them to the loop preheader.
type LICMPass struct{}

func (*LICMPass) Name() string { return "licm" }

func (*LICMPass) Run(fn *mir.Function) {
	loops := detectLoops(fn)
	for _, loop := range loops {
		hoistInvariants(fn, loop)
	}
}

// loopInfo describes a natural loop in the CFG.
type loopInfo struct {
	header    mir.BlockID
	blocks    map[mir.BlockID]bool
	preheader mir.BlockID // the single predecessor of header outside the loop
}

// detectLoops finds natural loops by identifying back edges.
func detectLoops(fn *mir.Function) []loopInfo {
	// Compute dominators using a simple iterative algorithm.
	doms := computeDominators(fn)

	// Find back edges: edges where the target dominates the source.
	var loops []loopInfo
	for _, blk := range fn.Blocks {
		if blk.Term == nil {
			continue
		}
		for _, succ := range blk.Term.Successors() {
			if dominates(doms, succ, blk.ID) {
				// Back edge: blk -> succ. succ is the loop header.
				loop := collectLoop(fn, succ, blk.ID)
				loops = append(loops, loop)
			}
		}
	}

	return loops
}

// computeDominators returns a map from block ID to its immediate dominator.
func computeDominators(fn *mir.Function) map[mir.BlockID]mir.BlockID {
	idom := make(map[mir.BlockID]mir.BlockID)
	idom[fn.Entry] = fn.Entry

	changed := true
	for changed {
		changed = false
		for _, blk := range fn.Blocks {
			if blk.ID == fn.Entry {
				continue
			}
			var newIdom mir.BlockID = -1
			for _, pred := range blk.Preds {
				if _, ok := idom[pred]; !ok {
					continue
				}
				if newIdom == -1 {
					newIdom = pred
				} else {
					newIdom = intersect(idom, newIdom, pred, fn.Entry)
				}
			}
			if newIdom != -1 {
				if old, ok := idom[blk.ID]; !ok || old != newIdom {
					idom[blk.ID] = newIdom
					changed = true
				}
			}
		}
	}

	return idom
}

func intersect(idom map[mir.BlockID]mir.BlockID, a, b, entry mir.BlockID) mir.BlockID {
	fa, fb := a, b
	for fa != fb {
		for fa > fb {
			if fa == entry {
				break
			}
			fa = idom[fa]
		}
		for fb > fa {
			if fb == entry {
				break
			}
			fb = idom[fb]
		}
	}
	return fa
}

func dominates(idom map[mir.BlockID]mir.BlockID, a, b mir.BlockID) bool {
	cur := b
	for i := 0; i < 1000; i++ { // bound to prevent infinite loop
		if cur == a {
			return true
		}
		dom, ok := idom[cur]
		if !ok || dom == cur {
			return false
		}
		cur = dom
	}
	return false
}

// collectLoop collects all blocks in the natural loop with the given header and latch.
func collectLoop(fn *mir.Function, header, latch mir.BlockID) loopInfo {
	blocks := map[mir.BlockID]bool{header: true}
	// Traverse backwards from latch to find all blocks that can reach latch
	// without going through header.
	var stack []mir.BlockID
	if latch != header {
		blocks[latch] = true
		stack = append(stack, latch)
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, pred := range fn.Block(n).Preds {
			if !blocks[pred] {
				blocks[pred] = true
				stack = append(stack, pred)
			}
		}
	}

	// Find preheader: a predecessor of header that is outside the loop.
	var preheader mir.BlockID = -1
	for _, pred := range fn.Block(header).Preds {
		if !blocks[pred] {
			preheader = pred
			break
		}
	}

	return loopInfo{
		header:    header,
		blocks:    blocks,
		preheader: preheader,
	}
}

// hoistInvariants moves loop-invariant instructions to the preheader.
func hoistInvariants(fn *mir.Function, loop loopInfo) {
	if loop.preheader == -1 {
		return // no preheader to hoist to
	}

	// Collect locals defined inside the loop.
	loopDefs := make(map[mir.LocalID]bool)
	for bid := range loop.blocks {
		blk := fn.Block(bid)
		for _, phi := range blk.Phis {
			loopDefs[phi.Dest] = true
		}
		for _, stmt := range blk.Stmts {
			if d := stmt.DestLocal(); d != mir.NoLocal {
				loopDefs[d] = true
			}
		}
	}

	// A value is loop-invariant if it's a constant, global, or defined outside the loop.
	isInvariant := func(v mir.Value) bool {
		if v == nil {
			return true
		}
		switch val := v.(type) {
		case *mir.Const, *mir.Global:
			return true
		case *mir.Local:
			return !loopDefs[val.ID]
		default:
			return false
		}
	}

	isInvariantStmt := func(stmt mir.Stmt) bool {
		if hasSideEffect(stmt) {
			return false
		}
		switch s := stmt.(type) {
		case *mir.BinaryOpStmt:
			return isInvariant(s.Left) && isInvariant(s.Right)
		case *mir.UnaryOpStmt:
			return isInvariant(s.Operand)
		case *mir.Assign:
			return isInvariant(s.Src)
		case *mir.FieldAccessStmt:
			return isInvariant(s.Object)
		case *mir.IndexAccessStmt:
			return isInvariant(s.Object) && isInvariant(s.Index)
		default:
			return false
		}
	}

	preBlk := fn.Block(loop.preheader)

	// Iterate: hoist invariant instructions and update loop defs.
	changed := true
	for changed {
		changed = false
		for bid := range loop.blocks {
			blk := fn.Block(bid)
			var remaining []mir.Stmt
			for _, stmt := range blk.Stmts {
				if isInvariantStmt(stmt) {
					// Hoist to preheader (before the terminator).
					preBlk.Stmts = append(preBlk.Stmts, stmt)
					if d := stmt.DestLocal(); d != mir.NoLocal {
						delete(loopDefs, d)
					}
					changed = true
				} else {
					remaining = append(remaining, stmt)
				}
			}
			blk.Stmts = remaining
		}
	}
}
