# Swarm Goal

Ryx Programming Language — Compiler + Bytecode VM in Go

Build a complete compiler toolchain and virtual machine for **Ryx**, a
statically-typed, expression-oriented programming language with algebraic
data types, pattern matching, closures, traits, and a concurrent runtime.
Implemented in Go for compilation speed and goroutine-based concurrency.
A full test suite validates every layer from lexing individual tokens up
to whole-program semantic guarantees and VM execution correctness.

---

## 0) Language Overview

Ryx is a small but complete language designed to exercise every major
compiler subsystem. It combines ML-family type safety with Rust-like
ownership-free ergonomics and Go-like concurrency primitives.

### Sample Program
```ryx
type Tree<T> {
    Leaf(T),
    Node(Tree<T>, T, Tree<T>),
}

trait Summable {
    fn zero() -> Self;
    fn add(self, other: Self) -> Self;
}

impl Summable for Int {
    fn zero() -> Int { 0 }
    fn add(self, other: Int) -> Int { self + other }
}

fn sum_tree<T: Summable>(tree: Tree<T>) -> T {
    match tree {
        Leaf(val) => val,
        Node(left, val, right) => {
            let l = sum_tree(left);
            let r = sum_tree(right);
            l.add(val).add(r)
        }
    }
}

fn main() {
    let tree = Node(
        Node(Leaf(1), 2, Leaf(3)),
        4,
        Node(Leaf(5), 6, Leaf(7)),
    );
    let total = sum_tree(tree);
    println(total);  // 28

    // Closures
    let adder = |x: Int| -> (Int -> Int) {
        |y: Int| -> Int { x + y }
    };
    let add5 = adder(5);
    println(add5(10));  // 15

    // Concurrency
    let ch = channel<Int>(10);
    spawn {
        for i in 0..100 {
            ch.send(i * i);
        }
        ch.close();
    };
    let mut total = 0;
    for val in ch {
        total = total + val;
    }
    println(total);  // 328350

    // Error handling
    match parse_int("42") {
        Ok(n) => println(n),
        Err(msg) => println("error: " ++ msg),
    }
}
```

### Type System
- Primitives: `Int` (64-bit signed), `Float` (64-bit IEEE 754),
  `Bool`, `Char` (Unicode scalar), `String` (UTF-8, immutable),
  `Unit` (zero-size, like `void`)
- Algebraic data types (enums with associated data)
- Structs (named product types)
- Tuples: `(Int, String, Bool)`
- Arrays: `[Int]` (fixed-size after creation)
- Slices: `&[Int]` (view into array)
- Functions: `(Int, Int) -> Bool`
- Closures: same type as functions, capture by value
- Generics: `<T>`, `<T: Trait>`, `<T: Trait1 + Trait2>`
- `Option<T>` and `Result<T, E>` as built-in algebraic types
- `Channel<T>` for concurrent communication
- No null — `Option<T>` is the only way to represent absence
- No implicit conversions between any types
- Structural equality for all value types; reference equality opt-in

### Expression-Oriented
Everything is an expression:
- `if/else` returns a value: `let x = if cond { 1 } else { 2 };`
- `match` returns a value: `let y = match opt { Some(v) => v, None => 0 };`
- Block expressions: last expression is the block's value
- `for`/`while` return `Unit`

### Ownership Model
No ownership/borrow checking. Values are either:
- **Stack-allocated**: primitives, small structs (≤ 64 bytes)
- **Heap-allocated**: strings, arrays, closures, ADT variants with data,
  anything captured by a closure

Heap values are **garbage collected** (tricolor mark-sweep).

---

## 1) Lexer

Hand-written scanner producing a token stream. No regex, no generator.

### Token Types (52)
```
// Literals
INT_LIT         // 42, 0xFF, 0b1010, 0o77, 1_000_000
FLOAT_LIT       // 3.14, 1e10, 2.5e-3
STRING_LIT      // "hello\nworld", "tab\there"
CHAR_LIT        // 'a', '\n', '\u{1F600}'
BOOL_LIT        // true, false

// Identifiers & Keywords (25 keywords)
IDENT           // foo, _bar, camelCase
FN, LET, MUT, IF, ELSE, MATCH, FOR, WHILE, LOOP, BREAK, CONTINUE,
RETURN, TYPE, STRUCT, TRAIT, IMPL, SPAWN, CHANNEL, IN, AS, PUB,
IMPORT, MODULE, SELF_KW, SELF_TYPE

// Operators (20)
PLUS, MINUS, STAR, SLASH, PERCENT,           // + - * / %
EQ, NEQ, LT, GT, LEQ, GEQ,                  // == != < > <= >=
AND, OR, NOT,                                // && || !
PIPE,                                        // |>
CONCAT,                                      // ++
RANGE, RANGE_INCLUSIVE,                       // .. ..=
ASSIGN, ARROW,                               // = ->

// Delimiters
LPAREN, RPAREN, LBRACE, RBRACE, LBRACKET, RBRACKET,
COMMA, COLON, SEMICOLON, DOT, DOUBLE_COLON,
FAT_ARROW,                                   // =>

// Special
NEWLINE, EOF, ERROR
```

### Lexer Requirements
- Single-pass, character-by-character
- Track line number, column, byte offset for every token
- Source span: `(start_offset, end_offset)` — used for error messages
- Nested block comments: `/* ... /* ... */ ... */`
- Line comments: `// ...`
- String escapes: `\n`, `\t`, `\r`, `\\`, `\"`, `\0`, `\u{XXXX}`
- Raw strings: `r"no \escapes here"`
- Integer literal bases: decimal, hex (`0x`), binary (`0b`), octal (`0o`)
- Underscore separators in numeric literals: `1_000_000`, `0xFF_FF`
- Contextual keywords: `self` is both keyword and valid pattern variable
- Produce `ERROR` token with message for malformed input (never panic)
- UTF-8 aware: identifiers may contain Unicode letters (categories L*)
- Idempotent: same input always produces identical token stream

---

## 2) Parser

Hand-written recursive descent + Pratt parsing for expressions.

### Grammar Highlights
```
program        = (item)* EOF
item           = fn_def | type_def | struct_def | trait_def
               | impl_block | import | module_decl
fn_def         = "pub"? "fn" IDENT generics? "(" params ")" ("->" type)? block
type_def       = "pub"? "type" IDENT generics? "{" variant ("," variant)* ","? "}"
struct_def     = "pub"? "struct" IDENT generics? "{" field ("," field)* ","? "}"
trait_def      = "pub"? "trait" IDENT generics? "{" trait_method* "}"
impl_block     = "impl" generics? trait_name "for" type "{" method* "}"
               | "impl" generics? type "{" method* "}"

expr           = assignment
assignment     = pipe_expr ("=" expr)?
pipe_expr      = or_expr ("|>" or_expr)*
or_expr        = and_expr ("||" and_expr)*
and_expr       = equality ("&&" equality)*
equality       = comparison (("==" | "!=") comparison)*
comparison     = range (("<" | ">" | "<=" | ">=") range)*
range          = addition ((".." | "..=") addition)?
addition       = multiply (("+" | "-" | "++") multiply)*
multiply       = unary (("*" | "/" | "%") unary)*
unary          = ("!" | "-") unary | call
call           = primary ("(" args ")" | "." IDENT | "[" expr "]")*
primary        = INT_LIT | FLOAT_LIT | STRING_LIT | CHAR_LIT | BOOL_LIT
               | IDENT | "(" expr ")" | tuple_expr
               | if_expr | match_expr | block_expr
               | lambda | array_lit | struct_lit
               | for_expr | while_expr | loop_expr
               | spawn_expr | channel_expr

match_expr     = "match" expr "{" match_arm ("," match_arm)* ","? "}"
match_arm      = pattern ("if" expr)? "=>" expr
pattern        = "_" | IDENT | literal_pat | tuple_pat
               | variant_pat | struct_pat | "(" pattern ")"
               | range_pat | or_pat
variant_pat    = IDENT "(" pattern ("," pattern)* ")"
or_pat         = pattern ("|" pattern)*
```

