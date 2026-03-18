package mir

import (
	"fmt"
	"sort"
	"strings"
)

// Print returns a human-readable textual representation of a MIR program.
func Print(prog *Program) string {
	var b strings.Builder
	p := &printer{w: &b}
	p.printProgram(prog)
	return b.String()
}

// PrintFunction returns a human-readable textual representation of a single MIR function.
func PrintFunction(fn *Function) string {
	var b strings.Builder
	p := &printer{w: &b}
	p.printFunction(fn)
	return b.String()
}

type printer struct {
	w *strings.Builder
}

func (p *printer) printf(format string, args ...any) {
	fmt.Fprintf(p.w, format, args...)
}

func (p *printer) printProgram(prog *Program) {
	for i, s := range prog.Structs {
		if i > 0 {
			p.printf("\n")
		}
		p.printf("struct %s {\n", s.Name)
		for _, f := range s.Fields {
			p.printf("  %s: %s\n", f.Name, f.Type.String())
		}
		p.printf("}\n")
	}

	for _, e := range prog.Enums {
		p.printf("\nenum %s {\n", e.Name)
		for _, v := range e.Variants {
			if len(v.Fields) == 0 {
				p.printf("  %s\n", v.Name)
			} else {
				fields := make([]string, len(v.Fields))
				for i, f := range v.Fields {
					fields[i] = f.String()
				}
				p.printf("  %s(%s)\n", v.Name, strings.Join(fields, ", "))
			}
		}
		p.printf("}\n")
	}

	for _, fn := range prog.Functions {
		p.printf("\n")
		p.printFunction(fn)
	}
}

func (p *printer) printFunction(fn *Function) {
	// Function signature.
	params := make([]string, len(fn.Params))
	for i, pid := range fn.Params {
		def := fn.Locals[int(pid)]
		typStr := "?"
		if def.Type != nil {
			typStr = def.Type.String()
		}
		params[i] = fmt.Sprintf("%%%d: %s", pid, typStr)
	}
	retStr := "?"
	if fn.ReturnType != nil {
		retStr = fn.ReturnType.String()
	}
	p.printf("fn %s(%s) -> %s {\n", fn.Name, strings.Join(params, ", "), retStr)

	// Locals table.
	if len(fn.Locals) > 0 {
		p.printf("  // locals:\n")
		for _, l := range fn.Locals {
			mut := ""
			if l.Mutable {
				mut = " mut"
			}
			typStr := "?"
		if l.Type != nil {
			typStr = l.Type.String()
		}
		p.printf("  //   %%%d: %s%s  (%s)\n", l.ID, typStr, mut, l.Name)
		}
		p.printf("\n")
	}

	// Blocks.
	for _, blk := range fn.Blocks {
		p.printBlock(blk)
	}

	p.printf("}\n")
}

func (p *printer) printBlock(blk *BasicBlock) {
	p.printf("  bb%d", blk.ID)
	if blk.Label != "" {
		p.printf(" (%s)", blk.Label)
	}

	// Print predecessor list.
	if len(blk.Preds) > 0 {
		preds := make([]string, len(blk.Preds))
		for i, pred := range blk.Preds {
			preds[i] = fmt.Sprintf("bb%d", pred)
		}
		p.printf(" [preds: %s]", strings.Join(preds, ", "))
	}
	p.printf(":\n")

	// Phi nodes.
	for _, phi := range blk.Phis {
		phiTypStr := "?"
		if phi.Type != nil {
			phiTypStr = phi.Type.String()
		}
		p.printf("    %%%d = phi %s", phi.Dest, phiTypStr)
		// Sort args by block ID for deterministic output.
		type phiArg struct {
			block BlockID
			val   Value
		}
		args := make([]phiArg, 0, len(phi.Args))
		for bid, v := range phi.Args {
			args = append(args, phiArg{bid, v})
		}
		sort.Slice(args, func(i, j int) bool { return args[i].block < args[j].block })
		p.printf(" [")
		for i, a := range args {
			if i > 0 {
				p.printf(", ")
			}
			p.printf("bb%d: %s", a.block, printValue(a.val))
		}
		p.printf("]\n")
	}

	// Statements.
	for _, stmt := range blk.Stmts {
		p.printf("    ")
		p.printStmt(stmt)
		p.printf("\n")
	}

	// Terminator.
	if blk.Term != nil {
		p.printf("    ")
		p.printTerminator(blk.Term)
		p.printf("\n")
	}
}

