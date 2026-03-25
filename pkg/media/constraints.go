package media

import "fmt"

// IntConstraint supports browser-like exact/ideal/min/max constraint patterns.
// Use the constructor functions Exact(), Ideal(), Min(), Max() for convenience.
type IntConstraint struct {
	Exact *int // Exact requires one exact value.
	Ideal *int // Ideal expresses a preferred value.
	Min   *int // Min sets the inclusive lower bound.
	Max   *int // Max sets the inclusive upper bound.
}

// IsSet returns true if any constraint value is present.
func (c IntConstraint) IsSet() bool {
	return c.Exact != nil || c.Ideal != nil || c.Min != nil || c.Max != nil
}

// Value returns the effective value, preferring exact > ideal > min.
// Returns (0, false) if no value is set.
func (c IntConstraint) Value() (int, bool) {
	if c.Exact != nil {
		return *c.Exact, true
	}
	if c.Ideal != nil {
		return *c.Ideal, true
	}
	if c.Min != nil {
		return *c.Min, true
	}
	return 0, false
}

// IsExact returns true if this constraint requires an exact match.
func (c IntConstraint) IsExact() bool {
	return c.Exact != nil
}

// Validate checks if a value satisfies the constraint.
func (c IntConstraint) Validate(value int) error {
	if c.Exact != nil && value != *c.Exact {
		return &OverconstrainedError{
			Constraint: "value",
			Message:    fmt.Sprintf("requires exact %d, got %d", *c.Exact, value),
		}
	}
	if c.Min != nil && value < *c.Min {
		return &OverconstrainedError{
			Constraint: "value",
			Message:    fmt.Sprintf("minimum is %d, got %d", *c.Min, value),
		}
	}
	if c.Max != nil && value > *c.Max {
		return &OverconstrainedError{
			Constraint: "value",
			Message:    fmt.Sprintf("maximum is %d, got %d", *c.Max, value),
		}
	}
	return nil
}

// FloatConstraint supports browser-like exact/ideal/min/max for floating-point values.
type FloatConstraint struct {
	Exact *float64 // Exact requires one exact value.
	Ideal *float64 // Ideal expresses a preferred value.
	Min   *float64 // Min sets the inclusive lower bound.
	Max   *float64 // Max sets the inclusive upper bound.
}

// IsSet returns true if any constraint value is present.
func (c FloatConstraint) IsSet() bool {
	return c.Exact != nil || c.Ideal != nil || c.Min != nil || c.Max != nil
}

// Value returns the effective value, preferring exact > ideal > min.
// Returns (0, false) if no value is set.
func (c FloatConstraint) Value() (float64, bool) {
	if c.Exact != nil {
		return *c.Exact, true
	}
	if c.Ideal != nil {
		return *c.Ideal, true
	}
	if c.Min != nil {
		return *c.Min, true
	}
	return 0, false
}

// IsExact returns true if this constraint requires an exact match.
func (c FloatConstraint) IsExact() bool {
	return c.Exact != nil
}

// Validate checks if a value satisfies the constraint.
func (c FloatConstraint) Validate(value float64) error {
	if c.Exact != nil && value != *c.Exact {
		return &OverconstrainedError{
			Constraint: "value",
			Message:    fmt.Sprintf("requires exact %v, got %v", *c.Exact, value),
		}
	}
	if c.Min != nil && value < *c.Min {
		return &OverconstrainedError{
			Constraint: "value",
			Message:    fmt.Sprintf("minimum is %v, got %v", *c.Min, value),
		}
	}
	if c.Max != nil && value > *c.Max {
		return &OverconstrainedError{
			Constraint: "value",
			Message:    fmt.Sprintf("maximum is %v, got %v", *c.Max, value),
		}
	}
	return nil
}

// StringConstraint supports browser-like exact/ideal matching for string values.
type StringConstraint struct {
	Exact *string // Exact requires one exact value.
	Ideal *string // Ideal expresses a preferred value.
}

// Value returns the effective value, preferring exact > ideal.
// Returns ("", false) if no value is set.
func (c StringConstraint) Value() (string, bool) {
	if c.Exact != nil {
		return *c.Exact, true
	}
	if c.Ideal != nil {
		return *c.Ideal, true
	}
	return "", false
}

// IsSet returns true if any constraint value is present.
func (c StringConstraint) IsSet() bool {
	return c.Exact != nil || c.Ideal != nil
}

// Validate checks if a value satisfies the constraint.
func (c StringConstraint) Validate(value string) error {
	if c.Exact != nil && value != *c.Exact {
		return &OverconstrainedError{
			Constraint: "value",
			Message:    fmt.Sprintf("requires exact %q, got %q", *c.Exact, value),
		}
	}
	return nil
}

