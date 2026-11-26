package platform

// SystemEvent describes the current OS session.
// Lock is an edge (including resume/disconnect) that invalidates vault sessions.
// An unavailable provider must not be interpreted as an unlocked OS session.
type SystemEvent struct {
	Available bool
	Locked    bool
	Sleeping  bool
	Lock      bool
	Reason    string
}
