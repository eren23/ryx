package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// ConstPropPass propagates constant values through assignments and phi nodes.
type ConstPropPass struct{}

func (*ConstPropPass) Name() string { return "const-prop" }

func (*ConstPropPass) Run(fn *mir.Function) {
	// Build a map from local to constant value.
	constants := make(map[mir.LocalID]*mir.Const)

	// Iterate to fixed point.
	changed := true
	for changed {
		changed = false

		for _, blk := range fn.Blocks {
			// Check phi nodes — if all incoming values are the same constant.
			for _, phi := range blk.Phis {
				if _, already := constants[phi.Dest]; already {
					continue
				}
				c := phiConstant(phi, constants)
				if c != nil {
					constants[phi.Dest] = c
					changed = true
				}
			}

			for _, stmt := range blk.Stmts {
				switch s := stmt.(type) {
				case *mir.Assign:
					if c, ok := s.Src.(*mir.Const); ok {
						if _, already := constants[s.Dest]; !already {
							constants[s.Dest] = c
							changed = true
						}
					} else if local, ok := s.Src.(*mir.Local); ok {
						if c, ok := constants[local.ID]; ok {
							if _, already := constants[s.Dest]; !already {
								constants[s.Dest] = c
								changed = true
							}
						}
					}
				}
			}
		}
	}

	if len(constants) == 0 {
		return
	}

	// Replace uses of known constants.
	replaceValue := func(v mir.Value) mir.Value {
		if local, ok := v.(*mir.Local); ok {
			if c, ok := constants[local.ID]; ok {
				return c
			}
		}
		return v
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

	// Simplify branches on known constants.
	for _, blk := range fn.Blocks {
		if br, ok := blk.Term.(*mir.Branch); ok {
			if c, ok := br.Cond.(*mir.Const); ok && c.Kind == mir.ConstBool {
				if c.Bool {
					blk.Term = &mir.Goto{Target: br.Then}
					// Remove else edge.
					removeSuccEdge(fn, blk.ID, br.Else)
				} else {
					blk.Term = &mir.Goto{Target: br.Else}
					// Remove then edge.
					removeSuccEdge(fn, blk.ID, br.Then)
				}
			}
		}
	}
}

// phiConstant returns a constant if all phi args resolve to the same constant.
func phiConstant(phi *mir.Phi, constants map[mir.LocalID]*mir.Const) *mir.Const {
	var result *mir.Const
	for _, v := range phi.Args {
		var c *mir.Const
		if cv, ok := v.(*mir.Const); ok {
			c = cv
		} else if local, ok := v.(*mir.Local); ok {
			c = constants[local.ID]
		}
		if c == nil {
			return nil // not a constant
		}
		if result == nil {
			result = c
		} else if !constEqual(result, c) {
			return nil // different constants
		}
	}
	return result
}

func constEqual(a, b *mir.Const) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case mir.ConstInt:
		return a.Int == b.Int
	case mir.ConstFloat:
		return a.Float == b.Float
	case mir.ConstBool:
		return a.Bool == b.Bool
	case mir.ConstString, mir.ConstChar:
		return a.Str == b.Str
	case mir.ConstUnit:
		return true
	default:
		return false
	}
}

func removeSuccEdge(fn *mir.Function, from, to mir.BlockID) {
	srcBlk := fn.Block(from)
	newSuccs := make([]mir.BlockID, 0, len(srcBlk.Succs))
	for _, s := range srcBlk.Succs {
		if s != to {
			newSuccs = append(newSuccs, s)
		}
	}
	srcBlk.Succs = newSuccs

	dstBlk := fn.Block(to)
	newPreds := make([]mir.BlockID, 0, len(dstBlk.Preds))
	for _, p := range dstBlk.Preds {
		if p != from {
			newPreds = append(newPreds, p)
		}
	}
	dstBlk.Preds = newPreds
}
