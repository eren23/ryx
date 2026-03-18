package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// CopyPropPass replaces uses of copy assignments (x = y) with the original value.
type CopyPropPass struct{}

func (*CopyPropPass) Name() string { return "copy-prop" }

func (*CopyPropPass) Run(fn *mir.Function) {
	// Build a map from local -> its copy source, chasing through chains.
	copies := make(map[mir.LocalID]mir.Value)

	for _, blk := range fn.Blocks {
		for _, stmt := range blk.Stmts {
			if a, ok := stmt.(*mir.Assign); ok {
				copies[a.Dest] = a.Src
			}
		}
	}

	// Resolve copy chains: if x=y and y=z, resolve x to z.
	resolve := func(v mir.Value) mir.Value {
		for i := 0; i < 100; i++ { // bound to prevent cycles
			local, ok := v.(*mir.Local)
			if !ok {
				return v
			}
			src, ok := copies[local.ID]
			if !ok {
				return v
			}
			v = src
		}
		return v
	}

	// Replace all uses.
	replaceValue := func(v mir.Value) mir.Value {
		return resolve(v)
	}

	for _, blk := range fn.Blocks {
		for _, phi := range blk.Phis {
			for bid, v := range phi.Args {
				phi.Args[bid] = replaceValue(v)
			}
		}

		for _, stmt := range blk.Stmts {
			replaceStmtOperands(stmt, replaceValue)
		}

		replaceTermOperands(blk.Term, replaceValue)
	}
}

// replaceStmtOperands replaces all value operands in a statement.
func replaceStmtOperands(stmt mir.Stmt, replace func(mir.Value) mir.Value) {
	switch s := stmt.(type) {
	case *mir.Assign:
		s.Src = replace(s.Src)
	case *mir.CallStmt:
		s.Func = replace(s.Func)
		for i := range s.Args {
			s.Args[i] = replace(s.Args[i])
		}
	case *mir.BinaryOpStmt:
		s.Left = replace(s.Left)
		s.Right = replace(s.Right)
	case *mir.UnaryOpStmt:
		s.Operand = replace(s.Operand)
	case *mir.FieldAccessStmt:
		s.Object = replace(s.Object)
	case *mir.IndexAccessStmt:
		s.Object = replace(s.Object)
		s.Index = replace(s.Index)
	case *mir.ArrayAllocStmt:
		for i := range s.Elems {
			s.Elems[i] = replace(s.Elems[i])
		}
	case *mir.StructAllocStmt:
		for i := range s.Fields {
			s.Fields[i].Value = replace(s.Fields[i].Value)
		}
	case *mir.EnumAllocStmt:
		for i := range s.Args {
			s.Args[i] = replace(s.Args[i])
		}
	case *mir.ClosureAllocStmt:
		for i := range s.Captures {
			s.Captures[i] = replace(s.Captures[i])
		}
	case *mir.ChannelCreateStmt:
		if s.BufSize != nil {
			s.BufSize = replace(s.BufSize)
		}
	case *mir.ChannelSendStmt:
		s.Chan = replace(s.Chan)
		s.SendVal = replace(s.SendVal)
	case *mir.SpawnStmt:
		s.Func = replace(s.Func)
		for i := range s.Args {
			s.Args[i] = replace(s.Args[i])
		}
	case *mir.CastStmt:
		s.Src = replace(s.Src)
	case *mir.TagCheckStmt: // [CLAUDE-FIX]
		s.Object = replace(s.Object)
	}
}

// replaceTermOperands replaces all value operands in a terminator.
func replaceTermOperands(term mir.Terminator, replace func(mir.Value) mir.Value) {
	switch t := term.(type) {
	case *mir.Branch:
		t.Cond = replace(t.Cond)
	case *mir.Switch:
		t.Scrutinee = replace(t.Scrutinee)
	case *mir.Return:
		if t.Value != nil {
			t.Value = replace(t.Value)
		}
	}
}
