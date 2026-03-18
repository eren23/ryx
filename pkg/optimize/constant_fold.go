package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// ConstantFoldPass evaluates constant expressions at compile time.
type ConstantFoldPass struct{}

func (*ConstantFoldPass) Name() string { return "constant-fold" }

func (*ConstantFoldPass) Run(fn *mir.Function) {
	for _, blk := range fn.Blocks {
		for i, stmt := range blk.Stmts {
			if folded := tryFold(stmt); folded != nil {
				blk.Stmts[i] = folded
			}
		}
	}
}

func tryFold(stmt mir.Stmt) mir.Stmt {
	switch s := stmt.(type) {
	case *mir.BinaryOpStmt:
		return tryFoldBinary(s)
	case *mir.UnaryOpStmt:
		return tryFoldUnary(s)
	default:
		return nil
	}
}

func tryFoldBinary(s *mir.BinaryOpStmt) mir.Stmt {
	lc, lok := s.Left.(*mir.Const)
	rc, rok := s.Right.(*mir.Const)

	// Both operands constant: evaluate at compile time.
	if lok && rok {
		// Integer operations.
		if lc.Kind == mir.ConstInt && rc.Kind == mir.ConstInt {
			l, r := lc.Int, rc.Int
			var result *mir.Const
			switch s.Op {
			case "+":
				result = mir.IntConst(l + r)
			case "-":
				result = mir.IntConst(l - r)
			case "*":
				result = mir.IntConst(l * r)
			case "/":
				if r != 0 {
					result = mir.IntConst(l / r)
				}
			case "%":
				if r != 0 {
					result = mir.IntConst(l % r)
				}
			case "==":
				result = mir.BoolConst(l == r)
			case "!=":
				result = mir.BoolConst(l != r)
			case "<":
				result = mir.BoolConst(l < r)
			case "<=":
				result = mir.BoolConst(l <= r)
			case ">":
				result = mir.BoolConst(l > r)
			case ">=":
				result = mir.BoolConst(l >= r)
			}
			if result != nil {
				return &mir.Assign{Dest: s.Dest, Src: result, Type: s.Type}
			}
		}

		// Float operations.
		if lc.Kind == mir.ConstFloat && rc.Kind == mir.ConstFloat {
			l, r := lc.Float, rc.Float
			var result *mir.Const
			switch s.Op {
			case "+":
				result = mir.FloatConst(l + r)
			case "-":
				result = mir.FloatConst(l - r)
			case "*":
				result = mir.FloatConst(l * r)
			case "/":
				if r != 0 {
					result = mir.FloatConst(l / r)
				}
			case "==":
				result = mir.BoolConst(l == r)
			case "!=":
				result = mir.BoolConst(l != r)
			case "<":
				result = mir.BoolConst(l < r)
			case "<=":
				result = mir.BoolConst(l <= r)
			case ">":
				result = mir.BoolConst(l > r)
			case ">=":
				result = mir.BoolConst(l >= r)
			}
			if result != nil {
				return &mir.Assign{Dest: s.Dest, Src: result, Type: s.Type}
			}
		}

		// Boolean operations.
		if lc.Kind == mir.ConstBool && rc.Kind == mir.ConstBool {
			l, r := lc.Bool, rc.Bool
			var result *mir.Const
			switch s.Op {
			case "&&":
				result = mir.BoolConst(l && r)
			case "||":
				result = mir.BoolConst(l || r)
			case "==":
				result = mir.BoolConst(l == r)
			case "!=":
				result = mir.BoolConst(l != r)
			}
			if result != nil {
				return &mir.Assign{Dest: s.Dest, Src: result, Type: s.Type}
			}
		}
	}

	// Identity simplifications with one constant operand.
	if lok && lc.Kind == mir.ConstInt {
		switch s.Op {
		case "+":
			if lc.Int == 0 {
				return &mir.Assign{Dest: s.Dest, Src: s.Right, Type: s.Type}
			}
		case "*":
			if lc.Int == 0 {
				return &mir.Assign{Dest: s.Dest, Src: mir.IntConst(0), Type: s.Type}
			}
			if lc.Int == 1 {
				return &mir.Assign{Dest: s.Dest, Src: s.Right, Type: s.Type}
			}
		}
	}
	if rok && rc.Kind == mir.ConstInt {
		switch s.Op {
		case "+", "-":
			if rc.Int == 0 {
				return &mir.Assign{Dest: s.Dest, Src: s.Left, Type: s.Type}
			}
		case "*":
			if rc.Int == 0 {
				return &mir.Assign{Dest: s.Dest, Src: mir.IntConst(0), Type: s.Type}
			}
			if rc.Int == 1 {
				return &mir.Assign{Dest: s.Dest, Src: s.Left, Type: s.Type}
			}
		case "/":
			if rc.Int == 1 {
				return &mir.Assign{Dest: s.Dest, Src: s.Left, Type: s.Type}
			}
		}
	}

	// Boolean simplifications with one constant operand.
	if lok && lc.Kind == mir.ConstBool {
		switch s.Op {
		case "&&":
			if !lc.Bool {
				return &mir.Assign{Dest: s.Dest, Src: mir.BoolConst(false), Type: s.Type}
			}
			return &mir.Assign{Dest: s.Dest, Src: s.Right, Type: s.Type}
		case "||":
			if lc.Bool {
				return &mir.Assign{Dest: s.Dest, Src: mir.BoolConst(true), Type: s.Type}
			}
			return &mir.Assign{Dest: s.Dest, Src: s.Right, Type: s.Type}
		}
	}

	return nil
}

func tryFoldUnary(s *mir.UnaryOpStmt) mir.Stmt {
	c, ok := s.Operand.(*mir.Const)
	if !ok {
		return nil
	}

	switch s.Op {
	case "-":
		if c.Kind == mir.ConstInt {
			return &mir.Assign{Dest: s.Dest, Src: mir.IntConst(-c.Int), Type: s.Type}
		}
		if c.Kind == mir.ConstFloat {
			return &mir.Assign{Dest: s.Dest, Src: mir.FloatConst(-c.Float), Type: s.Type}
		}
	case "!":
		if c.Kind == mir.ConstBool {
			return &mir.Assign{Dest: s.Dest, Src: mir.BoolConst(!c.Bool), Type: s.Type}
		}
	}

	return nil
}
