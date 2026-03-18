package parser

import (
	"fmt"
	"strings"
)

// FormatAST serialises an AST into a canonical S-expression string for
// golden-file comparison.
func FormatAST(n Node) string {
	var b strings.Builder
	formatNode(&b, n, 0)
	return b.String()
}

func indent(b *strings.Builder, depth int) {
	for i := 0; i < depth; i++ {
		b.WriteString("  ")
	}
}

func formatNode(b *strings.Builder, n Node, depth int) {
	if n == nil {
		b.WriteString("nil")
		return
	}
	switch v := n.(type) {
	case *Program:
		formatProgram(b, v, depth)
	case *FnDef:
		formatFnDef(b, v, depth)
	case *TypeDef:
		formatTypeDef(b, v, depth)
	case *StructDef:
		formatStructDef(b, v, depth)
	case *TraitDef:
		formatTraitDef(b, v, depth)
	case *ImplBlock:
		formatImplBlock(b, v, depth)
	case *ImportDecl:
		fmt.Fprintf(b, "(Import %s)", strings.Join(v.Path, "::"))
	case *ModuleDecl:
		fmt.Fprintf(b, "(Module %s)", v.Name)
	case *LetStmt:
		formatLetStmt(b, v, depth)
	case *ExprStmt:
		b.WriteString("(ExprStmt ")
		formatExpr(b, v.Expr, depth)
		b.WriteString(")")
	case *ReturnStmt:
		b.WriteString("(ReturnStmt")
		if v.Value != nil {
			b.WriteString(" ")
			formatExpr(b, v.Value, depth)
		}
		b.WriteString(")")
	default:
		if e, ok := n.(Expr); ok {
			formatExpr(b, e, depth)
		} else if p, ok := n.(Pattern); ok {
			formatPattern(b, p)
		} else if t, ok := n.(TypeExpr); ok {
			formatTypeExpr(b, t)
		} else {
			fmt.Fprintf(b, "(%T)", n)
		}
	}
}

func formatProgram(b *strings.Builder, p *Program, depth int) {
	b.WriteString("(Program\n")
	for _, item := range p.Items {
		indent(b, depth+1)
		formatNode(b, item, depth+1)
		b.WriteString("\n")
	}
	indent(b, depth)
	b.WriteString(")")
}

func formatFnDef(b *strings.Builder, f *FnDef, depth int) {
	b.WriteString("(FnDef")
	if f.Public {
		b.WriteString(" pub")
	}
	fmt.Fprintf(b, " %s", f.Name)
	if f.GenParams != nil {
		b.WriteString("<")
		for i, gp := range f.GenParams.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(gp.Name)
			if len(gp.Bounds) > 0 {
				b.WriteString(": ")
				for j, bound := range gp.Bounds {
					if j > 0 {
						b.WriteString(" + ")
					}
					formatTypeExpr(b, bound)
				}
			}
		}
		b.WriteString(">")
	}
	b.WriteString("(")
	for i, p := range f.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name)
		if p.Type != nil {
			b.WriteString(": ")
			formatTypeExpr(b, p.Type)
		}
	}
	b.WriteString(")")
	if f.ReturnType != nil {
		b.WriteString(" -> ")
		formatTypeExpr(b, f.ReturnType)
	}
	if f.Body != nil {
		b.WriteString("\n")
		indent(b, depth+1)
		formatExpr(b, f.Body, depth+1)
	}
	b.WriteString(")")
}

func formatTypeDef(b *strings.Builder, t *TypeDef, depth int) {
	b.WriteString("(TypeDef")
	if t.Public {
		b.WriteString(" pub")
	}
	fmt.Fprintf(b, " %s", t.Name)
	if t.GenParams != nil {
		b.WriteString("<")
		for i, gp := range t.GenParams.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(gp.Name)
		}
		b.WriteString(">")
	}
	for _, v := range t.Variants {
		b.WriteString("\n")
		indent(b, depth+1)
		fmt.Fprintf(b, "(Variant %s", v.Name)
		if len(v.Fields) > 0 {
			b.WriteString("(")
			for i, f := range v.Fields {
				if i > 0 {
					b.WriteString(", ")
				}
				formatTypeExpr(b, f)
			}
			b.WriteString(")")
		}
		b.WriteString(")")
	}
	b.WriteString(")")
}

