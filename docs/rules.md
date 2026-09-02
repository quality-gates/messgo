# Rules and rulesets

Pass rulesets by name (comma-separated), or pass a path to your own
phpmd-format ruleset XML file.

| Ruleset | What it checks |
| :--- | :--- |
| **`go`** | **Recommended default.** Pulls in the component rulesets below, but tunes rules whose PHP defaults misfire on idiomatic Go: drops `ShortVariable`, `Design/ExitExpression`, `Design/CountInLoopExpression`, `Design/GlobalVariable`, `CleanCode/ElseExpression`, `CleanCode/BooleanArgumentFlag`, and `UnusedCode/UnusedFormalParameter`, and raises `LongVariable`'s maximum. |
| `codesize` | CyclomaticComplexity, NPathComplexity, ExcessiveMethodLength, ExcessiveClassLength, ExcessiveParameterList, ExcessivePublicCount, TooManyFields, TooManyMethods, TooManyPublicMethods, ExcessiveClassComplexity, NestingDepth, ExcessiveReturnCount, NakedReturn, CognitiveComplexity |
| `naming` | ShortClassName, LongClassName, ShortVariable, LongVariable, ShortMethodName, ConstantNamingConventions, BooleanGetMethodName, ConstructorWithNameAsEnclosingClass |
| `unusedcode` | UnusedPrivateField, UnusedLocalVariable, UnusedPrivateMethod, UnusedFormalParameter |
| `cleancode` | BooleanArgumentFlag, ElseExpression, IfStatementAssignment, DuplicatedArrayKey |
| `design` | ExitExpression, GotoStatement, CountInLoopExpression, DevelopmentCodeFragment, EmptyCatchBlock, CouplingBetweenObjects, GlobalVariable, LackOfCohesionOfMethods |
| `controversial` | CamelCaseClassName, CamelCaseMethodName, CamelCasePropertyName, CamelCaseParameterName, CamelCaseVariableName |
| `opinionated` | **Opt-in, not part of idiomatic Go.** Bundles the rules the `go` ruleset deliberately drops because they fight Go conventions: `ElseExpression`, `BooleanArgumentFlag`, `UnusedFormalParameter`, and `GlobalVariable`. Also includes Go-specific opinionated rules: `UncheckedTypeAssertion`, `IdenticalBranches`, `StructEmbeddingDepth`. Run them if you want a stricter, more PHP-flavoured style. |

Rules with a direct Go analog reproduce phpmd's behavior and message templates;
rules that are intrinsically PHP-specific are adapted to the nearest Go idiom
or omitted (the Go compiler already enforces several of them).

The `opinionated` ruleset holds checks from phpmd's PHP/OO heritage that are
*not* idiomatic Go. They stay available — `messgo ./... text opinionated`, or
`go,opinionated` to combine — but the default `go` ruleset leaves them off so a
clean run reflects idiomatic Go.

## Notable rule behavior

`GlobalVariable` is **mutation-aware**: by default it reports only package-level
variables that are actually mutated somewhere in the package (reassigned,
incremented, written through, deleted/cleared, or address-taken), analysed across all files of
the package. Effectively-constant globals — sentinel errors, compiled regexps,
lookup tables — stay silent. Set `report-immutable=true` to also surface
read-only globals.

`LackOfCohesionOfMethods` computes the **LCOM4** cohesion metric per struct
type: methods are linked when they use a common field or call one another
through the receiver, and the metric is the number of disconnected method
groups. A value above the `maximum` property (default 1) means the type bundles
unrelated responsibilities. Methods that touch no state (pure helpers, interface
stubs) and trivial getters/setters are ignored. A call to a getter counts as a
use of the field it wraps.

`TooManyMethods` also fires on **interfaces** (interface pollution), using a
separate lower threshold (`maxifacemethods`, default 10). `TooManyPublicMethods`
and `ExcessivePublicCount` intentionally do not fire on interfaces — on an
interface every method is part of the contract, so all three rules would report
the same smell three times.

`NestingDepth` flags functions whose deepest control-flow nesting (if/for/range/
switch/type-switch/select) exceeds `maxdepth` (default 5). This is the "arrow
code" smell that cyclomatic and NPath complexity do not capture. An else-if chain
does not add depth; an else block does.

`ExcessiveReturnCount` flags functions returning more values than `maxresults`
(default 3). Go allows multiple returns, but a function returning many values
begs a struct result.

`NakedReturn` flags functions with named results that use a bare `return` when
the function is long or complex enough for the implicit return to hurt
readability. Fires only when a bare return is present AND (LOC ≥ `minloc` OR CCN
≥ `minccn`). Defaults: minloc 50, minccn 10. Void functions are unaffected.

`CognitiveComplexity` flags functions whose cognitive complexity (SonarSource
algorithm) exceeds `reportLevel` (default 20). Unlike cyclomatic complexity,
cognitive complexity penalises nesting — a nested control-flow structure scores
nesting+1, not just 1. An else-if chain does not add nesting; an else block
adds +1. Boolean &&/|| sequences add 1 per operator change. Direct recursion
and labeled break/continue each add +1.

`UncheckedTypeAssertion` (opinionated) flags type assertions used without the
comma-ok form (`v, ok := x.(T)`), which panic on failure. Type switches are
safe and not flagged.

`IdenticalBranches` (opinionated) flags if/else and switch cases whose bodies
are textually identical, indicating copy-pasted logic.

`StructEmbeddingDepth` (opinionated) flags structs whose transitive embedding
chain exceeds `maxdepth` (default 3). Cross-package embeddings are treated as
depth-0 leaves (the AST alone cannot resolve them); within-package chains are
followed across files.

## Custom rulesets

Ruleset XML supports phpmd's `<rule ref="...">` form, `<exclude name="..."/>`
children, and single-rule property/priority overrides. Compose a tuned ruleset
the same way phpmd does, then pass its path as the ruleset argument.

```xml
<ruleset name="team policy">
  <rule ref="go">
    <exclude name="DevelopmentCodeFragment" />
  </rule>
  <rule ref="LongVariable">
    <priority>2</priority>
    <properties>
      <property name="maximum" value="50" />
    </properties>
  </rule>
</ruleset>
```

```console
messgo ./... text path/to/team-policy.xml --ignore-tests
```
