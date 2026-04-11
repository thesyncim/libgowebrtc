package media

import "testing"

func TestIntConstraintValueAndValidate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var c IntConstraint
		if got, ok := c.Value(); ok || got != 0 {
			t.Fatalf("Value() = (%d, %v), want (0, false)", got, ok)
		}
		if c.IsSet() {
			t.Fatal("IsSet() = true, want false")
		}
		if c.IsExact() {
			t.Fatal("IsExact() = true, want false")
		}
		if err := c.Validate(42); err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("exact", func(t *testing.T) {
		c := ExactInt(640)
		if got, ok := c.Value(); !ok || got != 640 {
			t.Fatalf("Value() = (%d, %v), want (640, true)", got, ok)
		}
		if !c.IsSet() {
			t.Fatal("IsSet() = false, want true")
		}
		if !c.IsExact() {
			t.Fatal("IsExact() = false, want true")
		}
		if err := c.Validate(640); err != nil {
			t.Fatalf("Validate(exact) unexpected error: %v", err)
		}
		if err := c.Validate(320); err == nil {
			t.Fatal("Validate(non-matching exact) expected error")
		}
	})

	t.Run("ideal", func(t *testing.T) {
		c := IdealInt(30)
		if got, ok := c.Value(); !ok || got != 30 {
			t.Fatalf("Value() = (%d, %v), want (30, true)", got, ok)
		}
		if c.IsExact() {
			t.Fatal("IsExact() = true, want false")
		}
		if err := c.Validate(24); err != nil {
			t.Fatalf("Validate(ideal) unexpected error: %v", err)
		}
	})

	t.Run("range", func(t *testing.T) {
		c := RangeInt(15, 60)
		if got, ok := c.Value(); !ok || got != 15 {
			t.Fatalf("Value() = (%d, %v), want (15, true)", got, ok)
		}
		if err := c.Validate(30); err != nil {
			t.Fatalf("Validate(in-range) unexpected error: %v", err)
		}
		if err := c.Validate(10); err == nil {
			t.Fatal("Validate(below-min) expected error")
		}
		if err := c.Validate(120); err == nil {
			t.Fatal("Validate(above-max) expected error")
		}
	})
}

func TestFloatConstraintValueAndValidate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var c FloatConstraint
		if got, ok := c.Value(); ok || got != 0 {
			t.Fatalf("Value() = (%v, %v), want (0, false)", got, ok)
		}
		if c.IsSet() {
			t.Fatal("IsSet() = true, want false")
		}
		if c.IsExact() {
			t.Fatal("IsExact() = true, want false")
		}
		if err := c.Validate(1.5); err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("exact", func(t *testing.T) {
		c := ExactFloat(29.97)
		if got, ok := c.Value(); !ok || got != 29.97 {
			t.Fatalf("Value() = (%v, %v), want (29.97, true)", got, ok)
		}
		if !c.IsSet() {
			t.Fatal("IsSet() = false, want true")
		}
		if !c.IsExact() {
			t.Fatal("IsExact() = false, want true")
		}
		if err := c.Validate(29.97); err != nil {
			t.Fatalf("Validate(exact) unexpected error: %v", err)
		}
		if err := c.Validate(30); err == nil {
			t.Fatal("Validate(non-matching exact) expected error")
		}
	})

	t.Run("ideal", func(t *testing.T) {
		c := IdealFloat(1.5)
		if got, ok := c.Value(); !ok || got != 1.5 {
			t.Fatalf("Value() = (%v, %v), want (1.5, true)", got, ok)
		}
		if c.IsExact() {
			t.Fatal("IsExact() = true, want false")
		}
		if err := c.Validate(2.0); err != nil {
			t.Fatalf("Validate(ideal) unexpected error: %v", err)
		}
	})

	t.Run("range", func(t *testing.T) {
		c := RangeFloat(1.0, 2.5)
		if got, ok := c.Value(); !ok || got != 1.0 {
			t.Fatalf("Value() = (%v, %v), want (1.0, true)", got, ok)
		}
		if err := c.Validate(1.5); err != nil {
			t.Fatalf("Validate(in-range) unexpected error: %v", err)
		}
		if err := c.Validate(0.5); err == nil {
			t.Fatal("Validate(below-min) expected error")
		}
		if err := c.Validate(3.0); err == nil {
			t.Fatal("Validate(above-max) expected error")
		}
	})
}

func TestStringConstraintValueAndValidate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var c StringConstraint
		if got, ok := c.Value(); ok || got != "" {
			t.Fatalf("Value() = (%q, %v), want (\"\", false)", got, ok)
		}
		if c.IsSet() {
			t.Fatal("IsSet() = true, want false")
		}
		if err := c.Validate("camera-1"); err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("exact", func(t *testing.T) {
		c := ExactString("camera-1")
		if got, ok := c.Value(); !ok || got != "camera-1" {
			t.Fatalf("Value() = (%q, %v), want (%q, true)", got, ok, "camera-1")
		}
		if !c.IsSet() {
			t.Fatal("IsSet() = false, want true")
		}
		if err := c.Validate("camera-1"); err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
		if err := c.Validate("camera-2"); err == nil {
			t.Fatal("Validate(non-matching exact) expected error")
		}
	})
}

func TestBoolConstraintValueAndValidate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var c BoolConstraint
		if got, ok := c.Value(); ok || got {
			t.Fatalf("Value() = (%v, %v), want (false, false)", got, ok)
		}
		if c.IsSet() {
			t.Fatal("IsSet() = true, want false")
		}
		if err := c.Validate(true); err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("exact", func(t *testing.T) {
		c := ExactBool(true)
		if got, ok := c.Value(); !ok || !got {
			t.Fatalf("Value() = (%v, %v), want (true, true)", got, ok)
		}
		if !c.IsSet() {
			t.Fatal("IsSet() = false, want true")
		}
		if err := c.Validate(true); err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
		if err := c.Validate(false); err == nil {
			t.Fatal("Validate(non-matching exact) expected error")
		}
	})
}

func TestConstraintEnumsAndErrors(t *testing.T) {
	facingModes := []struct {
		mode FacingMode
		want bool
	}{
		{FacingModeUser, true},
		{FacingModeEnvironment, true},
		{FacingModeLeft, true},
		{FacingModeRight, true},
		{"", true},
		{"sideways", false},
	}

	for _, tt := range facingModes {
		if got := tt.mode.IsValid(); got != tt.want {
			t.Fatalf("FacingMode(%q).IsValid() = %v, want %v", tt.mode, got, tt.want)
		}
	}

	displaySurfaces := []struct {
		surface DisplaySurface
		want    bool
	}{
		{DisplaySurfaceMonitor, true},
		{DisplaySurfaceWindow, true},
		{DisplaySurfaceBrowser, true},
		{"", true},
		{"projector", false},
	}

	for _, tt := range displaySurfaces {
		if got := tt.surface.IsValid(); got != tt.want {
			t.Fatalf("DisplaySurface(%q).IsValid() = %v, want %v", tt.surface, got, tt.want)
		}
	}

	err := (&OverconstrainedError{
		Constraint: "frameRate",
		Message:    "requires exact 30, got 15",
	}).Error()
	if want := "overconstrained: frameRate - requires exact 30, got 15"; err != want {
		t.Fatalf("Error() = %q, want %q", err, want)
	}
}
