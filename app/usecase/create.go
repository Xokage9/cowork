package application

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	domain "meeting-booking/app/entities"
)

type CreateRoomInput struct {
	Role        domain.Role
	Name        string
	Description string
	Capacity    int
}

func (s *Service) CreateRoom(input CreateRoomInput) (domain.Room, error) {
	if input.Role != domain.RoleAdmin {
		return domain.Room{}, domain.ErrForbidden
	}
	room := domain.Room{
		ID:          uuid.New(),
		Name:        input.Name,
		Description: input.Description,
		Capacity:    input.Capacity,
		CreatedAt:   s.nowUTC(),
	}
	if err := s.rooms.Create(room); err != nil {
		return domain.Room{}, err
	}
	return room, nil
}

type CreateScheduleInput struct {
	Role         domain.Role
	RoomID       uuid.UUID
	Weekdays     []time.Weekday
	StartMinutes int
	EndMinutes   int
}

func (s *Service) CreateSchedule(input CreateScheduleInput) (domain.Schedule, error) {
	if input.Role != domain.RoleAdmin {
		return domain.Schedule{}, domain.ErrForbidden
	}
	if err := domain.ValidateSchedule(input.Weekdays, input.StartMinutes, input.EndMinutes); err != nil {
		return domain.Schedule{}, err
	}
	if _, ok, err := s.rooms.Get(input.RoomID); err != nil {
		return domain.Schedule{}, err
	} else if !ok {
		return domain.Schedule{}, domain.ErrRoomNotFound
	}
	if _, exists, err := s.schedules.GetByRoomID(input.RoomID); err != nil {
		return domain.Schedule{}, err
	} else if exists {
		return domain.Schedule{}, domain.ErrScheduleAlreadyExists
	}

	schedule := domain.Schedule{
		RoomID:       input.RoomID,
		Weekdays:     input.Weekdays,
		StartMinutes: input.StartMinutes,
		EndMinutes:   input.EndMinutes,
		CreatedAt:    s.nowUTC(),
	}
	if err := s.schedules.Create(schedule); err != nil {
		return domain.Schedule{}, err
	}

	startDay := time.Date(schedule.CreatedAt.Year(), schedule.CreatedAt.Month(), schedule.CreatedAt.Day(), 0, 0, 0, 0, time.UTC)
	batch := make([]domain.Slot, 0)
	for i := 0; i < s.horizonDays; i++ {
		date := startDay.AddDate(0, 0, i)
		batch = append(batch, domain.GenerateSlotsForDate(input.RoomID, schedule, date)...)
	}
	if len(batch) > 0 {
		if err := s.slots.SaveBatch(batch); err != nil {
			return domain.Schedule{}, err
		}
	}

	return schedule, nil
}

type CreateBookingInput struct {
	Role                 domain.Role
	UserID               uuid.UUID
	SlotID               uuid.UUID
	CreateConferenceLink bool
}

func (s *Service) CreateBooking(input CreateBookingInput) (domain.Booking, error) {
	if input.Role != domain.RoleUser {
		return domain.Booking{}, domain.ErrForbidden
	}
	slot, ok, err := s.slots.FindByID(input.SlotID)
	if err != nil {
		return domain.Booking{}, err
	}
	if !ok {
		return domain.Booking{}, domain.ErrSlotNotFound
	}
	if !slot.StartAt.After(s.nowUTC()) {
		return domain.Booking{}, domain.ErrCannotBookPastSlot
	}
	if _, exists, err := s.bookings.GetActiveBySlotID(slot.ID); err != nil {
		return domain.Booking{}, err
	} else if exists {
		return domain.Booking{}, domain.ErrSlotAlreadyBooked
	}

	booking := domain.Booking{
		ID:               uuid.New(),
		SlotID:           slot.ID,
		RoomID:           slot.RoomID,
		UserID:           input.UserID,
		Status:           domain.BookingActive,
		ConferenceStatus: "not_requested",
		CreatedAt:        s.nowUTC(),
	}
	if err := s.bookings.Create(booking); err != nil {
		return domain.Booking{}, err
	}

	if !input.CreateConferenceLink {
		return booking, nil
	}

	booking = s.resolveConferenceLink(booking, slot)
	if err := s.bookings.Save(booking); err != nil {
		return domain.Booking{}, err
	}

	return booking, nil
}

func (s *Service) resolveConferenceLink(booking domain.Booking, slot domain.Slot) domain.Booking {
	booking.ConferenceStatus = "pending"
	if s.conference == nil {
		booking.ConferenceStatus = "failed"
		return booking
	}

	link, err := s.conference.CreateLink(context.Background(), booking.ID, booking.RoomID, booking.UserID, slot.StartAt, slot.EndAt)
	if err != nil {
		booking.ConferenceStatus = "failed"
		return booking
	}

	booking.ConferenceStatus = "created"
	booking.ConferenceLink = &link
	return booking
}

type RegisterInput struct {
	Email    string
	Password string
	Role     domain.Role
}

func (s *Service) Register(input RegisterInput) (domain.User, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" || strings.TrimSpace(input.Password) == "" {
		return domain.User{}, domain.ErrInvalidCredentials
	}

	role := input.Role
	if role == "" {
		role = domain.RoleUser
	}
	if role != domain.RoleUser && role != domain.RoleAdmin {
		return domain.User{}, domain.ErrForbidden
	}

	if _, exists, err := s.users.GetByEmail(email); err != nil {
		return domain.User{}, err
	} else if exists {
		return domain.User{}, domain.ErrEmailAlreadyExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	user := domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
		Registered:   s.nowUTC(),
	}
	if err := s.users.Create(user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

type LoginInput struct {
	Email    string
	Password string
}

func (s *Service) Login(input LoginInput) (domain.User, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	user, exists, err := s.users.GetByEmail(email)
	if err != nil {
		return domain.User{}, err
	}
	if !exists {
		return domain.User{}, domain.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return domain.User{}, domain.ErrInvalidCredentials
	}
	return user, nil
}
