package view

import (
	"time"

	"github.com/google/uuid"
	domain "meeting-booking/app/entities"
	booking "meeting-booking/app/usecase"
)

type RoomResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Capacity    int       `json:"capacity"`
	CreatedAt   time.Time `json:"createdAt"`
}

type SlotResponse struct {
	ID        uuid.UUID `json:"id"`
	StartAt   time.Time `json:"startAt"`
	EndAt     time.Time `json:"endAt"`
	Available bool      `json:"available"`
}

type BookingResponse struct {
	ID               uuid.UUID            `json:"id"`
	SlotID           uuid.UUID            `json:"slotId"`
	RoomID           uuid.UUID            `json:"roomId"`
	UserID           uuid.UUID            `json:"userId"`
	Status           domain.BookingStatus `json:"status"`
	ConferenceLink   *string              `json:"conferenceLink,omitempty"`
	ConferenceStatus string               `json:"conferenceStatus,omitempty"`
	CreatedAt        time.Time            `json:"createdAt"`
	CancelledAt      *time.Time           `json:"cancelledAt,omitempty"`
	SlotStartAt      time.Time            `json:"slotStartAt"`
	SlotEndAt        time.Time            `json:"slotEndAt"`
}

func MapRooms(rooms []domain.Room) []RoomResponse {
	resp := make([]RoomResponse, 0, len(rooms))
	for _, room := range rooms {
		resp = append(resp, RoomResponse(room))
	}
	return resp
}

func MapSlots(slots []booking.SlotWithAvailability) []SlotResponse {
	resp := make([]SlotResponse, 0, len(slots))
	for _, item := range slots {
		resp = append(resp, SlotResponse{
			ID:        item.Slot.ID,
			StartAt:   item.Slot.StartAt,
			EndAt:     item.Slot.EndAt,
			Available: item.Available,
		})
	}
	return resp
}

func MapBookings(items []booking.BookingWithSlot) []BookingResponse {
	resp := make([]BookingResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, BookingResponse{
			ID:               item.Booking.ID,
			SlotID:           item.Booking.SlotID,
			RoomID:           item.Booking.RoomID,
			UserID:           item.Booking.UserID,
			Status:           item.Booking.Status,
			ConferenceLink:   item.Booking.ConferenceLink,
			ConferenceStatus: item.Booking.ConferenceStatus,
			CreatedAt:        item.Booking.CreatedAt,
			CancelledAt:      item.Booking.CancelledAt,
			SlotStartAt:      item.Slot.StartAt,
			SlotEndAt:        item.Slot.EndAt,
		})
	}
	return resp
}
