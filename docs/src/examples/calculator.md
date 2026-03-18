# Calculator

[`examples/calculator.ryx`](https://github.com/ryx-lang/ryx/blob/main/examples/calculator.ryx) demonstrates algebraic data types and recursive evaluation via pattern matching.

## Source

```ryx
type Expr {
    Num(Float),
    Add(Expr, Expr),
    Sub(Expr, Expr),
    Mul(Expr, Expr),
    Div(Expr, Expr),
}

fn eval(expr: Expr) -> Float {
    match expr {
        Num(n) => n,
        Add(a, b) => eval(a) + eval(b),
        Sub(a, b) => eval(a) - eval(b),
        Mul(a, b) => eval(a) * eval(b),
        Div(a, b) => {
            let divisor = eval(b);
            if divisor == 0.0 { 0.0 } else { eval(a) / divisor }
        },
    }
}
```

## Run It

```bash
./ryx run examples/calculator.ryx
```

Observed output:

```text
20
5
21
```

## Try a Type-Check Only Pass

```bash
./ryx check examples/calculator.ryx
```

Observed output:

```text
check: no errors
```

## What It Demonstrates

- Enum variants with payloads
- Recursive functions
- Exhaustive `match`
- Block expressions inside match arms
