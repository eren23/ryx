package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// TCOPass transforms self-tail-recursive calls into loops.
// Only handles direct self-recursion where the last operation
// before return is a call to the same function.
type TCOPass struct{}

func (*TCOPass) Name() string { return "tco" }

func (*TCOPass) Run(fn *mir.Function) {
	// Find blocks that end with: call self(args...) -> %r; return %r.
	// Transform to: assign args to params; goto header.

	tailCalls := findTailCalls(fn)
	if len(tailCalls) == 0 {
		return
	}

	// Create a loop header block that the entry block jumps to.
	// Move the entry block's content to the header.
	headerID := fn.NewBlock("tco.header")
	header := fn.Block(headerID)

	entry := fn.Block(fn.Entry)

	// Copy entry's content to header.
	header.Phis = entry.Phis
	header.Stmts = entry.Stmts
	header.Term = entry.Term
	header.Succs = entry.Succs

	// Entry becomes a simple goto to header.
	entry.Phis = nil
	entry.Stmts = nil
	entry.Term = &mir.Goto{Target: headerID}
	entry.Succs = []mir.BlockID{headerID}
	header.Preds = append(header.Preds, fn.Entry)

	// Update predecessor references: blocks that pointed to entry as pred
	// now point to header.
	for _, blk := range fn.Blocks {
		if blk.ID == fn.Entry || blk.ID == headerID {
			continue
		}
		for i, pred := range blk.Preds {
			if pred == fn.Entry {
				blk.Preds[i] = headerID
			}
		}
		// Update successor references pointing to entry.
		for i, succ := range blk.Succs {
			if succ == fn.Entry {
				blk.Succs[i] = headerID
			}
		}
		// Update terminator block references.
		rewriteTermBlockIDSingle(blk, fn.Entry, headerID)
	}

	// Now transform each tail call site.
	for _, tc := range tailCalls {
		blk := fn.Block(tc.block)

		// Remove the call statement and return.
		// Replace with: assign args to param locals; goto header.
		var newStmts []mir.Stmt

		// Keep all stmts before the call.
		for _, s := range blk.Stmts {
			if s == tc.call {
				break
			}
			newStmts = append(newStmts, s)
		}

		// Assign call arguments to parameter locals.
		for i, arg := range tc.call.Args {
			if i < len(fn.Params) {
				paramID := fn.Params[i]
				newStmts = append(newStmts, &mir.Assign{
					Dest: paramID,
					Src:  arg,
					Type: fn.Locals[int(paramID)].Type,
				})
			}
		}

		blk.Stmts = newStmts
		blk.Term = &mir.Goto{Target: headerID}

		// Update edges.
		// Remove old successors.
		blk.Succs = []mir.BlockID{headerID}
		header.Preds = append(header.Preds, blk.ID)
	}
}

type tailCallInfo struct {
	block mir.BlockID
	call  *mir.CallStmt
}

func findTailCalls(fn *mir.Function) []tailCallInfo {
	var result []tailCallInfo

	for _, blk := range fn.Blocks {
		ret, ok := blk.Term.(*mir.Return)
		if !ok || ret.Value == nil {
			continue
		}

		// Check if the return value is the result of a self-call.
		retLocal, ok := ret.Value.(*mir.Local)
		if !ok {
			continue
		}

		// Find the statement that defines the returned local.
		var callStmt *mir.CallStmt
		for _, stmt := range blk.Stmts {
			if call, ok := stmt.(*mir.CallStmt); ok && call.Dest == retLocal.ID {
				callStmt = call
			}
		}
		if callStmt == nil {
			continue
		}

		// Check if it's a self-call.
		glob, ok := callStmt.Func.(*mir.Global)
		if !ok || glob.Name != fn.Name {
			continue
		}

		// Ensure the call is the last significant statement (no side effects after it).
		foundCall := false
		isLast := true
		for _, stmt := range blk.Stmts {
			if stmt == callStmt {
				foundCall = true
				continue
			}
			if foundCall && hasSideEffect(stmt) {
				isLast = false
				break
			}
		}
		if !isLast {
			continue
		}

		result = append(result, tailCallInfo{block: blk.ID, call: callStmt})
	}

	return result
}

func rewriteTermBlockIDSingle(blk *mir.BasicBlock, from, to mir.BlockID) {
	switch t := blk.Term.(type) {
	case *mir.Goto:
		if t.Target == from {
			t.Target = to
		}
	case *mir.Branch:
		if t.Then == from {
			t.Then = to
		}
		if t.Else == from {
			t.Else = to
		}
	case *mir.Switch:
		for i := range t.Cases {
			if t.Cases[i].Target == from {
				t.Cases[i].Target = to
			}
		}
		if t.Default == from {
			t.Default = to
		}
	}
}
