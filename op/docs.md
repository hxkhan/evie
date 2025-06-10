## Flat AST Representation

This is the format that `NewProgram()` expects the code in!

`0: NULL`
- Size `1` byte
- Returns the value `null`

`1: EXIT`
- Size `1` byte
- Exit the application with status code `0`

`2: OUT`
- Size `1` byte
- Print the next value to stdout

`3: INT`
- Size `1+8` bytes
- Returns the next `8` bytes interpreted as a `64` bit integer

`4: FLOAT`
- Size `1+8` bytes
- Returns the next `8` bytes interpreted as a `64` bit float encoded in IEEE 754

`5: STR`
- Size `1+2+len(string)` bytes
- The `2` extra bytes is a `u16` that tells the length of the string
- Which also means strings can have a maximum length of `65535`
- Returns the string

`6: TRUE`
- Size `1` byte
- Returns the value `true`

`7: FALSE`
- Size `1` byte
- Returns the value `false`

`8: ADD` `9: SUB` `10: DIV` `11: MUL`
- Size `1` byte
- Returns the result of this operation on the next `2` values

`12: EQ` `13: LS` `14: MR`
- Size `1` byte
- Returns the result of this operation on the next `2` values

`15: IF`
- Size `1` byte
- Evaluates the next value and does a conditional jump based on its truthiness
- Must have a corresponding `ELIF` or `ELSE` or `END`

`16: ELIF`
- Size `1` byte
- Evaluates the next value and does a conditional jump based on its truthiness
- Must have a corresponding `ELIF` or `ELSE` or `END`

`17: ELSE`
- Size `1` byte
- Must have a corresponding `END`
- Fall through if the last `IF` or `ELIF` was falsy
- Jump to the corresponding `END` if the last `IF` or `ELIF` was truthy


`18: END`
- Size `1` byte
- Indicates the end of an `IF` or `ELIF` or `ELSE` or `FN`; anything that could be jumped over

`19: DECL`
- Size `1+1+len(name)` bytes
- The `1` extra byte is `u8` that tells the length of the variable name
- Declares a variable with the provided name and sets it to the next value

`19: LOAD`
- Size `1+1+len(name)` bytes
- The `1` extra byte is `u8` that tells the length of the variable name
- Returns the value of the variable

`21: STORE`
- Size `1+1+len(name)` bytes
- The `1` extra byte is `u8` that tells the length of the variable name
- Sets the variable to the next value

`22: FN`
- Imagine a `func(name, age) {}` then our `n = ['name', 'age']`
- Size `1+1+len(n)+len(join(n)) = 11` bytes
- The `1` extra byte is a `u8` that tells the number of argument names to expect

`23: CALL`
- Size `1+1` bytes
- The `1` extra byte is `u8` that tells the number of arguments to give to the next function

`24: RET`
- Size `1` byte
- Returns from the current function with the next value