func formatStructDef(b *strings.Builder, s *StructDef, depth int) {
	b.WriteString("(StructDef")
	if s.Public {
		b.WriteString(" pub")
	}
	fmt.Fprintf(b, " %s", s.Name)
	if s.GenParams != nil {
		b.WriteString("<")
		for i, gp := range s.GenParams.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(gp.Name)
		}
		b.WriteString(">")
	}
	for _, f := range s.Fields {
		b.WriteString("\n")
		indent(b, depth+1)
		if f.Public {
			b.WriteString("(Field pub ")
		} else {
			b.WriteString("(Field ")
		}
		fmt.Fprintf(b, "%s: ", f.Name)
		formatTypeExpr(b, f.Type)
		b.WriteString(")")
	}
	b.WriteString(")")
}

func formatTraitDef(b *strings.Builder, t *TraitDef, depth int) {
	b.WriteString("(TraitDef")
	if t.Public {
		b.WriteString(" pub")
	}
	fmt.Fprintf(b, " %s", t.Name)
	for _, m := range t.Methods {
		b.WriteString("\n")
		indent(b, depth+1)
		formatFnDef(b, m, depth+1)
	}
	b.WriteString(")")
}

func formatImplBlock(b *strings.Builder, im *ImplBlock, depth int) {
	b.WriteString("(ImplBlock")
	if im.TraitName != "" {
		fmt.Fprintf(b, " %s for", im.TraitName)
	}
	b.WriteString(" ")
	formatTypeExpr(b, im.TargetType)
	for _, m := range im.Methods {
		b.WriteString("\n")
		indent(b, depth+1)
		formatFnDef(b, m, depth+1)
	}
	b.WriteString(")")
}

func formatLetStmt(b *strings.Builder, l *LetStmt, depth int) {
	b.WriteString("(Let")
	if l.Mutable {
		b.WriteString(" mut")
	}
	fmt.Fprintf(b, " %s", l.Name)
	if l.Type != nil {
		b.WriteString(": ")
		formatTypeExpr(b, l.Type)
	}
	if l.Value != nil {
		b.WriteString(" = ")
		formatExpr(b, l.Value, depth)
	}
	b.WriteString(")")
}