### Pratt Precedence (14 levels)
```
1  (lowest)  Assignment      =
2            Pipe            |>
3            LogicalOr       ||
4            LogicalAnd      &&
5            Equality        == !=
6            Comparison      < > <= >=
7            Range           .. ..=
8            Concat          ++
9            Addition        + -
10           Multiplication  * / %
11           Unary           ! - (prefix)
12           Call            () . []
13           Field           .field
14 (highest) Primary         literals, idents, grouped
```

### AST Node Types
```go
type Node interface {
    Span() Span
    nodeTag()
}

// 40+ AST node types including:
// - Program, FnDef, TypeDef, StructDef, TraitDef, ImplBlock
// - Block, LetStmt, ExprStmt, ReturnStmt
// - IfExpr, MatchExpr, ForExpr, WhileExpr, LoopExpr
// - BinaryExpr, UnaryExpr, CallExpr, FieldExpr, IndexExpr
// - LambdaExpr, SpawnExpr, ChannelExpr
// - IntLit, FloatLit, StringLit, CharLit, BoolLit
// - ArrayLit, TupleLit, StructLit
// - Pattern variants: WildcardPat, BindingPat, VariantPat, etc.
// - TypeExpr variants: NamedType, FnType, TupleType, ArrayType, etc.
```

### Error Recovery
- Synchronize on `{`, `}`, `;`, `fn`, `type`, `struct`, `trait`, `impl`
- Collect up to 20 errors before aborting
- Each error includes: source span, message, optional "did you mean?" hint
- Missing semicolons: insert automatically before `}` and after
  expressions followed by newline (if no ambiguity)
- Unclosed delimiters: report location of opening delimiter

---

## 3) Name Resolution & Semantic Analysis

Two-pass analysis on the AST before type checking.

### Pass 1: Symbol Table Construction
Build a scope tree with lexical scoping:
```go
type Scope struct {
    Parent   *Scope
    Children []*Scope
    Symbols  map[string]*Symbol
    Kind     ScopeKind  // Module | Function | Block | Impl | Trait
}

type Symbol struct {
    Name       string
    Kind       SymbolKind  // Variable | Function | Type | Trait | Field | Variant | TypeParam
    Span       Span
    Type       Type        // filled in by type checker
    Mutable    bool
    Public     bool
    DefScope   *Scope
    UsedCount  int
}
```

Rules:
- No shadowing in same scope (error). Shadowing in nested scope: warning.
- Variables must be declared before use.
- Functions and types can be used before declaration (hoisted within module).
- `self` only valid inside `impl` blocks.
- `Self` type alias only valid inside `trait` and `impl` blocks.
- Generic type parameters scoped to their declaration (fn, type, trait, impl).

### Pass 2: Resolution
- Resolve every identifier to its `Symbol`.
- Resolve type names to type definitions.
- Resolve trait method calls to the correct `impl` block.
- Resolve variant constructors (e.g., `Leaf(x)`) to their parent type.
- Check: all match arms cover the type's variants (exhaustiveness checking).
- Check: no unreachable match arms after a wildcard.
- Check: `break`/`continue` only inside loops.
- Check: `return` type matches function signature.
- Detect unused variables (warning), unused imports (warning), unused
  mut (warning: "variable does not need to be mutable").

---

## 4) Type System & Inference

Hindley-Milner type inference extended with traits (type classes).

### Type Representation
```go
type Type interface {
    typeTag()
    String() string
}

// Concrete types
type IntType struct{}
type FloatType struct{}
type BoolType struct{}
type CharType struct{}
type StringType struct{}
type UnitType struct{}
type ArrayType struct{ Elem Type; Size int }  // -1 for unsized
type SliceType struct{ Elem Type }
type TupleType struct{ Elems []Type }
type FnType struct{ Params []Type; Return Type }
type StructType struct{ Name string; Fields []Field; TypeParams []Type }
type EnumType struct{ Name string; Variants []Variant; TypeParams []Type }
type ChannelType struct{ Elem Type }

// Inference machinery
type TypeVar struct{ ID int }          // unification variable
type TypeScheme struct {               // ∀ a b. Constraint => Type
    Params      []TypeVar
    Constraints []TraitBound
    Body        Type
}
```

### Inference Algorithm
1. **Generate constraints**: walk AST, assign fresh type variables,
   emit equality constraints and trait constraints.
2. **Unify**: standard union-find unification.
   - Occurs check (reject infinite types).
   - Error on concrete type mismatch with source spans.
3. **Solve trait constraints**: for each `T: Trait` constraint, find
   matching `impl`. If `T` is still a variable, defer until resolved.
   Error if no impl found after all constraints processed.
4. **Generalize**: at `let` bindings and `fn` definitions, generalize
   unconstrained type variables into `TypeScheme`.
5. **Monomorphize**: at call sites, instantiate `TypeScheme` with fresh
   type variables and add new constraints.

### Trait Resolution
```
impl Summable for Int { ... }
impl<T: Summable> Summable for Option<T> { ... }
```
- Trait impls form a registry keyed by `(TraitName, ConcreteType)`.
- Generic impls matched by unification.
- Coherence: at most one impl per (Trait, Type) pair. Overlapping impls
  are a compile error.
- Orphan rule: an impl must be in the same module as either the trait
  or the type.

### Type Checking Rules (selected)
- `if cond { a } else { b }`: `cond` must be `Bool`, `a` and `b` must
  unify to the same type.
- `match` arms: all patterns must be compatible with scrutinee type,
  all arm bodies must unify.
- Binary ops: `+ - * / %` require both operands same numeric type.
  `++` requires both `String`. `&& ||` require both `Bool`.
  `== !=` require both same type that implements `Eq`.
  `< > <= >=` require `Ord`.
- Array indexing: index must be `Int`, result is element type.
- Closure: parameter types may be inferred from context. If annotation
  present, must be consistent.
- `spawn`: body must return `Unit`. Captures by value (clone).
- `channel<T>(cap)`: `cap` must be `Int`, result is `Channel<T>`.
- `ch.send(val)`: `val` must match channel element type.
- `for x in ch { ... }`: `x` gets channel element type.
- `for x in a..b { ... }`: `a`, `b` must be `Int`. `x` is `Int`.

### Exhaustiveness Checker
For `match` expressions, verify that patterns cover all cases:
- Wildcard `_` covers everything.
- Enum: all variants must appear (or wildcard).
- Tuple: each position must be independently covered.
- Nested: recursive coverage check.
- Guard clauses (`if expr`) make the arm non-exhaustive for that variant.
- Report: "non-exhaustive patterns: `Variant3` not covered".

---

## 5) Intermediate Representation

### HIR (High-level IR)
Desugared AST, produced after type checking:
- All types fully resolved (no type variables).
- Generic functions monomorphized (one copy per concrete type combination).
  Monomorphization limit: 64 instances per generic function (error beyond).
- Pattern matching compiled to decision trees.
- `for x in a..b` desugared to `while` + counter.
- `for x in ch` desugared to `loop { match ch.recv() ... }`.
- Method calls desugared to static dispatch: `x.foo(y)` → `Type::foo(x, y)`.
- Pipe operator desugared: `a |> f |> g` → `g(f(a))`.
- String concatenation `++` desugared to `String::concat(a, b)`.
- Closures: captured variables identified, closure struct generated.

