package optimize

import (
	"fmt"

	"github.com/ryx-lang/ryx/pkg/mir"
)

// CSEPass performs common subexpression elimination within each basic block.
type CSEPass struct{}

func (*CSEPass) Name() string { return "cse" }

func (*CSEPass) Run(fn *mir.Function) {
	for _, blk := range fn.Blocks {
		cseBlock(blk)
	}
}

func cseBlock(blk *mir.BasicBlock) {
	// Map from expression key to the local that holds the result.
	available := make(map[string]mir.LocalID)

	for i, stmt := range blk.Stmts {
		key := stmtKey(stmt)
		if key == "" {
			continue
		}

		if existing, ok := available[key]; ok {
			// Replace with a copy from the existing computation.
			dest := stmt.DestLocal()
			if dest != mir.NoLocal {
				blk.Stmts[i] = &mir.Assign{
					Dest: dest,
					Src:  &mir.Local{ID: existing, Type: stmt.StmtType()},
					Type: stmt.StmtType(),
				}
			}
		} else {
			dest := stmt.DestLocal()
			if dest != mir.NoLocal {
				available[key] = dest
			}
		}
	}
}

// stmtKey returns a canonical string key for a statement's computation.
// Returns "" for non-pure or non-eliminable statements.
func stmtKey(stmt mir.Stmt) string {
	switch s := stmt.(type) {
	case *mir.BinaryOpStmt:
		return fmt.Sprintf("binop:%s:%s:%s", s.Op, valueKey(s.Left), valueKey(s.Right))
	case *mir.UnaryOpStmt:
		return fmt.Sprintf("unop:%s:%s", s.Op, valueKey(s.Operand))
	case *mir.FieldAccessStmt:
		return fmt.Sprintf("field:%s:%s", valueKey(s.Object), s.Field)
	case *mir.IndexAccessStmt:
		return fmt.Sprintf("index:%s:%s", valueKey(s.Object), valueKey(s.Index))
	default:
		return "" // calls, allocs, etc. are not pure
	}
}

func valueKey(v mir.Value) string {
	if v == nil {
		return "nil"
	}
	switch val := v.(type) {
	case *mir.Local:
		return fmt.Sprintf("%%%d", val.ID)
	case *mir.Const:
		return val.String()
	case *mir.Global:
		return fmt.Sprintf("@%s", val.Name)
	default:
		return fmt.Sprintf("?%T", v)
	}
}
