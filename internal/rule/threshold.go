package rule

import (
	"fmt"
	"strconv"

	"github.com/quality-gates/messgo/internal/model"
)

// Boundary decides whether a measured value violates a configured threshold.
// PHPMD's threshold rules pick one of two boundary conventions; centralising
// them here keeps the off-by-one decision in exactly one place instead of
// re-deriving "<" vs "<=" per rule.
type Boundary int

const (
	// AtOrAbove violates when value >= threshold. Metric rules such as
	// cyclomatic complexity, NPath, and length use this form (PHPMD's
	// "value < threshold" skip).
	AtOrAbove Boundary = iota
	// Above violates when value > threshold. Counting rules such as the
	// TooMany* family use this form (PHPMD's "value <= threshold" skip).
	Above
)

// Violates reports whether value breaches threshold under this boundary.
func (b Boundary) Violates(value, threshold int) bool {
	if b == Above {
		return value > threshold
	}
	return value >= threshold
}

// ThresholdNodeKind selects which artifact stream a threshold rule evaluates.
type ThresholdNodeKind int

const (
	ThresholdFunction ThresholdNodeKind = iota
	ThresholdClass
)

// ThresholdMeasurement is the observable metric result for one artifact.
// Args are the rule-specific leading message arguments; ThresholdRule appends
// the measured value and configured threshold in one place.
type ThresholdMeasurement struct {
	Value int
	Args  []any
}

// FuncThresholdMetric measures a function-like artifact.
type FuncThresholdMetric func(*Context, *model.Function) (ThresholdMeasurement, bool)

// ClassThresholdMetric measures a class-like artifact.
type ClassThresholdMetric func(*Context, *model.Class) (ThresholdMeasurement, bool)

// InterfaceThresholdMetric measures an interface artifact.
type InterfaceThresholdMetric func(*Context, *model.Interface) (ThresholdMeasurement, bool)

// ThresholdDeclaration describes the stable configuration for a threshold rule.
type ThresholdDeclaration struct {
	Property    string
	Default     int
	Boundary    Boundary
	NodeKind    ThresholdNodeKind
	FuncMetric  FuncThresholdMetric
	ClassMetric ClassThresholdMetric
	// InterfaceMetric, when non-nil, makes the threshold rule also evaluate
	// interfaces. This is independent of NodeKind: a class rule (NodeKind ==
	// ThresholdClass) may declare an InterfaceMetric to extend the same smell
	// to interfaces without a separate rule. Rules that omit InterfaceMetric
	// short-circuit on interfaces (ApplyInterface is a no-op).
	InterfaceMetric InterfaceThresholdMetric
	// InterfaceProperty and InterfaceDefault configure a separate threshold
	// for the interface metric, since interface thresholds are typically
	// lower than class thresholds. When InterfaceMetric is nil these are
	// ignored. When InterfaceProperty is empty, InterfaceDefault is used.
	InterfaceProperty string
	InterfaceDefault  int
}

// ThresholdRule owns the common read-compare-report skeleton for threshold
// rules. It is configured once by the ruleset loader, then reused during walks.
type ThresholdRule struct {
	decl            ThresholdDeclaration
	threshold       int
	interfaceThresh int
}

// NewThresholdRule creates a threshold rule from its declaration.
func NewThresholdRule(decl ThresholdDeclaration) *ThresholdRule {
	return &ThresholdRule{decl: decl, threshold: decl.Default, interfaceThresh: decl.InterfaceDefault}
}

// Configure parses and stores typed threshold configuration once at load time.
func (r *ThresholdRule) Configure(props Properties) error {
	threshold, err := intProperty(props, r.decl.Property, r.decl.Default)
	if err != nil {
		return err
	}
	r.threshold = threshold
	if r.decl.InterfaceMetric != nil {
		propKey := r.decl.InterfaceProperty
		if propKey == "" {
			propKey = r.decl.Property
		}
		ithreshold, err := intProperty(props, propKey, r.decl.InterfaceDefault)
		if err != nil {
			return err
		}
		r.interfaceThresh = ithreshold
	}
	return nil
}

// ApplyFunc evaluates configured function metrics.
func (r *ThresholdRule) ApplyFunc(c *Context, fn *model.Function) {
	if r.decl.NodeKind != ThresholdFunction || r.decl.FuncMetric == nil {
		return
	}
	measurement, ok := r.decl.FuncMetric(c, fn)
	if !ok {
		return
	}
	r.reportFunc(c, fn, measurement)
}

// ApplyClass evaluates configured class metrics.
func (r *ThresholdRule) ApplyClass(c *Context, class *model.Class) {
	if r.decl.NodeKind != ThresholdClass || r.decl.ClassMetric == nil {
		return
	}
	measurement, ok := r.decl.ClassMetric(c, class)
	if !ok {
		return
	}
	r.reportClass(c, class, measurement)
}

// ApplyInterface evaluates the configured interface metric, if any. Rules
// without an InterfaceMetric short-circuit (no-op), so they never fire on
// interfaces even though the method is promoted to them via embedding.
func (r *ThresholdRule) ApplyInterface(c *Context, iface *model.Interface) {
	if r.decl.InterfaceMetric == nil {
		return
	}
	measurement, ok := r.decl.InterfaceMetric(c, iface)
	if !ok {
		return
	}
	r.reportInterface(c, iface, measurement)
}

func (r *ThresholdRule) reportFunc(c *Context, fn *model.Function, measurement ThresholdMeasurement) {
	if !r.decl.Boundary.Violates(measurement.Value, r.threshold) {
		return
	}
	c.ReportFunc(fn, appendThresholdArgs(measurement, r.threshold)...)
}

func (r *ThresholdRule) reportClass(c *Context, class *model.Class, measurement ThresholdMeasurement) {
	if !r.decl.Boundary.Violates(measurement.Value, r.threshold) {
		return
	}
	c.ReportClass(class, appendThresholdArgs(measurement, r.threshold)...)
}

func (r *ThresholdRule) reportInterface(c *Context, iface *model.Interface, measurement ThresholdMeasurement) {
	if !r.decl.Boundary.Violates(measurement.Value, r.interfaceThresh) {
		return
	}
	c.ReportInterface(iface, appendThresholdArgs(measurement, r.interfaceThresh)...)
}

func appendThresholdArgs(measurement ThresholdMeasurement, threshold int) []any {
	args := make([]any, 0, len(measurement.Args)+2)
	args = append(args, measurement.Args...)
	args = append(args, measurement.Value, threshold)
	return args
}

func intProperty(props Properties, key string, def int) (int, error) {
	raw, ok := props[key]
	if !ok || raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid integer property %q=%q: %w", key, raw, err)
	}
	return n, nil
}
