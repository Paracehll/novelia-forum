package domain

import "time"

type BlacklistEntry struct {
	UserID        int64
	BlockedUserID int64
	CreatedAt     time.Time
}
