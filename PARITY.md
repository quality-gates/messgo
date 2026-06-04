# Feature parity with phpmd

messgo is a port of **phpmd 2.15.0**. Because phpmd analyzes PHP and messgo
analyzes Go, "parity" means:

1. **Same rule catalog** — every phpmd rule is either ported, adapted to the
   nearest Go idiom, or (when intrinsically PHP-only) documented as omitted.
2. **Same metric algorithms** — cyclomatic and NPath complexity reproduce
   pdepend's numbers exactly (verified against real phpmd output, below).
3. **Same message templates** — taken byte-for-byte from phpmd's ruleset XML,
   with only deliberate Go adaptations (no `$` sigil, `::` → `.`).
4. **Same CLI and reports** — argument layout, ruleset XML format, the nine
   renderers, and exit codes 0/1/2 all match.

## Manual phpmd comparison evidence

These were produced by running the real `phpmd` binary (2.15.0, installed via
Composer) on PHP and the equivalent Go through messgo.

### Code Size — metrics match exactly

PHP (`phpmd cmp.php text codesize`) and the line-for-line Go equivalent
(`messgo cmp.go text codesize`):

| Metric | phpmd | messgo |
| --- | --- | --- |
| CyclomaticComplexity (`highComplexity`) | **12** | **12** |
| NPathComplexity (`highComplexity`) | **324** | **324** |
| ExcessiveParameterList (`manyParams`) | **11** | **11** |

phpmd:
```
cmp.php:2   CyclomaticComplexity    The function highComplexity() has a Cyclomatic Complexity of 12. The configured cyclomatic complexity threshold is 10.
cmp.php:2   NPathComplexity         The function highComplexity() has an NPath complexity of 324. The configured NPath complexity threshold is 200.
cmp.php:18  ExcessiveParameterList  The function manyParams has 11 parameters. Consider reducing the number of parameters to less than 10.
```
messgo:
```
cmp.go:3   CyclomaticComplexity    The function highComplexity() has a Cyclomatic Complexity of 12. The configured cyclomatic complexity threshold is 10.
cmp.go:3   NPathComplexity         The function highComplexity() has an NPath complexity of 324. The configured NPath complexity threshold is 200.
cmp.go:38  ExcessiveParameterList  The function manyParams has 11 parameters. Consider reducing the number of parameters to less than 10.
```

The NPath algorithm follows pdepend's `NPathComplexityAnalyzer` precisely
(if/else, for, switch-without-default-as-just-another-label, return-as-boolean-
op-count). These numbers are pinned in `internal/metrics/metrics_test.go`.

### Naming — message templates match

phpmd (`phpmd cmp.php text naming`):
```
cmp.php:2  ShortClassName   Avoid classes with short names like Fo. Configured minimum length is 3.
cmp.php:3  ShortVariable    Avoid variables with short names like $q. Configured minimum length is 3.
cmp.php:4  ShortMethodName  Avoid using short method names like Fo::a(). The configured minimum method name length is 3.
```
messgo (equivalent Go):
```
cmp.go:5  ShortClassName   Avoid classes with short names like Fo. Configured minimum length is 3.
cmp.go:6  ShortVariable    Avoid variables with short names like x. Configured minimum length is 3.
cmp.go:11 ShortMethodName  Avoid using short method names like Fo.a(). The configured minimum method name length is 3.
```
The templates are identical; differences are the documented Go adaptations
(`$q` → `x` because Go variables have no sigil; `Fo::a()` → `Fo.a()`).

The column-aligned text format, priorities, and the XML/JSON field structure
are likewise copied from phpmd's renderers.

## Rule-by-rule mapping

Legend: **Ported** = same behavior; **Adapted** = mapped to nearest Go idiom
(noted); **Omitted** = no meaningful Go analog (noted).

### Code Size (`codesize`)
| phpmd rule | Status | Go mapping |
| --- | --- | --- |
| CyclomaticComplexity | Ported | decision points + boolean ops, base 1 |
| NPathComplexity | Ported | pdepend NPath over Go AST |
| ExcessiveMethodLength | Ported | function LOC (incl. `ignore-whitespace`) |
| ExcessiveClassLength | Adapted | type decl LOC + sum of its methods' LOC |
| ExcessiveParameterList | Ported | parameter count |
| ExcessivePublicCount | Adapted | exported methods + exported fields (`cis`) |
| TooManyFields | Ported | struct field count |
| TooManyMethods | Ported | method count, `ignorepattern` honored |
| TooManyPublicMethods | Adapted | exported methods, `ignorepattern` honored |
| ExcessiveClassComplexity | Ported | WMC = Σ method CCN |

### Naming (`naming`)
| phpmd rule | Status | Go mapping |
| --- | --- | --- |
| ShortClassName / LongClassName | Ported | type & interface names |
| ShortVariable / LongVariable | Ported | fields, params, locals (loop counters exempt) |
| ShortMethodName | Ported | methods & functions |
| BooleanGetMethodName | Adapted | `get*` method returning `bool` |
| ConstantNamingConventions | Adapted | flags constant names with underscores (Go wants MixedCaps, not PHP UPPERCASE) |
| ConstructorWithNameAsEnclosingClass | Adapted | method whose name equals its receiver type |

### Unused Code (`unusedcode`)
| phpmd rule | Status | Go mapping |
| --- | --- | --- |
| UnusedPrivateField | Adapted | unexported field never selected in file |
| UnusedPrivateMethod | Adapted | unexported method never selected in file |
| UnusedFormalParameter | Ported | parameter never read in body |
| UnusedLocalVariable | Ported | local written but never read |

### Clean Code (`cleancode`)
| phpmd rule | Status | Go mapping |
| --- | --- | --- |
| BooleanArgumentFlag | Ported | `bool`-typed parameter |
| ElseExpression | Ported | `else` block (not `else if`) |
| IfStatementAssignment | Adapted | `=` assignment in `if` initializer (not `:=`) |
| DuplicatedArrayKey | Ported | duplicate constant keys in a composite literal |
| StaticAccess | Omitted | Go has no PHP-style static access |
| ErrorControlOperator | Omitted | Go has no `@` operator |
| MissingImport | Omitted | Go requires imports; compiler-enforced |
| UndefinedVariable | Omitted | compiler-enforced in Go |

### Design (`design`)
| phpmd rule | Status | Go mapping |
| --- | --- | --- |
| ExitExpression | Adapted | `os.Exit` / `syscall.Exit` call |
| GotoStatement | Ported | `goto` statement |
| CountInLoopExpression | Adapted | `len`/`cap` in a `for` condition |
| DevelopmentCodeFragment | Adapted | calls to debug funcs (`println`,`print` by default) |
| EmptyCatchBlock | Adapted | empty `if err != nil {}` (swallowed error) |
| CouplingBetweenObjects | Adapted | distinct non-builtin types in fields + method signatures |
| EvalExpression | Omitted | Go has no `eval` |
| NumberOfChildren | Omitted | Go has no class inheritance |
| DepthOfInheritance | Omitted | Go has no class inheritance |

### Controversial (`controversial`)
| phpmd rule | Status | Go mapping |
| --- | --- | --- |
| CamelCaseClassName | Adapted | type name is MixedCaps (no underscores) |
| CamelCaseMethodName | Adapted | method name has no underscores |
| CamelCasePropertyName | Adapted | field name has no underscores |
| CamelCaseParameterName | Adapted | parameter name has no underscores |
| CamelCaseVariableName | Adapted | local variable name has no underscores |
| Superglobals | Omitted | Go has no superglobals |