func (p *printer) printStmt(stmt Stmt) {
	switch s := stmt.(type) {
	case *Assign:
		p.printf("%%%d = %s", s.Dest, printValue(s.Src))

	case *CallStmt:
		args := printValues(s.Args)
		p.printf("%%%d = call %s(%s)", s.Dest, printValue(s.Func), strings.Join(args, ", "))

	case *BinaryOpStmt:
		p.printf("%%%d = %s %s %s", s.Dest, printValue(s.Left), s.Op, printValue(s.Right))

	case *UnaryOpStmt:
		p.printf("%%%d = %s%s", s.Dest, s.Op, printValue(s.Operand))

	case *FieldAccessStmt:
		p.printf("%%%d = %s.%s", s.Dest, printValue(s.Object), s.Field)

	case *IndexAccessStmt:
		p.printf("%%%d = %s[%s]", s.Dest, printValue(s.Object), printValue(s.Index))

	case *ArrayAllocStmt:
		elems := printValues(s.Elems)
		p.printf("%%%d = array [%s]", s.Dest, strings.Join(elems, ", "))

	case *StructAllocStmt:
		fields := make([]string, len(s.Fields))
		for i, f := range s.Fields {
			fields[i] = fmt.Sprintf("%s: %s", f.Name, printValue(f.Value))
		}
		p.printf("%%%d = struct %s { %s }", s.Dest, s.Name, strings.Join(fields, ", "))

	case *EnumAllocStmt:
		args := printValues(s.Args)
		p.printf("%%%d = enum %s::%s(%s)", s.Dest, s.EnumName, s.Variant, strings.Join(args, ", "))

	case *TagCheckStmt: // [CLAUDE-FIX]
		p.printf("%%%d = tagcheck %s %s::%s", s.Dest, s.Object, s.EnumName, s.Variant)

	case *ClosureAllocStmt:
		caps := printValues(s.Captures)
		p.printf("%%%d = closure @%s [%s]", s.Dest, s.FuncName, strings.Join(caps, ", "))

	case *ChannelCreateStmt:
		elemStr := "?"
		if s.ElemType != nil {
			elemStr = s.ElemType.String()
		}
		if s.BufSize != nil {
			p.printf("%%%d = channel<%s>(%s)", s.Dest, elemStr, printValue(s.BufSize))
		} else {
			p.printf("%%%d = channel<%s>()", s.Dest, elemStr)
		}

	case *ChannelSendStmt:
		p.printf("send %s <- %s", printValue(s.Chan), printValue(s.SendVal))

	case *SpawnStmt:
		args := printValues(s.Args)
		if len(args) > 0 {
			p.printf("%%%d = spawn %s(%s)", s.Dest, printValue(s.Func), strings.Join(args, ", "))
		} else {
			p.printf("%%%d = spawn %s", s.Dest, printValue(s.Func))
		}

	case *CastStmt:
		p.printf("%%%d = cast %s as %s", s.Dest, printValue(s.Src), s.Target.String())

	default:
		p.printf("<unknown stmt %T>", stmt)
	}
}

func (p *printer) printTerminator(term Terminator) {
	switch t := term.(type) {
	case *Goto:
		p.printf("goto bb%d", t.Target)

	case *Branch:
		p.printf("branch %s, bb%d, bb%d", printValue(t.Cond), t.Then, t.Else)

	case *Return:
		if t.Value != nil {
			p.printf("return %s", printValue(t.Value))
		} else {
			p.printf("return")
		}

	case *Switch:
		p.printf("switch %s [", printValue(t.Scrutinee))
		for i, c := range t.Cases {
			if i > 0 {
				p.printf(", ")
			}
			p.printf("%s -> bb%d", printValue(c.Value), c.Target)
		}
		p.printf("] default bb%d", t.Default)

	case *Unreachable:
		p.printf("unreachable")

	default:
		p.printf("<unknown term %T>", term)
	}
}

func printValue(v Value) string {
	if v == nil {
		return "<nil>"
	}
	return v.String()
}

func printValues(vals []Value) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = printValue(v)
	}
	return out
}
