package media

import "fmt"

// CameraFacing indicates which camera direction to prefer.
type CameraFacing string

const (
	// CameraFacingUser is the front-facing camera.
	CameraFacingUser CameraFacing = "user"

	// CameraFacingEnvironment is the rear-facing camera.
	CameraFacingEnvironment CameraFacing = "environment"

	// CameraFacingLeft is a camera facing left.
	CameraFacingLeft CameraFacing = "left"

	// CameraFacingRight is a camera facing right.
	CameraFacingRight CameraFacing = "right"
)

// IsValid returns true if this is a supported camera facing value.
func (m CameraFacing) IsValid() bool {
	switch m {
	case CameraFacingUser, CameraFacingEnvironment, CameraFacingLeft, CameraFacingRight, "":
		return true
	default:
		return false
	}
}

// DisplayKind indicates what type of native display target to capture.
type DisplayKind string

const (
	// DisplayKindMonitor captures a full monitor.
	DisplayKindMonitor DisplayKind = "monitor"

	// DisplayKindWindow captures a single application window.
	DisplayKindWindow DisplayKind = "window"
)

// IsValid returns true if this is a supported display target kind.
func (k DisplayKind) IsValid() bool {
	switch k {
	case DisplayKindMonitor, DisplayKindWindow, "":
		return true
	default:
		return false
	}
}

// ConfigError reports one invalid or unsatisfied capture field.
type ConfigError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ConfigError) Error() string {
	return fmt.Sprintf("invalid capture config: %s - %s", e.Field, e.Message)
}

// Unwrap allows callers to match config validation failures with errors.Is.
func (e *ConfigError) Unwrap() error {
	return ErrInvalidCaptureConfig
}
