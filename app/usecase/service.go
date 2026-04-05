package application

import (
	domain "meeting-booking/app/entities"
	"time"
)

type Service struct {
	rooms       domain.RoomRepository
	schedules   domain.ScheduleRepository
	slots       domain.SlotRepository
	bookings    domain.BookingRepository
	users       domain.UserRepository
	conference  ConferenceProvider
	clock       func() time.Time
	horizonDays int
}

func NewService(
	rooms domain.RoomRepository,
	schedules domain.ScheduleRepository,
	slots domain.SlotRepository,
	bookings domain.BookingRepository,
	users domain.UserRepository,
	conference ConferenceProvider,
	clock func() time.Time,
	horizonDays int,
) *Service {
	if clock == nil {
		clock = time.Now
	}
	if horizonDays <= 0 {
		horizonDays = 30
	}
	return &Service{
		rooms:       rooms,
		schedules:   schedules,
		slots:       slots,
		bookings:    bookings,
		users:       users,
		conference:  conference,
		clock:       clock,
		horizonDays: horizonDays,
	}
}

func (s *Service) nowUTC() time.Time {
	return s.clock().UTC()
}
