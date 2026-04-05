package domain

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type BookingStatus string

const (
	BookingActive    BookingStatus = "active"
	BookingCancelled BookingStatus = "cancelled"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	Registered   time.Time `json:"registeredAt"`
}

type Room struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Capacity    int       `json:"capacity"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Schedule struct {
	RoomID       uuid.UUID      `json:"roomId"`
	Weekdays     []time.Weekday `json:"weekdays"`
	StartMinutes int            `json:"startMinutes"`
	EndMinutes   int            `json:"endMinutes"`
	CreatedAt    time.Time      `json:"createdAt"`
}

type Slot struct {
	ID      uuid.UUID `json:"id"`
	RoomID  uuid.UUID `json:"roomId"`
	StartAt time.Time `json:"startAt"`
	EndAt   time.Time `json:"endAt"`
}

type Booking struct {
	ID               uuid.UUID     `json:"id"`
	SlotID           uuid.UUID     `json:"slotId"`
	RoomID           uuid.UUID     `json:"roomId"`
	UserID           uuid.UUID     `json:"userId"`
	Status           BookingStatus `json:"status"`
	ConferenceLink   *string       `json:"conferenceLink,omitempty"`
	ConferenceStatus string        `json:"conferenceStatus,omitempty"`
	CreatedAt        time.Time     `json:"createdAt"`
	CancelledAt      *time.Time    `json:"cancelledAt,omitempty"`
}
