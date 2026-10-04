package usecase

// Actor contains the caller identity needed to evaluate application permissions.
// The transport layer is responsible for authenticating this identity.
type Actor struct {
	UserID   int64
	Username string
	IsAdmin  bool
}
