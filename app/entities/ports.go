package domain

import (
	"time"

	"github.com/google/uuid"
)

type RoomRepository interface {
	Create(room Room) error
	List() ([]Room, error)
	Get(id uuid.UUID) (Room, bool, error)
}

type ScheduleRepository interface {
	Create(schedule Schedule) error
	GetByRoomID(roomID uuid.UUID) (Schedule, bool, error)
}

type SlotRepository interface {
	SaveBatch(slots []Slot) error
	FindByRoomAndDate(roomID uuid.UUID, date time.Time) ([]Slot, error)
	FindByID(slotID uuid.UUID) (Slot, bool, error)
	HasSlotsForRoomAndDate(roomID uuid.UUID, date time.Time) (bool, error)
}

type BookingRepository interface {
	Create(booking Booking) error
	GetByID(bookingID uuid.UUID) (Booking, bool, error)
	GetActiveBySlotID(slotID uuid.UUID) (Booking, bool, error)
	Save(booking Booking) error
	ListByUserIDFuture(userID uuid.UUID, now time.Time) ([]Booking, error)
	ListAll(offset, limit int) ([]Booking, int, error)
}

type UserRepository interface {
	Create(user User) error
	GetByID(userID uuid.UUID) (User, bool, error)
	GetByEmail(email string) (User, bool, error)
}
