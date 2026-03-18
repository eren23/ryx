# Hello World

The smallest shipped program is [`examples/hello.ryx`](https://github.com/ryx-lang/ryx/blob/main/examples/hello.ryx).

```ryx
fn greet(name: String) -> String {
    "Hello, " ++ name ++ "!"
}

fn main() {
    println(greet("World"));
    println(greet("Ryx"));

    let x = 42;
    println(x);

    let is_even = x % 2 == 0;
    println(is_even)
}
```

## Run It

```bash
./ryx run examples/hello.ryx
```

Observed output:

```text
"Hello, ""World""!"
"Hello, ""Ryx""!"
42
true
```

## What It Demonstrates

- Function definitions
- String concatenation with `++`
- Local bindings
- Arithmetic and boolean expressions
- `main` as the program entry point
