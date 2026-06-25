package rule

import "sync"

// Base holds the metadata that PHPMD loads from a ruleset XML <rule> element.
// Concrete rule logic types embed *Base, so they satisfy the Rule interface
// for free and only implement the Apply* method(s) they care about.
type Base struct {
	RuleName    string
	RuleMessage string
	RulePrio    int
	RuleSet     string
	RuleURL     string
	RuleDesc    string
	RuleSince   string
	RuleProps   Properties
}

func (b *Base) Name() string        { return b.RuleName }
func (b *Base) Message() string     { return b.RuleMessage }
func (b *Base) Priority() int       { return b.RulePrio }
func (b *Base) SetName() string     { return b.RuleSet }
func (b *Base) ExternalURL() string { return b.RuleURL }
func (b *Base) Description() string { return b.RuleDesc }
func (b *Base) Since() string       { return b.RuleSince }

// BaseRef is implemented by every rule logic type via the embedded *Base. The
// loader uses it to inject metadata after construction.
type BaseRef interface {
	base() *Base
}

// Configurable is implemented by rules that parse typed properties once when
// the ruleset is loaded.
type Configurable interface {
	Configure(Properties) error
}

func (b *Base) base() *Base { return b }

// BaseOf returns the embedded *Base of a rule, or nil if it has none.
func BaseOf(r Rule) *Base {
	if br, ok := r.(BaseRef); ok {
		return br.base()
	}
	return nil
}

// Constructor builds a fresh rule logic instance with an initialized *Base.
type Constructor func() Rule

var (
	registryMu sync.RWMutex
	registry   = map[string]Constructor{}
)

// Register associates a PHPMD rule class name (e.g.
// "PHPMD\\Rule\\CyclomaticComplexity") with a constructor. Called from rule
// packages' init() functions.
func Register(class string, c Constructor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[class] = c
}

// Lookup returns the constructor for a class name.
func Lookup(class string) (Constructor, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	c, ok := registry[class]
	return c, ok
}

// Registered reports whether a class has a constructor.
func Registered(class string) bool {
	_, ok := Lookup(class)
	return ok
}

// NewBase is a helper for constructors.
func NewBase() *Base { return &Base{RuleProps: Properties{}} }