### MIR (Mid-level IR)
SSA-form (Static Single Assignment) control flow graph:
```go
type MIRFunction struct {
    Name       string
    Params     []MIRLocal
    ReturnType Type
    Locals     []MIRLocal      // all local variables
    Blocks     []BasicBlock
    EntryBlock BlockID
}

type BasicBlock struct {
    ID         BlockID
    Stmts      []MIRStmt
    Terminator Terminator
}

type MIRStmt interface{ mirStmt() }
// Assign, Call, BinaryOp, UnaryOp, FieldAccess, IndexAccess,
// ArrayAlloc, StructAlloc, EnumAlloc, ClosureAlloc,
// ChannelCreate, ChannelSend, Spawn

type Terminator interface{ termTag() }
// Goto(BlockID)
// Branch(cond MIRValue, then BlockID, else BlockID)
// Switch(val MIRValue, cases []SwitchCase, default BlockID)
// Return(MIRValue)
// Unreachable
```

Each value in MIR is either:
- `MIRLocal` (SSA variable, assigned exactly once)
- `MIRConst` (compile-time constant)
- `MIRGlobal` (module-level binding)

---

## 6) Optimization Passes

Operate on MIR. Each pass is a function `MIRFunction → MIRFunction`.
Passes run in a fixed pipeline:

### Pass Pipeline (11 passes)
```
1. ConstantFolding        — evaluate constant expressions at compile time
2. DeadCodeElimination    — remove unreachable blocks and unused assignments
3. CopyPropagation        — replace uses of `x = y` with `y` directly
4. CommonSubexprElim      — deduplicate identical computations in same block
5. InlineSmallFunctions   — inline functions with ≤ 8 MIR statements
6. ConstantPropagation    — propagate known constant values through graph
7. LoopInvariantMotion    — hoist invariant computations out of loops
8. TailCallOptimization   — convert tail-recursive calls to loops
9. EscapeAnalysis         — move heap allocations to stack when possible
10. DeadCodeElimination   — second pass to clean up after other opts
11. BlockMerging          — merge single-predecessor/successor blocks
```

### Constant Folding Rules
- `3 + 4` → `7` (all arithmetic on int/float literals)
- `true && false` → `false`
- `"hello" ++ " world"` → `"hello world"`
- `if true { a } else { b }` → `a`
- `!true` → `false`
- `-(-x)` → `x`
- `x * 0` → `0` (int only, not float due to NaN)
- `x * 1` → `x`
- `x + 0` → `x`
- `x / 1` → `x`

### Escape Analysis
Determine if heap-allocated values escape their creating function:
- If value is only used locally and not captured by closures or sent
  to channels: allocate on stack instead.
- Tracks: return, channel send, closure capture, store to heap object.
- Conservative: if uncertain, keep on heap.
- Benefit: reduces GC pressure significantly for short-lived objects.

### Tail Call Optimization
Detect functions where the last operation is a self-recursive call:
```ryx
fn factorial(n: Int, acc: Int) -> Int {
    if n <= 1 { acc }
    else { factorial(n - 1, n * acc) }  // tail position
}
```
Convert to:
```
loop {
    if n <= 1 { return acc }
    acc = n * acc
    n = n - 1
}
```
- Only self-recursion (not mutual recursion).
- Tail position: last expression in function body, last expression in
  both `if` branches, last expression in match arms.

---

## 7) Bytecode

### Instruction Set (68 instructions)

```
// Stack operations
CONST_INT(i64)          // push integer constant
CONST_FLOAT(f64)        // push float constant
CONST_TRUE              // push true
CONST_FALSE             // push false
CONST_UNIT              // push unit value
CONST_STRING(idx)       // push string from string pool
CONST_CHAR(u32)         // push char (Unicode scalar)
POP                     // discard top of stack
DUP                     // duplicate top of stack
SWAP                    // swap top two values

// Local variable access
LOAD_LOCAL(slot)        // push local variable
STORE_LOCAL(slot)       // pop into local variable
LOAD_UPVALUE(idx)       // push captured variable (closures)
STORE_UPVALUE(idx)      // store into captured variable

// Global access
LOAD_GLOBAL(idx)        // push module-level binding
STORE_GLOBAL(idx)       // store module-level binding

// Arithmetic (operate on top-of-stack)
ADD_INT                 // int + int
ADD_FLOAT               // float + float
SUB_INT, SUB_FLOAT
MUL_INT, MUL_FLOAT
DIV_INT, DIV_FLOAT      // div by zero → runtime error
MOD_INT, MOD_FLOAT
NEG_INT, NEG_FLOAT      // unary negate

// Comparison
EQ, NEQ                 // structural equality (any type)
LT_INT, LT_FLOAT
GT_INT, GT_FLOAT
LEQ_INT, LEQ_FLOAT
GEQ_INT, GEQ_FLOAT

// Logical
NOT                     // boolean not
// && and || compiled as branches (short-circuit)

// String
CONCAT_STRING           // pop two strings, push concatenation

// Control flow
JUMP(offset)            // unconditional jump
JUMP_IF_TRUE(offset)    // pop, jump if true
JUMP_IF_FALSE(offset)   // pop, jump if false
JUMP_TABLE(table_idx)   // indexed jump (for match on int/enum tag)

// Functions
CALL(arg_count)         // call function on stack with N args
CALL_METHOD(name_idx, arg_count)  // dispatch method
TAIL_CALL(arg_count)    // tail call (reuse frame)
RETURN                  // return top of stack
MAKE_CLOSURE(fn_idx, upvalue_count)  // create closure object

// Data structures
MAKE_ARRAY(count)       // pop N values, push array
MAKE_TUPLE(count)       // pop N values, push tuple
MAKE_STRUCT(type_idx, field_count)  // pop N values, push struct
MAKE_ENUM(type_idx, variant_idx, field_count)  // push enum variant
INDEX_GET               // pop index + collection, push element
INDEX_SET               // pop value + index + collection
FIELD_GET(field_idx)    // pop struct, push field value
FIELD_SET(field_idx)    // pop value + struct, set field

// Heap / GC
ALLOC_ARRAY(count)      // allocate array on heap
ALLOC_CLOSURE           // allocate closure on heap (GC-tracked)

// Pattern matching
TAG_CHECK(variant_idx)  // check enum tag, push bool
DESTRUCTURE(count)      // pop enum/tuple, push N fields onto stack

// Concurrency
CHANNEL_CREATE(cap)     // create channel, push handle
CHANNEL_SEND            // pop value + channel, send (may block)
CHANNEL_RECV            // pop channel, push received value (may block)
CHANNEL_CLOSE           // pop channel, close it
SPAWN(fn_idx)           // spawn goroutine-like green thread

// Built-ins
PRINT                   // pop value, print to stdout
PRINTLN                 // pop value, print with newline
INT_TO_FLOAT            // type conversion
FLOAT_TO_INT            // truncating conversion
INT_TO_STRING
FLOAT_TO_STRING
STRING_LEN              // push length of string
ARRAY_LEN               // push length of array

// Debug
BREAKPOINT              // VM pauses (debugger hook)
SOURCE_LOC(line, col)   // annotate for stack traces (no runtime effect)
```

### Bytecode Format (Binary)
```
Header:
  magic: [u8; 4]     = [0x52, 0x59, 0x58, 0x00]  // "RYX\\0"
  version: u16        = 1
  flags: u16          = 0 (reserved)

String Pool:
  count: u32
  entries: [
    len: u32
    data: [u8; len]   // UTF-8
  ]

Type Pool:
  count: u32
  entries: [TypeDescriptor]

Function Table:
  count: u32
  entries: [
    name_idx: u32       // index into string pool
    arity: u16
    locals_count: u16
    upvalue_count: u16
    max_stack: u16      // computed by compiler for stack sizing
    code_offset: u32    // byte offset into code section
    code_length: u32
    source_map_offset: u32   // offset into source map section
  ]

Code Section:
  raw bytes: instructions encoded as opcode (1 byte) + operands

Source Map:
  entries: [(bytecode_offset: u32, line: u32, col: u16)]

Entry Point:
  main_fn_idx: u32    // index of `main` function
```