func formatExpr(b *strings.Builder, e Expr, depth int) {
	if e == nil {
		b.WriteString("nil")
		return
	}
	switch v := e.(type) {
	case *IntLit:
		fmt.Fprintf(b, "(Int %s)", v.Value)
	case *FloatLit:
		fmt.Fprintf(b, "(Float %s)", v.Value)
	case *StringLit:
		fmt.Fprintf(b, "(String %s)", v.Value)
	case *CharLit:
		fmt.Fprintf(b, "(Char %s)", v.Value)
	case *BoolLit:
		fmt.Fprintf(b, "(Bool %v)", v.Value)
	case *Ident:
		fmt.Fprintf(b, "(Ident %s)", v.Name)
	case *SelfExpr:
		b.WriteString("(Self)")
	case *BinaryExpr:
		fmt.Fprintf(b, "(Binary %s ", v.Op)
		formatExpr(b, v.Left, depth)
		b.WriteString(" ")
		formatExpr(b, v.Right, depth)
		b.WriteString(")")
	case *UnaryExpr:
		fmt.Fprintf(b, "(Unary %s ", v.Op)
		formatExpr(b, v.Operand, depth)
		b.WriteString(")")
	case *CallExpr:
		b.WriteString("(Call ")
		formatExpr(b, v.Func, depth)
		for _, arg := range v.Args {
			b.WriteString(" ")
			formatExpr(b, arg, depth)
		}
		b.WriteString(")")
	case *FieldExpr:
		b.WriteString("(Field ")
		formatExpr(b, v.Object, depth)
		fmt.Fprintf(b, " .%s)", v.Field)
	case *IndexExpr:
		b.WriteString("(Index ")
		formatExpr(b, v.Object, depth)
		b.WriteString(" ")
		formatExpr(b, v.Index, depth)
		b.WriteString(")")
	case *IfExpr:
		b.WriteString("(If ")
		formatExpr(b, v.Cond, depth)
		b.WriteString("\n")
		indent(b, depth+1)
		formatExpr(b, v.Then, depth+1)
		if v.Else != nil {
			b.WriteString("\n")
			indent(b, depth+1)
			formatExpr(b, v.Else, depth+1)
		}
		b.WriteString(")")
	case *MatchExpr:
		b.WriteString("(Match ")
		formatExpr(b, v.Scrutinee, depth)
		for _, arm := range v.Arms {
			b.WriteString("\n")
			indent(b, depth+1)
			b.WriteString("(Arm ")
			formatPattern(b, arm.Pattern)
			if arm.Guard != nil {
				b.WriteString(" if ")
				formatExpr(b, arm.Guard, depth+1)
			}
			b.WriteString(" => ")
			formatExpr(b, arm.Body, depth+1)
			b.WriteString(")")
		}
		b.WriteString(")")
	case *ForExpr:
		fmt.Fprintf(b, "(For %s in ", v.Binding)
		formatExpr(b, v.Iter, depth)
		b.WriteString("\n")
		indent(b, depth+1)
		formatExpr(b, v.Body, depth+1)
		b.WriteString(")")
	case *WhileExpr:
		b.WriteString("(While ")
		formatExpr(b, v.Cond, depth)
		b.WriteString("\n")
		indent(b, depth+1)
		formatExpr(b, v.Body, depth+1)
		b.WriteString(")")
	case *LoopExpr:
		b.WriteString("(Loop\n")
		indent(b, depth+1)
		formatExpr(b, v.Body, depth+1)
		b.WriteString(")")
	case *Block:
		b.WriteString("(Block")
		for _, s := range v.Stmts {
			b.WriteString("\n")
			indent(b, depth+1)
			formatNode(b, s, depth+1)
		}
		if v.TrailingExpr != nil {
			b.WriteString("\n")
			indent(b, depth+1)
			b.WriteString("=> ")
			formatExpr(b, v.TrailingExpr, depth+1)
		}
		b.WriteString(")")
	case *LambdaExpr:
		b.WriteString("(Lambda |")
		for i, p := range v.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(p.Name)
			if p.Type != nil {
				b.WriteString(": ")
				formatTypeExpr(b, p.Type)
			}
		}
		b.WriteString("|")
		if v.ReturnType != nil {
			b.WriteString(" -> ")
			formatTypeExpr(b, v.ReturnType)
		}
		b.WriteString(" ")
		formatExpr(b, v.Body, depth)
		b.WriteString(")")
	case *SpawnExpr:
		b.WriteString("(Spawn\n")
		indent(b, depth+1)
		formatExpr(b, v.Body, depth+1)
		b.WriteString(")")
	case *ChannelExpr:
		b.WriteString("(Channel ")
		formatTypeExpr(b, v.ElemType)
		if v.Size != nil {
			b.WriteString(" ")
			formatExpr(b, v.Size, depth)
		}
		b.WriteString(")")
	case *ArrayLit:
		b.WriteString("(Array")
		for _, el := range v.Elems {
			b.WriteString(" ")
			formatExpr(b, el, depth)
		}
		b.WriteString(")")
	case *TupleLit:
		b.WriteString("(Tuple")
		for _, el := range v.Elems {
			b.WriteString(" ")
			formatExpr(b, el, depth)
		}
		b.WriteString(")")
	case *StructLit:
		fmt.Fprintf(b, "(StructLit %s", v.Name)
		for _, f := range v.Fields {
			fmt.Fprintf(b, " %s=", f.Name)
			formatExpr(b, f.Value, depth)
		}
		b.WriteString(")")
	case *GroupExpr:
		b.WriteString("(Group ")
		formatExpr(b, v.Inner, depth)
		b.WriteString(")")
	case *BreakExpr:
		b.WriteString("(Break)")
	case *ContinueExpr:
		b.WriteString("(Continue)")
	case *ReturnExpr:
		b.WriteString("(Return")
		if v.Value != nil {
			b.WriteString(" ")
			formatExpr(b, v.Value, depth)
		}
		b.WriteString(")")
	case *PathExpr:
		fmt.Fprintf(b, "(Path %s)", strings.Join(v.Segments, "::"))
	case *AsExpr:
		b.WriteString("(As ")
		formatExpr(b, v.Expr, depth)
		b.WriteString(" ")
		formatTypeExpr(b, v.Type)
		b.WriteString(")")
	default:
		fmt.Fprintf(b, "(?Expr %T)", e)
	}
}

