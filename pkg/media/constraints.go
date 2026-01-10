package media

import "fmt"

// IntConstraint supports browser-like exact/ideal/min/max constraint patterns.
// Use the constructor functions Exact(), Ideal(), Min(), Max() for convenience.
type IntConstraint struct {
	Exact *int
	Ideal *int
	Min   *int
	Max   *int
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
	Exact *float64
	Ideal *float64
	Min   *float64
	Max   *float64
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
	Constraint string
	Message    string
}

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