### Instruction Encoding
- Opcode: 1 byte (0x00–0xFF)
- Operands: variable width depending on opcode
  - `slot`, `idx`, `count`: u16 (max 65535)
  - `offset`: i16 (relative jump, ±32767)
  - `CONST_INT`: i64 (8 bytes)
  - `CONST_FLOAT`: f64 (8 bytes)
  - `CONST_CHAR`: u32 (4 bytes)
  - `JUMP_TABLE`: u16 count + count × i16 offsets

---

## 8) Virtual Machine

Stack-based VM with green thread scheduler.

### Runtime Value Representation
```go
type Value struct {
    Tag  ValueTag   // 1 byte: Int, Float, Bool, Char, Unit, Object
    Data uint64     // for primitives: raw bits; for objects: heap pointer index
}

type HeapObject struct {
    Header  ObjectHeader
    Payload interface{}  // StringObj, ArrayObj, StructObj, EnumObj, ClosureObj, ChannelObj
}

type ObjectHeader struct {
    TypeID    uint32
    GCMark   uint8     // tricolor: White(0), Gray(1), Black(2)
    Size      uint32    // total allocation size in bytes
}

type ClosureObj struct {
    FnIndex   uint32
    Upvalues  []*UpvalueCell
}

type UpvalueCell struct {
    Location  *Value     // points to stack slot (open) or own storage (closed)
    Closed    Value      // storage for closed-over value
    IsOpen    bool
}

type ChannelObj struct {
    Buffer    []Value
    Capacity  int
    Closed    bool
    Senders   []*Fiber    // blocked senders
    Receivers []*Fiber    // blocked receivers
    Mutex     sync.Mutex
}
```

### Execution Engine
```go
type VM struct {
    Fibers       []*Fiber
    ActiveFiber  *Fiber
    Heap         *Heap
    Globals      []Value
    StringPool   []string
    Functions    []FnProto
    Scheduler    *Scheduler
    GC           *GarbageCollector
    DebugHooks   *DebugHooks
}

type Fiber struct {
    ID          uint64
    Frames      []CallFrame       // call stack (max depth 1024)
    Stack       []Value           // operand stack (max 65536)
    SP          int               // stack pointer
    State       FiberState        // Running, Suspended, Blocked, Dead
    BlockedOn   *ChannelObj       // if blocked on channel op
    Error       *RuntimeError     // if panicked
}

type CallFrame struct {
    Function   *FnProto
    IP         int          // instruction pointer (byte offset)
    BaseSlot   int          // stack base for this frame
    ReturnSlot int          // where to put return value
}
```

### Dispatch Loop
```go
func (vm *VM) Run() error {
    for {
        fiber := vm.Scheduler.NextReady()
        if fiber == nil {
            if vm.Scheduler.AllDead() {
                return nil  // program complete
            }
            // All fibers blocked → deadlock
            return ErrDeadlock
        }
        vm.ActiveFiber = fiber
        result := vm.ExecuteSlice(fiber, TIMESLICE_INSTRUCTIONS)
        switch result {
        case Yielded:
            vm.Scheduler.Enqueue(fiber)
        case Blocked:
            // fiber stays in blocked queue
        case Completed:
            fiber.State = Dead
        case Errored:
            return fiber.Error
        }
    }
}
```

- **Timeslice**: each fiber runs for 4096 instructions before yielding
  to the scheduler (cooperative preemption).
- **Dispatch**: `switch` on opcode. Each case: decode operands, execute,
  advance IP.
- **Stack overflow**: `SP > 65536` → runtime error with stack trace.
- **Call depth**: `len(Frames) > 1024` → runtime error with stack trace.
- **Division by zero**: runtime error with source location.
- **Index out of bounds**: runtime error with index and length.
- **Unwinding**: on error, walk call frames to produce stack trace with
  source locations (via source map).

### Scheduler
Cooperative M:1 green thread scheduler:
- Single OS thread runs all fibers.
- Ready queue: round-robin FIFO.
- Blocked fibers removed from ready queue, re-added when channel
  operation unblocks them.
- `spawn` creates new Fiber, adds to ready queue.
- Deadlock detection: if all living fibers are blocked and no fiber is
  ready → `ErrDeadlock` with list of blocked channels.

### Channel Semantics (CSP-style)
- Buffered channel: `channel<T>(cap)` — send blocks when buffer full,
  recv blocks when buffer empty.
- Unbuffered channel: `channel<T>(0)` — send and recv rendezvous
  (both block until matched).
- `ch.close()` — subsequent sends panic, subsequent recvs return
  remaining buffered values then `None`.
- `for val in ch { ... }` — receives until channel closed and empty.
- Multiple fibers can send/recv on same channel (fan-in, fan-out).

---

## 9) Garbage Collector

### Algorithm: Tricolor Mark-Sweep
Incremental, non-moving collector operating on the VM heap.

### Heap Structure
```go
type Heap struct {
    Objects    []*HeapObject     // all allocated objects
    FreeList   []int             // indices of freed slots
    BytesAlloc uint64            // total bytes currently allocated
    Threshold  uint64            // trigger GC when BytesAlloc > Threshold
    GCStats    GCStats
}

type GCStats struct {
    Collections     uint64
    TotalPauseNs    uint64
    TotalFreed      uint64
    MaxPauseNs      uint64
    LastCollectionNs uint64
}
```

### Collection Phases
1. **Root scanning**: scan all fiber stacks, all call frames, all globals,
   all open upvalue cells. Mark reachable objects Gray.
2. **Tracing**: while Gray set non-empty, pick a Gray object, mark it
   Black, mark all its references Gray (if White).
3. **Sweeping**: iterate all objects. White objects are garbage → free.
   Reset Black objects to White for next cycle.

### Incremental Mode
To avoid long pauses:
- Root scanning: done atomically (fast, just stack + globals).
- Tracing: process up to 256 objects per VM instruction slice. If not
  finished, resume next slice. Write barrier: if a Black object gets a
  new reference to a White object, re-mark it Gray.
- Sweeping: process up to 512 objects per slice.

### GC Triggers
- `BytesAlloc > Threshold` after any allocation.
- `Threshold` starts at 1MB, grows to `2 × BytesAlloc` after each GC
  (adaptive: grows with live set size).
- Manual trigger: `__gc()` built-in function.
- Emergency GC: if allocation fails (shouldn't happen with Go's memory,
  but safety net).

### Write Barrier
Required for incremental collection:
```go
func (heap *Heap) WriteBarrier(parent *HeapObject, child *HeapObject) {
    if parent.GCMark == Black && child.GCMark == White {
        parent.GCMark = Gray
        heap.GrayList = append(heap.GrayList, parent)
    }
}
```
Inserted by compiler at every STORE_UPVALUE, FIELD_SET, INDEX_SET,
and channel send.

---

## 10) Standard Library (Built-in)

Implemented in Go, exposed as built-in functions.

