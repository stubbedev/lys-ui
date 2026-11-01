package app

// OpenDialogs returns the dialogs a test cannot reach through the API
// that opened them, oldest first.
func (a *Application) OpenDialogs() []*Dialog { return a.dialogs }
