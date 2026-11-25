package platform

// SystemEvent describes the current OS session, never credentials or content.
// Lock is an edge (including resume/disconnect) that invalidates vault sessions.
// An unavailable provider must NOT be interpreted as an unlocked OS session.
type SystemEvent struct {
	Available bool
	Locked    bool
	Sleeping  bool
	Lock      bool
	Reason    string
}