### Core Module
```ryx
// Type conversions
fn int_to_float(n: Int) -> Float
fn float_to_int(f: Float) -> Int
fn int_to_string(n: Int) -> String
fn float_to_string(f: Float) -> String
fn parse_int(s: String) -> Result<Int, String>
fn parse_float(s: String) -> Result<Float, String>

// I/O
fn print(val: String) -> Unit
fn println(val: String) -> Unit
fn read_line() -> String
fn read_file(path: String) -> Result<String, String>
fn write_file(path: String, content: String) -> Result<Unit, String>

// String operations
fn string_len(s: String) -> Int
fn string_slice(s: String, start: Int, end: Int) -> String
fn string_contains(haystack: String, needle: String) -> Bool
fn string_split(s: String, sep: String) -> [String]
fn string_trim(s: String) -> String
fn string_chars(s: String) -> [Char]
fn char_to_string(c: Char) -> String
fn string_replace(s: String, old: String, new: String) -> String
fn string_starts_with(s: String, prefix: String) -> Bool
fn string_ends_with(s: String, suffix: String) -> Bool
fn string_to_upper(s: String) -> String
fn string_to_lower(s: String) -> String

// Array operations
fn array_len<T>(arr: [T]) -> Int
fn array_push<T>(arr: [T], val: T) -> [T]    // returns new array
fn array_pop<T>(arr: [T]) -> ([T], Option<T>) // returns new array + popped
fn array_map<T, U>(arr: [T], f: (T) -> U) -> [U]
fn array_filter<T>(arr: [T], f: (T) -> Bool) -> [T]
fn array_fold<T, U>(arr: [T], init: U, f: (U, T) -> U) -> U
fn array_sort<T: Ord>(arr: [T]) -> [T]
fn array_reverse<T>(arr: [T]) -> [T]
fn array_contains<T: Eq>(arr: [T], val: T) -> Bool
fn array_zip<T, U>(a: [T], b: [U]) -> [(T, U)]
fn array_enumerate<T>(arr: [T]) -> [(Int, T)]
fn array_flat_map<T, U>(arr: [T], f: (T) -> [U]) -> [U]

// Math
fn abs_int(n: Int) -> Int
fn abs_float(f: Float) -> Float
fn min_int(a: Int, b: Int) -> Int
fn max_int(a: Int, b: Int) -> Int
fn min_float(a: Float, b: Float) -> Float
fn max_float(a: Float, b: Float) -> Float
fn sqrt(f: Float) -> Float
fn pow(base: Float, exp: Float) -> Float
fn floor(f: Float) -> Int
fn ceil(f: Float) -> Int
fn round(f: Float) -> Int
fn random_int(min: Int, max: Int) -> Int
fn random_float() -> Float

// Time
fn time_now_ms() -> Int       // milliseconds since epoch
fn sleep_ms(ms: Int) -> Unit  // blocks current fiber

// Assertions (for testing)
fn assert(cond: Bool, msg: String) -> Unit
fn assert_eq<T: Eq>(a: T, b: T) -> Unit
fn panic(msg: String) -> !    // never returns, unwind + stack trace
```

### Built-in Traits
```ryx
trait Eq {
    fn eq(self, other: Self) -> Bool;
}

trait Ord: Eq {
    fn cmp(self, other: Self) -> Int;  // -1, 0, 1
}

trait Display {
    fn to_string(self) -> String;
}

trait Default {
    fn default() -> Self;
}

trait Clone {
    fn clone(self) -> Self;
}

trait Hash {
    fn hash(self) -> Int;
}
```

All primitives auto-implement `Eq`, `Ord`, `Display`, `Default`, `Clone`, `Hash`.
Arrays and tuples implement these if their elements do.
Structs and enums implement these if all fields/variants do.

---

## 11) Error Reporting

### Compile-Time Errors
```
error[E001]: type mismatch
  --> src/main.ryx:12:15
   |
12 |     let x: Int = "hello";
   |            ---   ^^^^^^^ expected `Int`, found `String`
   |            |
   |            type annotation here

error[E002]: unknown variable `foo`
  --> src/main.ryx:8:5
   |
 8 |     foo + 1
   |     ^^^ not found in this scope
   |
   = help: did you mean `for`?

error[E003]: non-exhaustive match
  --> src/main.ryx:20:5
   |
20 |     match opt {
   |     ^^^^^ patterns not covered: `None`
   |
   = help: add a `None => ...` arm or a wildcard `_ => ...`

warning[W001]: unused variable
  --> src/main.ryx:5:9
   |
 5 |     let unused = 42;
   |         ^^^^^^ prefix with `_` to suppress this warning
```

### Runtime Errors
```
runtime error: division by zero
  --> src/main.ryx:15:20
   |
15 |     let x = 100 / n;
   |                    ^

stack trace:
  at divide(n: Int) -> Int         src/main.ryx:15
  at process(items: [Int]) -> Int  src/main.ryx:22
  at main()                        src/main.ryx:31

runtime error: index out of bounds (index: 5, length: 3)
  --> src/main.ryx:10:12
   |
10 |     arr[5]
   |         ^

runtime error: deadlock detected
  all 3 fibers blocked:
    fiber 0: blocked receiving on channel#1  src/main.ryx:40
    fiber 1: blocked receiving on channel#2  src/main.ryx:55
    fiber 2: blocked sending on channel#1    src/main.ryx:48
```

---

## 12) Configuration

All compiler/VM settings in `ryx_config.toml`:

```toml
[compiler]
max_errors = 20
max_warnings = 100
monomorphize_limit = 64
opt_level = 2                  # 0 = none, 1 = basic, 2 = full pipeline
inline_threshold = 8           # max MIR statements to inline
dump_ast = false
dump_hir = false
dump_mir = false
dump_bytecode = false

[vm]
stack_size = 65536
max_call_depth = 1024
fiber_timeslice = 4096         # instructions per fiber slice
max_fibers = 10000

[gc]
initial_threshold_bytes = 1048576    # 1 MB
growth_factor = 2.0
incremental_trace_batch = 256
incremental_sweep_batch = 512
enable_incremental = true

[debug]
source_maps = true
stack_traces = true
gc_stats = false
instruction_trace = false      # print every executed instruction (very slow)
```

---

## 13) Project Structure

