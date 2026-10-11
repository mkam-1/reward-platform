package domain

import "time"

// Model holds the fields shared by every domain entity. Entities embed it
// anonymously so ID and CreatedAt only need to be defined once:
//
//	type Player struct {
//		Model
//		Username string
//	}
//
// player.ID and player.CreatedAt then work directly via Go's field
// promotion, without Player re-declaring them.
type Model struct {
	ID        string
	CreatedAt time.Time
}
