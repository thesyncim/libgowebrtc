package media

import "testing"

func TestCameraFacingIsValid(t *testing.T) {
	tests := []struct {
		facing CameraFacing
		want   bool
	}{
		{CameraFacingUser, true},
		{CameraFacingEnvironment, true},
		{CameraFacingLeft, true},
		{CameraFacingRight, true},
		{"", true},
		{"sideways", false},
	}

	for _, tt := range tests {
		if got := tt.facing.IsValid(); got != tt.want {
			t.Fatalf("CameraFacing(%q).IsValid() = %v, want %v", tt.facing, got, tt.want)
		}
	}
}

func TestDisplayKindIsValid(t *testing.T) {
	tests := []struct {
		kind DisplayKind
		want bool
	}{
		{DisplayKindMonitor, true},
		{DisplayKindWindow, true},
		{"", true},
		{"projector", false},
	}

	for _, tt := range tests {
		if got := tt.kind.IsValid(); got != tt.want {
			t.Fatalf("DisplayKind(%q).IsValid() = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestConfigErrorError(t *testing.T) {
	err := (&ConfigError{
		Field:   "frameRate",
		Message: "must be greater than zero",
	}).Error()
	if want := "invalid capture config: frameRate - must be greater than zero"; err != want {
		t.Fatalf("Error() = %q, want %q", err, want)
	}
}