```
ryx/
├── go.mod
├── go.sum
├── ryx_config.toml
├── README.md
├── cmd/
│   └── ryx/
│       └── main.go                    # CLI entry: compile, run, repl, disasm
├── pkg/
│   ├── lexer/
│   │   ├── lexer.go                   # Scanner: char-by-char tokenization
│   │   ├── token.go                   # Token type enum + Token struct
│   │   └── lexer_test.go             # Token stream tests
│   ├── parser/
│   │   ├── parser.go                  # Recursive descent + Pratt expression parser
│   │   ├── ast.go                     # All AST node types (40+)
│   │   ├── precedence.go             # Pratt precedence table + handlers
│   │   ├── error.go                   # Parse error types + recovery
│   │   └── parser_test.go            # Parse tree tests
│   ├── resolver/
│   │   ├── resolver.go                # Name resolution (two-pass)
│   │   ├── scope.go                   # Scope tree + Symbol table
│   │   ├── exhaustiveness.go          # Pattern exhaustiveness checker
│   │   └── resolver_test.go          # Resolution + exhaustiveness tests
│   ├── types/
│   │   ├── types.go                   # Type representation (all type nodes)
│   │   ├── inference.go               # Constraint generation + unification
│   │   ├── traits.go                  # Trait resolution + coherence checking
│   │   ├── checker.go                 # Type checking driver (walks AST)
│   │   └── types_test.go             # Type inference + checking tests
│   ├── hir/
│   │   ├── hir.go                     # HIR node types
│   │   ├── lower.go                   # AST → HIR lowering + desugaring
│   │   ├── monomorphize.go            # Generic instantiation
│   │   ├── match_compile.go           # Pattern match → decision tree
│   │   └── hir_test.go               # Lowering + monomorphization tests
│   ├── mir/
│   │   ├── mir.go                     # MIR types (BasicBlock, Stmt, Terminator)
│   │   ├── builder.go                 # HIR → MIR SSA construction
│   │   ├── printer.go                 # MIR pretty-printer (for debugging)
│   │   └── mir_test.go               # MIR construction tests
│   ├── optimize/
│   │   ├── pipeline.go                # Optimization pass runner
│   │   ├── constant_fold.go           # Constant folding
│   │   ├── dead_code.go               # Dead code elimination
│   │   ├── copy_prop.go               # Copy propagation
│   │   ├── cse.go                     # Common subexpression elimination
│   │   ├── inline.go                  # Function inlining
│   │   ├── const_prop.go              # Constant propagation
│   │   ├── licm.go                    # Loop-invariant code motion
│   │   ├── tco.go                     # Tail call optimization
│   │   ├── escape.go                  # Escape analysis
│   │   ├── block_merge.go             # Block merging
│   │   └── optimize_test.go          # Tests per pass + combined pipeline
│   ├── codegen/
│   │   ├── codegen.go                 # MIR → bytecode emission
│   │   ├── bytecode.go                # Instruction set definitions
│   │   ├── encoder.go                 # Binary bytecode format writer
│   │   ├── decoder.go                 # Binary bytecode format reader
│   │   ├── disasm.go                  # Bytecode disassembler (human-readable)
│   │   └── codegen_test.go           # Bytecode emission + round-trip tests
│   ├── vm/
│   │   ├── vm.go                      # VM core: dispatch loop, stack operations
│   │   ├── fiber.go                   # Fiber struct, call frames
│   │   ├── scheduler.go               # Green thread scheduler
│   │   ├── value.go                   # Runtime value representation
│   │   ├── heap.go                    # Heap allocation + object headers
│   │   ├── channel.go                 # Channel implementation (CSP semantics)
│   │   ├── builtins.go                # Built-in function implementations
│   │   ├── error.go                   # Runtime error types + stack trace builder
│   │   └── vm_test.go                # VM execution tests
│   ├── gc/
│   │   ├── gc.go                      # Tricolor mark-sweep collector
│   │   ├── write_barrier.go           # Write barrier for incremental GC
│   │   └── gc_test.go                # GC correctness + pause time tests
│   ├── stdlib/
│   │   ├── core.go                    # Core built-in functions
│   │   ├── string_ops.go             # String operations
│   │   ├── array_ops.go              # Array operations
│   │   ├── math_ops.go               # Math functions
│   │   ├── io.go                      # File I/O
│   │   └── stdlib_test.go            # Standard library tests
│   ├── diagnostic/
│   │   ├── diagnostic.go              # Error/warning message formatting
│   │   ├── source.go                  # Source file registry + span → line/col
│   │   └── diagnostic_test.go        # Error message formatting tests
│   └── repl/
│       ├── repl.go                    # Interactive REPL loop
│       └── repl_test.go              # REPL session tests
├── tests/
│   ├── testdata/
│   │   ├── lexer/                     # .ryx files + expected .tokens files
│   │   │   ├── literals.ryx
│   │   │   ├── operators.ryx
│   │   │   ├── keywords.ryx
│   │   │   ├── strings.ryx
│   │   │   ├── comments.ryx
│   │   │   ├── unicode.ryx
│   │   │   └── errors.ryx
│   │   ├── parser/                    # .ryx files + expected .ast.json files
│   │   │   ├── expressions.ryx
│   │   │   ├── statements.ryx
│   │   │   ├── functions.ryx
│   │   │   ├── types.ryx
│   │   │   ├── patterns.ryx
│   │   │   ├── generics.ryx
│   │   │   ├── traits.ryx
│   │   │   ├── closures.ryx
│   │   │   ├── concurrency.ryx
│   │   │   └── error_recovery.ryx
│   │   ├── typecheck/                 # .ryx files + expected errors or success
│   │   │   ├── inference_basic.ryx
│   │   │   ├── inference_generics.ryx
│   │   │   ├── traits.ryx
│   │   │   ├── exhaustiveness.ryx
│   │   │   ├── errors.ryx
│   │   │   └── complex_types.ryx
│   │   ├── optimize/                  # .mir + expected optimized .mir
│   │   │   ├── constant_fold.mir
│   │   │   ├── dead_code.mir
│   │   │   ├── inline.mir
│   │   │   ├── tco.mir
│   │   │   └── escape.mir
│   │   └── programs/                  # Full .ryx programs + expected output
│   │       ├── hello.ryx
│   │       ├── fibonacci.ryx
│   │       ├── quicksort.ryx
│   │       ├── binary_tree.ryx
│   │       ├── linked_list.ryx
│   │       ├── hash_map.ryx
│   │       ├── closures.ryx
│   │       ├── pattern_matching.ryx
│   │       ├── traits.ryx
│   │       ├── generics_complex.ryx
│   │       ├── channels_basic.ryx
│   │       ├── channels_fanout.ryx
│   │       ├── producer_consumer.ryx
│   │       ├── dining_philosophers.ryx
│   │       ├── pipeline.ryx
│   │       ├── adt_expression.ryx
│   │       ├── game_of_life.ryx
│   │       ├── json_parser.ryx
│   │       ├── brainfuck_interp.ryx
│   │       ├── mandelbrot.ryx
│   │       ├── red_black_tree.ryx
│   │       ├── sieve_of_eratosthenes.ryx
│   │       ├── graph_bfs.ryx
│   │       └── matrix_multiply.ryx
│   ├── integration/
│   │   ├── lexer_golden_test.go       # Golden file tests for lexer
│   │   ├── parser_golden_test.go      # Golden file tests for parser
│   │   ├── typecheck_golden_test.go   # Golden file tests for type checker
│   │   ├── optimize_golden_test.go    # Golden file tests for optimizer
│   │   ├── e2e_test.go               # Compile + run programs, check output
│   │   ├── error_message_test.go      # Verify error messages are helpful
│   │   ├── concurrency_test.go        # Multi-fiber correctness tests
│   │   ├── gc_stress_test.go          # GC under allocation pressure
│   │   ├── stdlib_test.go             # Standard library integration
│   │   └── regression_test.go         # Bug regression tests
│   └── benchmark/
│       ├── bench_lexer_test.go        # Tokens/sec on large input
│       ├── bench_parser_test.go       # AST nodes/sec
│       ├── bench_typecheck_test.go    # Type checking throughput
│       ├── bench_codegen_test.go      # Bytecode emission speed
│       ├── bench_vm_test.go           # VM instruction throughput
│       ├── bench_gc_test.go           # GC pause times + throughput
│       ├── bench_programs_test.go     # End-to-end program benchmarks
│       └── bench_fibonacci_test.go    # fib(35) execution time
└── examples/
    ├── hello.ryx
    ├── calculator.ryx
    ├── todo_list.ryx
    ├── concurrent_primes.ryx
    └── expression_evaluator.ryx
```

---

## 14) Test Suite

### Unit Tests

**lexer_test.go**
- All 52 token types produced from representative input
- Integer literal bases: decimal, hex, binary, octal
- Underscore separators: `1_000`, `0xFF_FF`
- Float literals: `3.14`, `1e10`, `2.5e-3`
- String escapes: all 7 escape sequences
- Raw strings: no escape processing
- Character literals: ASCII, Unicode escape
- Nested block comments: `/* /* */ */`
- Line + column tracking across multi-line input
- Source spans: correct byte offsets for every token
- UTF-8 identifiers: `café`, `naïve`, `日本語`
- Error recovery: malformed token → ERROR token with message, scanning continues
- Keywords vs identifiers: `fn` is keyword, `fn_name` is ident
- Contextual: `self` recognized in both keyword and pattern contexts
- Empty input → single EOF token
- Consecutive tokens with no whitespace: `1+2` → INT_LIT PLUS INT_LIT
- Maximum-munch: `..` is RANGE, `..=` is RANGE_INCLUSIVE, `.` is DOT

**parser_test.go**
- Pratt precedence: `1 + 2 * 3` → `Add(1, Mul(2, 3))`
- All 14 precedence levels with associativity verified
- Every expression form: if, match, for, while, loop, lambda, spawn, channel
- Pipe operator: `a |> f |> g` → `Call(g, Call(f, a))`
- Every statement form: let, let mut, expression statement, return
- Every item form: fn, type, struct, trait, impl, import, module
- Generic syntax: `fn foo<T: Trait1 + Trait2>(x: T) -> T`
- Pattern matching: all pattern forms, nested, with guards
- Struct literals: `Point { x: 1, y: 2 }`
- Array literals: `[1, 2, 3]`
- Tuple literals: `(1, "hello", true)`
- Closure: `|x: Int, y: Int| -> Int { x + y }`
- Closure type inference: `|x| x + 1` (parameter type inferred)
- Block expression: `{ let x = 1; x + 2 }` value is `3`
- Method call chaining: `a.foo().bar(x).baz()`
- Index chaining: `a[0][1]`
- Error recovery: 20 errors then stop; synchronize correctly
- Missing semicolons: auto-insert where unambiguous
- Unclosed delimiters: report opener location
- Empty function body: `fn foo() {}`
- Trailing commas: allowed in all comma-separated lists
- AST span preservation: every node has correct source span

