package terminaltext

// safeError is an error whose text is safe to draw.
type safeError struct{ err error }

func (safeError safeError) Error() string { return Clean(safeError.err.Error()) }
func (safeError safeError) Unwrap() error { return safeError.err }

// SafeError wraps err so that Error returns its text passed through Clean.
// API and launcher errors can carry attacker-controlled text, and the shell
// draws error messages as given, so views wrap them before sending userinterface.Error.
// errors.Is and errors.As still see the original error through Unwrap.
func SafeError(err error) error { return safeError{err} }