func formatPattern(b *strings.Builder, p Pattern) {
	if p == nil {
		b.WriteString("nil")
		return
	}
	switch v := p.(type) {
	case *WildcardPat:
		b.WriteString("_")
	case *BindingPat:
		fmt.Fprintf(b, "(Bind %s)", v.Name)
	case *LiteralPat:
		b.WriteString("(LitPat ")
		formatExpr(b, v.Value, 0)
		b.WriteString(")")
	case *TuplePat:
		b.WriteString("(TuplePat")
		for _, el := range v.Elems {
			b.WriteString(" ")
			formatPattern(b, el)
		}
		b.WriteString(")")
	case *VariantPat:
		fmt.Fprintf(b, "(VariantPat %s", v.Name)
		for _, f := range v.Fields {
			b.WriteString(" ")
			formatPattern(b, f)
		}
		b.WriteString(")")
	case *StructPat:
		fmt.Fprintf(b, "(StructPat %s", v.Name)
		for _, f := range v.Fields {
			fmt.Fprintf(b, " %s:", f.Name)
			if f.Pattern != nil {
				formatPattern(b, f.Pattern)
			} else {
				b.WriteString("_")
			}
		}
		b.WriteString(")")
	case *OrPat:
		b.WriteString("(OrPat")
		for _, alt := range v.Alts {
			b.WriteString(" ")
			formatPattern(b, alt)
		}
		b.WriteString(")")
	case *RangePat:
		b.WriteString("(RangePat ")
		formatExpr(b, v.Start, 0)
		if v.Inclusive {
			b.WriteString("..=")
		} else {
			b.WriteString("..")
		}
		formatExpr(b, v.End, 0)
		b.WriteString(")")
	default:
		fmt.Fprintf(b, "(?Pat %T)", p)
	}
}

func formatTypeExpr(b *strings.Builder, t TypeExpr) {
	if t == nil {
		b.WriteString("nil")
		return
	}
	switch v := t.(type) {
	case *NamedType:
		b.WriteString(v.Name)
		if len(v.GenArgs) > 0 {
			b.WriteString("<")
			for i, a := range v.GenArgs {
				if i > 0 {
					b.WriteString(", ")
				}
				formatTypeExpr(b, a)
			}
			b.WriteString(">")
		}
	case *FnType:
		b.WriteString("(")
		for i, p := range v.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			formatTypeExpr(b, p)
		}
		b.WriteString(") -> ")
		formatTypeExpr(b, v.Return)
	case *TupleType:
		b.WriteString("(")
		for i, el := range v.Elems {
			if i > 0 {
				b.WriteString(", ")
			}
			formatTypeExpr(b, el)
		}
		b.WriteString(")")
	case *ArrayType:
		b.WriteString("[")
		formatTypeExpr(b, v.Elem)
		b.WriteString("]")
	case *SliceType:
		b.WriteString("&[")
		formatTypeExpr(b, v.Elem)
		b.WriteString("]")
	case *ReferenceType:
		b.WriteString("&")
		formatTypeExpr(b, v.Elem)
	case *SelfType:
		b.WriteString("Self")
	default:
		fmt.Fprintf(b, "(?Type %T)", t)
	}
}