**resolver_test.go**
- Variable use before declaration → error
- Function hoisting: call before definition → ok
- Nested scopes: inner shadows outer → warning
- Same-scope redeclaration → error
- Unused variable → warning
- Unused `mut` → warning "does not need to be mutable"
- `self` outside impl → error
- `Self` outside trait/impl → error
- Type name resolution: all primitive types, user-defined types, generics
- Variant constructor resolution: `Leaf(x)` → parent enum type
- Trait method resolution to correct impl
- `break` outside loop → error
- `continue` outside loop → error
- Import resolution: module path → symbols
- Generic type parameter scoping: visible in fn body, not outside
- Exhaustiveness: missing enum variant → error
- Exhaustiveness: wildcard covers all → ok
- Exhaustiveness: unreachable arm after wildcard → warning
- Exhaustiveness: nested patterns checked recursively
- Exhaustiveness: guard clauses make arm non-exhaustive

**types_test.go**
- Literal types: `42` is `Int`, `3.14` is `Float`, `true` is `Bool`
- Let inference: `let x = 42;` → `x: Int`
- Let annotation: `let x: Float = 42;` → type mismatch error
- Function return type inferred from body
- Generic function: type variables unified across call
- Monomorphization: `identity<Int>` and `identity<String>` produce separate instances
- Trait bound checking: `T: Summable` required for `+` on generic
- Missing trait impl → error with helpful message
- Coherence: overlapping impl → error
- Orphan rule: impl in wrong module → error
- If/else type: both branches must unify
- Match arm types: all arms must unify
- Array element types: all elements must unify
- Closure capture types: correctly inferred from environment
- Channel type: `send`/`recv` types must match channel element type
- Recursive types: `type List<T> { Nil, Cons(T, List<T>) }` → ok
- Mutually recursive types → ok
- Infinite type (occurs check) → error
- `spawn` body must return `Unit`
- Higher-order function types: `fn apply(f: (Int) -> Int, x: Int) -> Int`
- Nested generics: `Option<[T]>`, `Result<Option<Int>, String>`
- Type alias: `type Pair<T> = (T, T)` → transparent alias
- Monomorphization limit: >64 instances → error

**hir_test.go**
- Desugar `for x in a..b` to while loop with counter
- Desugar `for x in ch` to loop with channel recv
- Desugar `x.method(y)` to `Type::method(x, y)`
- Desugar `a |> f |> g` to `g(f(a))`
- Desugar `a ++ b` to `String::concat(a, b)`
- Pattern match compilation to decision tree: correct dispatch
- Decision tree: no redundant checks
- Decision tree: respects guard clauses
- Closure capture: correct variables identified
- Closure: captured vars become struct fields
- Monomorphize: generic fn → concrete fn per type combination
- Monomorphize: generic struct → concrete struct per type
- All type annotations resolved (no TypeVar remaining)

**mir_test.go**
- SSA construction: each variable assigned exactly once
- Control flow graph: correct block structure for if/else
- CFG: correct structure for while loop (header, body, exit blocks)
- CFG: correct structure for match (one block per arm)
- CFG: nested if/match flattened correctly
- Phi nodes: placed at join points for variables defined in branches
- All terminators: Goto, Branch, Switch, Return, Unreachable
- Function calls correctly represented
- Heap allocation instructions for closures, arrays, ADTs

**optimize_test.go**
- Constant folding: all arithmetic rules verified
- Constant folding: boolean short-circuit `true && x` → `x`
- Dead code: unreachable block removed
- Dead code: unused assignment removed
- Copy propagation: `x = y; use(x)` → `use(y)`
- CSE: `a + b; ... a + b` → compute once, reuse
- Inlining: function ≤ 8 stmts inlined at call site
- Inlining: recursive function NOT inlined
- Constant propagation: `let x = 5; y = x + 3` → `y = 8`
- LICM: loop-invariant `len(arr)` hoisted before loop
- TCO: tail-recursive factorial converted to loop
- TCO: non-tail-recursive function left as recursive call
- Escape analysis: locally-created array not sent/returned → stack
- Escape analysis: array returned from function → heap
- Escape analysis: value sent on channel → heap
- Block merging: single-pred/single-succ blocks merged
- Full pipeline: combine all passes, verify correctness preserved
- Optimization preserves program semantics (run before + after, compare output)

**codegen_test.go**
- Each opcode encoded/decoded correctly (round-trip)
- CONST_INT: correct 8-byte encoding
- CONST_FLOAT: correct IEEE 754 encoding
- JUMP: correct relative offset encoding
- JUMP_TABLE: correct count + offsets
- String pool: strings deduplicated
- Function table: correct offsets into code section
- Source map: correct bytecode-to-source mapping
- Disassembler output matches expected text
- Bytecode file header: magic number, version
- Binary format round-trip: encode → decode → encode = identical bytes
- Max stack depth correctly computed per function

