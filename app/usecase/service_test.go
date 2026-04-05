package application_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	domain "meeting-booking/app/entities"
	"meeting-booking/app/repository"
	booking "meeting-booking/app/usecase"
)

func newServiceWithClock(now time.Time) *booking.Service {
	return booking.NewService(
		repository.NewMemoryRoomRepository(),
		repository.NewMemoryScheduleRepository(),
		repository.NewMemorySlotRepository(),
		repository.NewMemoryBookingRepository(),
		repository.NewMemoryUserRepository(),
		booking.MockConferenceProvider{},
		func() time.Time { return now },
		30,
	)
}

func TestBookingService_CreateBooking_WhenSlotInPast_ExpectError(t *testing.T) {
	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(now)

	room, err := svc.CreateRoom(booking.CreateRoomInput{Role: domain.RoleAdmin, Name: "R", Capacity: 4})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	_, err = svc.CreateSchedule(booking.CreateScheduleInput{
		Role: domain.RoleAdmin, RoomID: room.ID, Weekdays: []time.Weekday{time.Saturday}, StartMinutes: 8 * 60, EndMinutes: 10 * 60,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	slots, err := svc.GetAvailableSlots(domain.RoleUser, room.ID, now)
	if err != nil {
		t.Fatalf("get slots: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected slots")
	}

	_, err = svc.CreateBooking(booking.CreateBookingInput{Role: domain.RoleUser, UserID: uuid.New(), SlotID: slots[0].Slot.ID})
	if err != domain.ErrCannotBookPastSlot {
		t.Fatalf("expected ErrCannotBookPastSlot, got: %v", err)
	}
}

func TestBookingService_CancelBooking_WhenRepeatedCall_ExpectIdempotentResult(t *testing.T) {
	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(now)

	room, _ := svc.CreateRoom(booking.CreateRoomInput{Role: domain.RoleAdmin, Name: "R", Capacity: 4})
	_, _ = svc.CreateSchedule(booking.CreateScheduleInput{
		Role: domain.RoleAdmin, RoomID: room.ID, Weekdays: []time.Weekday{time.Sunday}, StartMinutes: 9 * 60, EndMinutes: 11 * 60,
	})
	slots, _ := svc.GetAvailableSlots(domain.RoleUser, room.ID, now.AddDate(0, 0, 1))
	created, err := svc.CreateBooking(booking.CreateBookingInput{Role: domain.RoleUser, UserID: uuid.New(), SlotID: slots[0].Slot.ID})
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	first, err := svc.CancelBooking(domain.RoleUser, created.UserID, created.ID)
	if err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	second, err := svc.CancelBooking(domain.RoleUser, created.UserID, created.ID)
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}

	if first.Status != domain.BookingCancelled || second.Status != domain.BookingCancelled {
		t.Fatal("expected cancelled status for both calls")
	}
}

func TestScheduleService_CreateSchedule_WhenAlreadyExists_ExpectConflictError(t *testing.T) {
	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(now)
	room, _ := svc.CreateRoom(booking.CreateRoomInput{Role: domain.RoleAdmin, Name: "R", Capacity: 4})

	_, err := svc.CreateSchedule(booking.CreateScheduleInput{Role: domain.RoleAdmin, RoomID: room.ID, Weekdays: []time.Weekday{time.Monday}, StartMinutes: 9 * 60, EndMinutes: 10 * 60})
	if err != nil {
		t.Fatalf("first create schedule: %v", err)
	}
	_, err = svc.CreateSchedule(booking.CreateScheduleInput{Role: domain.RoleAdmin, RoomID: room.ID, Weekdays: []time.Weekday{time.Monday}, StartMinutes: 10 * 60, EndMinutes: 11 * 60})
	if err != domain.ErrScheduleAlreadyExists {
		t.Fatalf("expected ErrScheduleAlreadyExists, got: %v", err)
	}
}

func TestBookingService_ListMyBookings_WhenPastSlotsExist_ExpectOnlyFutureReturned(t *testing.T) {
	startNow := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	rooms := repository.NewMemoryRoomRepository()
	schedules := repository.NewMemoryScheduleRepository()
	slots := repository.NewMemorySlotRepository()
	bookings := repository.NewMemoryBookingRepository()

	users := repository.NewMemoryUserRepository()
	svc := booking.NewService(rooms, schedules, slots, bookings, users, booking.MockConferenceProvider{}, func() time.Time { return startNow }, 30)
	room, _ := svc.CreateRoom(booking.CreateRoomInput{Role: domain.RoleAdmin, Name: "R", Capacity: 4})
	_, _ = svc.CreateSchedule(booking.CreateScheduleInput{Role: domain.RoleAdmin, RoomID: room.ID, Weekdays: []time.Weekday{time.Saturday}, StartMinutes: 10 * 60, EndMinutes: 12 * 60})

	slotsForDay, _ := svc.GetAvailableSlots(domain.RoleUser, room.ID, startNow)
	if len(slotsForDay) < 2 {
		t.Fatalf("expected at least 2 slots")
	}
	userID := uuid.New()
	created, err := svc.CreateBooking(booking.CreateBookingInput{Role: domain.RoleUser, UserID: userID, SlotID: slotsForDay[1].Slot.ID})
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	later := startNow.Add(3 * time.Hour)
	svcLater := booking.NewService(rooms, schedules, slots, bookings, users, booking.MockConferenceProvider{}, func() time.Time { return later }, 30)
	items, err := svcLater.ListMyBookings(domain.RoleUser, userID)
	if err != nil {
		t.Fatalf("list my bookings: %v", err)
	}
	for _, item := range items {
		if item.Booking.ID == created.ID {
			t.Fatal("past booking should not be returned by /bookings/my")
		}
	}
}

func TestAuthorizationService_WhenRoleViolatesAction_ExpectForbidden(t *testing.T) {
	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(now)

	if _, err := svc.CreateRoom(booking.CreateRoomInput{Role: domain.RoleUser, Name: "X", Capacity: 1}); err != domain.ErrForbidden {
		t.Fatalf("expected forbidden on user create room, got: %v", err)
	}
	if _, _, err := svc.ListAllBookings(domain.RoleUser, 1, 10); err != domain.ErrForbidden {
		t.Fatalf("expected forbidden on user list all bookings, got: %v", err)
	}
}

func TestAuthService_RegisterAndLogin_WhenCredentialsValid_ExpectSuccess(t *testing.T) {
	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(now)

	user, err := svc.Register(booking.RegisterInput{
		Email:    "User@Example.com",
		Password: "Pass123!",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if user.Email != "user@example.com" {
		t.Fatalf("expected normalized email, got %s", user.Email)
	}

	_, err = svc.Register(booking.RegisterInput{
		Email:    "user@example.com",
		Password: "Pass123!",
	})
	if err != domain.ErrEmailAlreadyExists {
		t.Fatalf("expected email conflict, got %v", err)
	}

	_, err = svc.Login(booking.LoginInput{Email: "user@example.com", Password: "Pass123!"})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	_, err = svc.Login(booking.LoginInput{Email: "user@example.com", Password: "wrong"})
	if err != domain.ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}

func TestAuthService_Register_WhenRoleMissing_ExpectDefaultUserRole(t *testing.T) {
	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(now)

	user, err := svc.Register(booking.RegisterInput{
		Email:    "new.user@example.com",
		Password: "Pass123!",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if user.Role != domain.RoleUser {
		t.Fatalf("expected default role user, got %s", user.Role)
	}
}

func TestBookingService_CreateBooking_WhenConferenceLinkOptionUsed_ExpectExpectedStatuses(t *testing.T) {
	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(now)
	room, _ := svc.CreateRoom(booking.CreateRoomInput{Role: domain.RoleAdmin, Name: "Conf", Capacity: 5})
	_, _ = svc.CreateSchedule(booking.CreateScheduleInput{
		Role: domain.RoleAdmin, RoomID: room.ID, Weekdays: []time.Weekday{time.Sunday}, StartMinutes: 9 * 60, EndMinutes: 11 * 60,
	})
	slots, _ := svc.GetAvailableSlots(domain.RoleUser, room.ID, now.AddDate(0, 0, 1))

	b1, err := svc.CreateBooking(booking.CreateBookingInput{
		Role:                 domain.RoleUser,
		UserID:               uuid.New(),
		SlotID:               slots[0].Slot.ID,
		CreateConferenceLink: false,
	})
	if err != nil {
		t.Fatalf("create booking no conference: %v", err)
	}
	if b1.ConferenceStatus != "not_requested" {
		t.Fatalf("expected not_requested, got %s", b1.ConferenceStatus)
	}
	if b1.ConferenceLink != nil {
		t.Fatalf("conference link should be nil when not requested")
	}

	b2, err := svc.CreateBooking(booking.CreateBookingInput{
		Role:                 domain.RoleUser,
		UserID:               uuid.New(),
		SlotID:               slots[1].Slot.ID,
		CreateConferenceLink: true,
	})
	if err != nil {
		t.Fatalf("create booking with conference: %v", err)
	}
	if b2.ConferenceStatus != "created" || b2.ConferenceLink == nil {
		t.Fatalf("expected created conference with link, got status=%s link=%v", b2.ConferenceStatus, b2.ConferenceLink)
	}
}
