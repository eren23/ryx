# Language Guide

Ryx is expression-oriented: most constructs produce values, and the final expression in a block becomes the result.

## Variables and Mutability

```ryx
let x = 42;
let y: Float = 3.14;
let mut count = 0;
count = count + 1;
```

- Bindings are immutable by default
- `mut` enables reassignment
- Type annotations are optional when inference is sufficient

## Primitive Types

| Type | Notes |
|------|-------|
| `Int` | 64-bit signed integer |
| `Float` | 64-bit floating point |
| `Bool` | `true` or `false` |
| `Char` | Unicode scalar value |
| `String` | UTF-8 string |
| `Unit` | Implicit no-value result |

## Functions and Control Flow

```ryx
fn add(a: Int, b: Int) -> Int {
    a + b
}

let max = if a > b { a } else { b };

while i < 10 {
    println(i);
    i = i + 1;
};
```

- The last expression in a function or block is the return value
- `return` is only needed for early exit
- `if`, `match`, and blocks are value-producing expressions

## Structs and Enums

```ryx
struct Point {
    x: Float,
    y: Float,
}

type Option<T> {
    Some(T),
    None,
}
```

Structs model named fields. Enums are algebraic data types with variants and optional payloads.

## Pattern Matching

```ryx
fn area(shape: Shape) -> Float {
    match shape {
        Circle(r) => 3.14159 * r * r,
        Rectangle(w, h) => w * h,
        Triangle(a, b, c) => {
            let s = (a + b + c) / 2.0;
            sqrt(s * (s - a) * (s - b) * (s - c))
        },
    }
}
```

Supported patterns include:

- Wildcards: `_`
- Bindings: `x`
- Literals
- Tuples
- Enum variants
- Struct destructuring
- Or-patterns
- Range patterns

## Traits and Closures

```ryx
trait Describable {
    fn describe(self) -> String;
}

let add = |a: Int, b: Int| -> Int { a + b };
```

Traits define shared behavior; closures allow small inline functions and higher-order operations.

## Arrays and Tuples

```ryx
let nums = [1, 2, 3, 4, 5];
let pair = (42, "hello");
println(nums[0]);
println(array_len(nums));
```

- Arrays are homogeneous
- Tuples are fixed-size and heterogeneous

## Type Casting

```ryx
let x: Int = 42;
let f = x as Float;
let s = int_to_string(x);
```

## Error Handling

Ryx uses value-based error handling instead of exceptions.

```ryx
type Result<T, E> {
    Ok(T),
    Err(E),
}

match safe_divide(10.0, 3.0) {
    Ok(val) => println(val),
    Err(msg) => println(msg),
}
```

## Concurrency

```ryx
fn worker(id: Int, ch: channel<Int>) {
    ch.send(id * 10);
}

fn main() {
    let ch = channel<Int>(4);
    spawn { worker(1, ch) };
    let val = ch.recv();
    println(val);
}
```

- `spawn { ... }` launches a new fiber
- `channel<T>(cap)` creates a channel
- `send` and `recv` synchronize between fibers

## Operators

| Category | Operators |
|----------|-----------|
| Arithmetic | `+`, `-`, `*`, `/`, `%` |
| Comparison | `==`, `!=`, `<`, `>`, `<=`, `>=` |
| Logical | `&&`, `||`, `!` |
| String | `++` |
| Pipe | `|>` |
| Range | `..`, `..=` |
