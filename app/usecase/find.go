package application

import (
	"time"

	"github.com/google/uuid"
	domain "meeting-booking/app/entities"
)

type SlotWithAvailability struct {
	Slot      domain.Slot `json:"slot"`
	Available bool        `json:"available"`
}

type BookingWithSlot struct {
	Booking domain.Booking `json:"booking"`
	Slot    domain.Slot    `json:"slot"`
}

func (s *Service) ListRooms() ([]domain.Room, error) {
	return s.rooms.List()
}

func (s *Service) GetAvailableSlots(role domain.Role, roomID uuid.UUID, date time.Time) ([]SlotWithAvailability, error) {
	if role != domain.RoleAdmin && role != domain.RoleUser {
		return nil, domain.ErrForbidden
	}
	if _, ok, err := s.rooms.Get(roomID); err != nil {
		return nil, err
	} else if !ok {
		return nil, domain.ErrRoomNotFound
	}
	schedule, hasSchedule, err := s.schedules.GetByRoomID(roomID)
	if err != nil {
		return nil, err
	}
	if !hasSchedule {
		return []SlotWithAvailability{}, nil
	}

	date = time.Date(date.UTC().Year(), date.UTC().Month(), date.UTC().Day(), 0, 0, 0, 0, time.UTC)
	hasSlots, err := s.slots.HasSlotsForRoomAndDate(roomID, date)
	if err != nil {
		return nil, err
	}
	if !hasSlots {
		generated := domain.GenerateSlotsForDate(roomID, schedule, date)
		if len(generated) > 0 {
			if err := s.slots.SaveBatch(generated); err != nil {
				return nil, err
			}
		}
	}

	slots, err := s.slots.FindByRoomAndDate(roomID, date)
	if err != nil {
		return nil, err
	}
	result := make([]SlotWithAvailability, 0, len(slots))
	for _, slot := range slots {
		_, taken, err := s.bookings.GetActiveBySlotID(slot.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, SlotWithAvailability{Slot: slot, Available: !taken})
	}
	return result, nil
}

func (s *Service) ListMyBookings(role domain.Role, userID uuid.UUID) ([]BookingWithSlot, error) {
	if role != domain.RoleUser {
		return nil, domain.ErrForbidden
	}
	bookings, err := s.bookings.ListByUserIDFuture(userID, s.nowUTC())
	if err != nil {
		return nil, err
	}
	now := s.nowUTC()
	result := make([]BookingWithSlot, 0, len(bookings))
	for _, b := range bookings {
		slot, ok, err := s.slots.FindByID(b.SlotID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if slot.EndAt.After(now) {
			result = append(result, BookingWithSlot{Booking: b, Slot: slot})
		}
	}
	return result, nil
}

func (s *Service) ListAllBookings(role domain.Role, page, pageSize int) ([]BookingWithSlot, int, error) {
	if role != domain.RoleAdmin {
		return nil, 0, domain.ErrForbidden
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	bookings, total, err := s.bookings.ListAll(offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]BookingWithSlot, 0, len(bookings))
	for _, b := range bookings {
		slot, ok, err := s.slots.FindByID(b.SlotID)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			continue
		}
		result = append(result, BookingWithSlot{Booking: b, Slot: slot})
	}
	return result, total, nil
}

func validatePagination(page, pageSize int) error {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return domain.ErrInvalidPagination
	}
	return nil
}