**vm_test.go**
- Each instruction executes correctly (unit test per opcode)
- Stack operations: push, pop, dup, swap
- Arithmetic: all ops with edge cases (overflow, div-by-zero)
- Comparison: all ops, all types
- String concatenation: empty strings, unicode
- Function calls: correct frame setup, argument passing, return
- Recursive calls: stack frames accumulate correctly
- Tail calls: frame reuse (stack doesn't grow)
- Closures: upvalue capture and access
- Closures: upvalue closing when stack frame exits
- Arrays: create, index, set, bounds checking
- Tuples: create, destructure
- Structs: create, field access, field set
- Enums: create, tag check, destructure
- Pattern matching: correct dispatch via decision tree
- Channel: send/recv unbuffered (rendezvous)
- Channel: send/recv buffered
- Channel: close + drain
- Channel: recv on closed empty channel → None
- Channel: send on closed channel → runtime error
- Fiber scheduling: round-robin order
- Fiber timeslicing: yield after N instructions
- Deadlock detection: all fibers blocked → error
- Stack overflow: >65536 values → error with trace
- Call depth overflow: >1024 frames → error with trace
- Runtime error: source location included in message
- Stack trace: correct unwinding through multiple frames

**gc_test.go**
- Unreachable objects collected after GC cycle
- Reachable objects survive GC
- Circular references collected (not reference counting)
- GC triggered at threshold
- Threshold grows after collection (adaptive)
- Incremental: tracing spread across multiple slices
- Write barrier: Black→White re-marked Gray
- Stress: allocate 100K objects, verify only reachable survive
- Concurrent fibers + GC: no dangling pointers
- GC stats: correct counts after multiple cycles
- Open upvalues: not collected while stack frame alive
- Closed upvalues: collected when closure dead
- Channel buffers: values in transit not collected

**stdlib_test.go**
- All string operations: boundary cases, empty strings, Unicode
- All array operations: empty arrays, single element, large arrays
- Math functions: edge cases (sqrt negative, pow overflow)
- parse_int/parse_float: valid input, invalid input, edge cases
- I/O: read_file/write_file on temp files
- assert/assert_eq: pass and fail cases
- Type conversions: int↔float↔string round-trips

**diagnostic_test.go**
- Error message formatting matches expected output
- Source span → line/col conversion correct
- Multi-line error underlining correct
- "did you mean?" suggestions for close misspellings
- Warning formatting distinct from errors
- Multiple errors accumulated and displayed

### Integration Tests

**e2e_test.go**
For each `.ryx` file in `tests/testdata/programs/`:
- Compile to bytecode
- Run in VM
- Capture stdout
- Assert matches expected output (`.expected` file)

Programs tested (24):
- `hello.ryx` — basic output
- `fibonacci.ryx` — recursive + iterative, verify first 20 values
- `quicksort.ryx` — generic quicksort on [Int] and [String]
- `binary_tree.ryx` — insert, search, in-order traversal with ADT
- `linked_list.ryx` — cons list, map, fold, reverse
- `hash_map.ryx` — open-addressing hash map implementation in Ryx
- `closures.ryx` — adder factory, counter, compose, partial application
- `pattern_matching.ryx` — nested patterns, guards, exhaustive coverage
- `traits.ryx` — trait definition, impl, generic dispatch
- `generics_complex.ryx` — multi-param generics, nested generic types
- `channels_basic.ryx` — send/recv, buffered/unbuffered
- `channels_fanout.ryx` — 1 producer, N consumers
- `producer_consumer.ryx` — bounded buffer via channels
- `dining_philosophers.ryx` — deadlock-free solution using channels
- `pipeline.ryx` — multi-stage pipeline (generate → filter → transform → collect)
- `adt_expression.ryx` — expression evaluator with ADT (Num, Add, Mul, Neg)
- `game_of_life.ryx` — N-step Conway's Game of Life (grid as array)
- `json_parser.ryx` — simple JSON parser returning ADT
- `brainfuck_interp.ryx` — brainfuck interpreter in Ryx
- `mandelbrot.ryx` — ASCII art mandelbrot set
- `red_black_tree.ryx` — self-balancing BST with pattern matching
- `sieve_of_eratosthenes.ryx` — concurrent sieve with channels
- `graph_bfs.ryx` — BFS on adjacency list graph
- `matrix_multiply.ryx` — NxN matrix multiplication

**concurrency_test.go**
- 100 fibers all incrementing shared counter via channel → correct total
- Fan-out: 1 sender, 10 receivers, all values received exactly once
- Fan-in: 10 senders, 1 receiver, all values received
- Pipeline: 5-stage pipeline, correct order and values
- Channel close: receivers see all buffered values then stop
- Deadlock detection: two fibers mutually blocked → error
- No-deadlock false positive: temporarily blocked fibers recover
- Fiber spawn ordering: spawned fibers execute in reasonable order
- High fiber count: 1000 fibers, all complete
- Sleep: fiber sleeps, others continue executing

**gc_stress_test.go**
- Allocate 1M small objects, keep 1K alive, verify GC reclaims rest
- Closure chain: 10000 closures, each capturing previous, verify GC
  doesn't prematurely collect
- Concurrent allocation: multiple fibers allocating simultaneously
- Write barrier correctness: modify heap objects during GC, verify integrity
- GC pause time: p99 < 10ms under sustained allocation pressure (1M objects/sec)
- GC throughput: reclaim rate > 100K objects/sec

**error_message_test.go**
- Type mismatch: correct expected/found types shown
- Unknown variable: "did you mean?" suggestion shown
- Missing trait impl: trait and type names shown
- Non-exhaustive match: missing variants listed
- Duplicate definition: location of first definition shown
- Runtime error: source location + stack trace present

**regression_test.go**
- (Populated as bugs are found during development)
- Each test case: minimal reproduction, expected behavior

### Golden File Tests

**lexer_golden_test.go**
- For each `tests/testdata/lexer/*.ryx`: lex file, compare token stream
  against `*.tokens` golden file. `go test -update` flag regenerates.

**parser_golden_test.go**
- For each `tests/testdata/parser/*.ryx`: parse file, compare AST JSON
  against `*.ast.json` golden file.

**typecheck_golden_test.go**
- For each `tests/testdata/typecheck/*.ryx`: type check, compare
  diagnostics against `*.diagnostics` golden file.

**optimize_golden_test.go**
- For each `tests/testdata/optimize/*.mir`: optimize, compare result
  against `*.optimized.mir` golden file.

### Benchmarks (`go test -bench`)

- **Lexer throughput**: tokens/sec on 10K-line source file (target: >5M tokens/sec)
- **Parser throughput**: AST nodes/sec on 10K-line source (target: >500K nodes/sec)
- **Type checking**: expressions/sec (target: >200K/sec)
- **Codegen**: bytecode bytes/sec (target: >1M bytes/sec)
- **VM dispatch**: instructions/sec (target: >100M instr/sec)
- **VM fibonacci**: `fib(35)` execution time (target: < 2s)
- **VM quicksort**: sort 10K integers (target: < 500ms)
- **GC pause**: p99 pause time with 1M live objects (target: < 10ms)
- **GC throughput**: objects freed/sec (target: > 100K/sec)
- **Compile + run**: end-to-end time for `fibonacci.ryx` (target: < 100ms)

---

## 15) CLI Interface

```bash
# Compile and run
ryx run program.ryx

# Compile to bytecode file
ryx build program.ryx -o program.ryxc

# Run compiled bytecode
ryx exec program.ryxc

# Interactive REPL
ryx repl

# Disassemble bytecode
ryx disasm program.ryxc

# Dump intermediate representations
ryx run program.ryx --dump-ast
ryx run program.ryx --dump-hir
ryx run program.ryx --dump-mir
ryx run program.ryx --dump-bytecode

# Run with GC stats
ryx run program.ryx --gc-stats

# Run with instruction trace (very verbose)
ryx run program.ryx --trace

# Type check only (no execution)
ryx check program.ryx

# Format source code (bonus feature)
ryx fmt program.ryx
```

### REPL Features
- Compile and execute expressions interactively
- Persistent state across inputs (variables survive between entries)
- Multi-line input: open brace → continue on next line
- `:type expr` — show inferred type without executing
- `:ast expr` — show parsed AST
- `:bytecode expr` — show generated bytecode
- `:quit` — exit
- Arrow key history

---

## 16) Deliverables

Source code for:
- Lexer (hand-written scanner, 52 token types, UTF-8 aware, error recovery)
- Parser (recursive descent + Pratt, 14 precedence levels, 40+ AST nodes,
  error recovery with synchronization)
- Name resolver (two-pass, scope tree, exhaustiveness checker)
- Type system (Hindley-Milner inference, trait resolution, coherence
  checking, exhaustiveness, monomorphization)
- HIR lowering (desugaring, pattern match compilation, closure transform)
- MIR construction (SSA form, CFG, phi nodes)
- Optimization passes (11 passes: constant folding, DCE, copy propagation,
  CSE, inlining, constant propagation, LICM, TCO, escape analysis, DCE2,
  block merging)
- Bytecode codegen (68-instruction ISA, binary format, source maps)
- Virtual machine (stack-based, fiber scheduler, channel CSP semantics)
- Garbage collector (tricolor incremental mark-sweep, write barriers)
- Standard library (50+ built-in functions, 6 built-in traits)
- Error reporting (compile-time and runtime, source spans, stack traces,
  suggestions)
- CLI (compile, run, REPL, disassemble, check, dump IR)
- Full test suite (unit + golden + integration + concurrency + GC stress
  + benchmarks)
- 24 example programs from hello world to concurrent sieve
- TOML configuration with documented defaults

README.md including:
- Language tutorial (syntax, types, pattern matching, traits, closures,
  concurrency, error handling)
- Architecture overview (compiler pipeline diagram)
- How to build and run
- REPL usage guide
- Bytecode instruction set reference
- Optimization passes explained
- GC design and tuning
- Standard library reference
- CLI reference
- Test suite description + `go test` instructions
- Benchmark results and performance notes
- Config reference
- Grammar specification (EBNF)
- Known limitations / future ideas (module system, incremental compilation,
  JIT, debugger, LSP server, package manager)