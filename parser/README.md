# LL vs LR vs Recursive Descent Parser Comparison

## LL Parsers (Left-to-right, Leftmost derivation)

### How they work:
- **Top-down** parsing approach
- Read input **left-to-right**
- Build **leftmost derivation** of the parse tree
- Use a **stack** and **parsing table**
- Make decisions based on **current state** and **next k tokens** (LL(k))

### Algorithm:
```
1. Start with start symbol on stack
2. While stack not empty:
   - If top of stack is terminal: match with input
   - If top of stack is non-terminal: 
     - Look up production rule in table using (non-terminal, lookahead)
     - Replace non-terminal with right-hand side of production
3. Accept if stack empty and input consumed
```

### Example LL(1) Grammar:
```
E → T E'
E' → + T E' | ε
T → F T'
T' → * F T' | ε
F → ( E ) | id
```

### Limitations:
- Cannot handle **left recursion** (A → A α)
- Cannot handle **ambiguous grammars**
- Needs **left factoring** for common prefixes
- LL(1) is restrictive - many languages need more lookahead

---

## LR Parsers (Left-to-right, Rightmost derivation)

### How they work:
- **Bottom-up** parsing approach
- Read input **left-to-right**
- Build **rightmost derivation** (in reverse)
- Use **shift-reduce** algorithm with parsing table
- More powerful than LL - can handle left recursion

### Algorithm:
```
1. Start with empty stack and input
2. While not accepted:
   - SHIFT: Move input token to stack
   - REDUCE: Replace stack top with left-hand side of production rule
   - Decision made by consulting parsing table
3. Accept when stack contains only start symbol
```

### Example LR Grammar:
```
E → E + T | T
T → T * F | F
F → ( E ) | id
```
*(Note: This has left recursion, which LL can't handle)*

---

## Recursive Descent with Precedence Climbing

### How it works:
- **Top-down** parsing approach
- **Hand-written** recursive functions
- Each grammar rule can become a function
- Uses **precedence climbing** for expressions

### Characteristics:
```go
// At the top of the chain we just have a way to parse statements and expressions
func (ps *parser) parseStmt() ast.Node
func (ps *parser) parseExpr() ast.Node
// We also need a way to climb precedence
func (ps *parser) parseExprRest(left ast.Node, currentPrecedence int) ast.Node
```

The idea is that the main loop will call `parseStmt` in a repeatedly. Then `parseStmt` will try to parse statements like variable/function declarations and also function calls which are technically both a statement and an expression. The `parseExpr` function parses a single `ast.Node` and then calls `parseExprRest` with that as the `left` argument and `0` as `currentPrecedence`. Then `parseExprRest` will *try* to parse the rest of the expression (if there is any). This is how precedence climbing is implemented.

---

## Comparison Table

| Aspect | LL(1) | LR(1) | Recursive Descent |
|--------|-------|---------------|----------------------|
| **Direction** | Top-down | Bottom-up | Top-down |
| **Implementation** | Table-driven | Table-driven | Hand-written |
| **Left Recursion** | Cannot handle ❌ | Can handle ✅ | Can handle ✅ (with precedence climbing) |
| **Grammar Flexibility** | Not flexible | More flexible | As flexible as you make it |
| **Error Recovery** | Harder | Harder | Easier (custom error messages) |
| **Expressiveness** | Very limited | Limited  | As expressive as you make it |

---

## Power Hierarchy

```
LR(1) > LL(1)
```

**Recursive descent** sits outside this hierarchy - it's more like **LL(∞)** because you can look ahead as far as needed and make complex decisions.

---

## When to Use Each

### LL Parsers:
- Simple languages with clear structure
- When you need guaranteed linear time parsing
- Educational purposes (easier to understand)

### LR Parsers:
- Complex programming languages
- When you need to handle left recursion naturally
- When using parser generators (Yacc, Bison)

### Recursive Descent:
- When you need excellent error messages
- When you want full control over parsing decisions
- Examples: GCC, Clang, Rust compiler, Go compiler



## Why choose Recursive Descent

1. **Flexibility**: Can handle any grammar you can code
2. **Error Recovery**: Easy to add custom error messages
3. **Debugging**: Step through with debugger
4. **Performance**: Direct function calls, no table lookups
5. **Maintainability**: Grammar changes = code changes
6. **Integration**: Easy to add semantic actions during parsing

Recursive Descent is imo the **most practical** approach for most real-world parsers that will be used by actual humans directly. We depend on user-friendly error messages!