// BoolConstraint supports browser-like exact/ideal matching for boolean values.
type BoolConstraint struct {
	Exact *bool // Exact requires one exact value.
	Ideal *bool // Ideal expresses a preferred value.
}

// Value returns the effective value, preferring exact > ideal.
// Returns (false, false) if no value is set.
func (c BoolConstraint) Value() (bool, bool) {
	if c.Exact != nil {
		return *c.Exact, true
	}
	if c.Ideal != nil {
		return *c.Ideal, true
	}
	return false, false
}

// IsSet returns true if any constraint value is present.
func (c BoolConstraint) IsSet() bool {
	return c.Exact != nil || c.Ideal != nil
}

// Validate checks if a value satisfies the constraint.
func (c BoolConstraint) Validate(value bool) error {
	if c.Exact != nil && value != *c.Exact {
		return &OverconstrainedError{
			Constraint: "value",
			Message:    fmt.Sprintf("requires exact %t, got %t", *c.Exact, value),
		}
	}
	return nil
}

// FacingMode indicates which camera direction to prefer.
// Matches browser's VideoFacingModeEnum.
type FacingMode string

const (
	// FacingModeUser is the front-facing camera (selfie camera).
	FacingModeUser FacingMode = "user"

	// FacingModeEnvironment is the rear-facing camera.
	FacingModeEnvironment FacingMode = "environment"

	// FacingModeLeft is a camera facing left (uncommon).
	FacingModeLeft FacingMode = "left"

	// FacingModeRight is a camera facing right (uncommon).
	FacingModeRight FacingMode = "right"
)

// IsValid returns true if this is a valid facing mode value.
func (m FacingMode) IsValid() bool {
	switch m {
	case FacingModeUser, FacingModeEnvironment, FacingModeLeft, FacingModeRight, "":
		return true
	default:
		return false
	}
}

// DisplaySurface indicates what type of display surface to capture.
// Matches browser's DisplayCaptureSurfaceType.
type DisplaySurface string

const (
	// DisplaySurfaceMonitor captures the entire screen/monitor.
	DisplaySurfaceMonitor DisplaySurface = "monitor"

	// DisplaySurfaceWindow captures a specific application window.
	DisplaySurfaceWindow DisplaySurface = "window"

	// DisplaySurfaceBrowser captures a browser tab (not applicable for native apps).
	DisplaySurfaceBrowser DisplaySurface = "browser"
)

// IsValid returns true if this is a valid display surface value.
func (s DisplaySurface) IsValid() bool {
	switch s {
	case DisplaySurfaceMonitor, DisplaySurfaceWindow, DisplaySurfaceBrowser, "":
		return true
	default:
		return false
	}
}

// OverconstrainedError is returned when constraints cannot be satisfied.
// Matches browser's OverconstrainedError interface.
type OverconstrainedError struct {
	Constraint string // Constraint is the browser-shaped constraint name that failed.
	Message    string // Message describes why the constraint could not be satisfied.
}

// Error implements the error interface.
func (e *OverconstrainedError) Error() string {
	return fmt.Sprintf("overconstrained: %s - %s", e.Constraint, e.Message)
}

// Helper functions for creating constraint values

// ExactInt creates an IntConstraint that requires an exact value.
func ExactInt(v int) IntConstraint {
	return IntConstraint{Exact: &v}
}

// IdealInt creates an IntConstraint with an ideal (preferred) value.
func IdealInt(v int) IntConstraint {
	return IntConstraint{Ideal: &v}
}

// RangeInt creates an IntConstraint with min and max bounds.
func RangeInt(minVal, maxVal int) IntConstraint {
	return IntConstraint{Min: &minVal, Max: &maxVal}
}

// ExactFloat creates a FloatConstraint that requires an exact value.
func ExactFloat(v float64) FloatConstraint {
	return FloatConstraint{Exact: &v}
}

// IdealFloat creates a FloatConstraint with an ideal (preferred) value.
func IdealFloat(v float64) FloatConstraint {
	return FloatConstraint{Ideal: &v}
}

// RangeFloat creates a FloatConstraint with min and max bounds.
func RangeFloat(minVal, maxVal float64) FloatConstraint {
	return FloatConstraint{Min: &minVal, Max: &maxVal}
}

// ExactString creates a StringConstraint that requires an exact value.
func ExactString(v string) StringConstraint {
	return StringConstraint{Exact: &v}
}

// IdealString creates a StringConstraint with an ideal (preferred) value.
func IdealString(v string) StringConstraint {
	return StringConstraint{Ideal: &v}
}

// ExactBool creates a BoolConstraint that requires an exact value.
func ExactBool(v bool) BoolConstraint {
	return BoolConstraint{Exact: &v}
}

// IdealBool creates a BoolConstraint with an ideal (preferred) value.
func IdealBool(v bool) BoolConstraint {
	return BoolConstraint{Ideal: &v}
}
