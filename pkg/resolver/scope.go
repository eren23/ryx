package resolver

import "github.com/ryx-lang/ryx/pkg/diagnostic"

// ScopeKind classifies the kind of scope.
type ScopeKind int

const (
	ModuleScope   ScopeKind = iota // top-level module
	FunctionScope                  // fn body
	BlockScope                     // block, if, loop, etc.
	ImplScope                      // impl block
	TraitScope                     // trait definition
)

func (k ScopeKind) String() string {
	switch k {
	case ModuleScope:
		return "module"
	case FunctionScope:
		return "function"
	case BlockScope:
		return "block"
	case ImplScope:
		return "impl"
	case TraitScope:
		return "trait"
	default:
		return "unknown"
	}
}

// SymbolKind classifies the kind of symbol.
type SymbolKind int

const (
	VariableSymbol  SymbolKind = iota // let binding or for-loop binding
	FunctionSymbol                    // fn definition
	TypeSymbol                        // type / struct definition
	TraitSymbol                       // trait definition
	FieldSymbol                       // struct field
	VariantSymbol                     // enum variant constructor
	TypeParamSymbol                   // generic type parameter
	ImportSymbol                      // import declaration
)

func (k SymbolKind) String() string {
	switch k {
	case VariableSymbol:
		return "variable"
	case FunctionSymbol:
		return "function"
	case TypeSymbol:
		return "type"
	case TraitSymbol:
		return "trait"
	case FieldSymbol:
		return "field"
	case VariantSymbol:
		return "variant"
	case TypeParamSymbol:
		return "type parameter"
	case ImportSymbol:
		return "import"
	default:
		return "unknown"
	}
}

// Symbol represents a named entity in the program.
type Symbol struct {
	Name      string
	Kind      SymbolKind
	Span      diagnostic.Span
	Mutable   bool
	Public    bool
	DefScope  *Scope
	UsedCount int

	// ParentType links variant symbols to their parent TypeDef name.
	ParentType string
	// VariantIndex is the index of this variant within its parent TypeDef.
	VariantIndex int
	// FieldCount stores how many data fields a variant constructor expects.
	FieldCount int
	// Variants stores variant names for TypeSymbol (used for exhaustiveness).
	Variants []string
}

// Scope represents a lexical scope in the scope tree.
type Scope struct {
	Parent   *Scope
	Children []*Scope
	Symbols  map[string]*Symbol
	Kind     ScopeKind

	// InLoop tracks whether this scope is inside a loop construct.
	InLoop bool
	// FnName is set for FunctionScope scopes, for return-type checking.
	FnName string
	// ImplTarget is set for ImplScope scopes.
	ImplTarget string
	// TraitName is set for TraitScope scopes.
	TraitName string
}

// NewScope creates a new scope with the given kind and parent.
func NewScope(kind ScopeKind, parent *Scope) *Scope {
	s := &Scope{
		Kind:    kind,
		Parent:  parent,
		Symbols: make(map[string]*Symbol),
	}
	if parent != nil {
		parent.Children = append(parent.Children, s)
		s.InLoop = parent.InLoop
	}
	return s
}

// Define adds a symbol to this scope. Returns the existing symbol if
// the name is already defined in this scope (same-scope shadowing).
func (s *Scope) Define(sym *Symbol) *Symbol {
	if existing, ok := s.Symbols[sym.Name]; ok {
		return existing
	}
	sym.DefScope = s
	s.Symbols[sym.Name] = sym
	return nil
}

// Lookup searches for a symbol by name, walking up the scope chain.
func (s *Scope) Lookup(name string) *Symbol {
	for scope := s; scope != nil; scope = scope.Parent {
		if sym, ok := scope.Symbols[name]; ok {
			return sym
		}
	}
	return nil
}

// LookupLocal searches for a symbol only in the current scope.
func (s *Scope) LookupLocal(name string) *Symbol {
	return s.Symbols[name]
}

// IsInsideImpl returns true if this scope or any ancestor is an ImplScope.
func (s *Scope) IsInsideImpl() bool {
	for scope := s; scope != nil; scope = scope.Parent {
		if scope.Kind == ImplScope {
			return true
		}
	}
	return false
}

// IsInsideTrait returns true if this scope or any ancestor is a TraitScope.
func (s *Scope) IsInsideTrait() bool {
	for scope := s; scope != nil; scope = scope.Parent {
		if scope.Kind == TraitScope {
			return true
		}
	}
	return false
}

// IsInsideTraitOrImpl returns true if inside either a trait or impl scope.
func (s *Scope) IsInsideTraitOrImpl() bool {
	return s.IsInsideTrait() || s.IsInsideImpl()
}

// EnclosingFunction returns the nearest enclosing FunctionScope, or nil.
func (s *Scope) EnclosingFunction() *Scope {
	for scope := s; scope != nil; scope = scope.Parent {
		if scope.Kind == FunctionScope {
			return scope
		}
	}
	return nil
}

// AllSymbolNames returns all names visible from this scope (for "did you mean" suggestions).
func (s *Scope) AllSymbolNames() []string {
	seen := make(map[string]bool)
	var names []string
	for scope := s; scope != nil; scope = scope.Parent {
		for name := range scope.Symbols {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}
