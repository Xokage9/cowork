package di

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"meeting-booking/app/api"
	domain "meeting-booking/app/entities"
	"meeting-booking/app/repository"
	booking "meeting-booking/app/usecase"
)

func BuildHTTPHandler() http.Handler {
	var (
		rooms     domain.RoomRepository
		schedules domain.ScheduleRepository
		slots     domain.SlotRepository
		bookings  domain.BookingRepository
		users     domain.UserRepository
	)

	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		db, err := repository.OpenPostgresDB(databaseURL)
		if err != nil {
			log.Printf("failed to connect postgres, fallback to in-memory repositories: %v", err)
		} else if err := repository.EnsureSchemaVersion(db); err != nil {
			log.Printf("postgres connectivity check failed, fallback to in-memory repositories: %v", err)
			_ = db.Close()
		} else {
			log.Printf("postgres repositories are enabled")
			rooms = repository.NewPostgresRoomRepository(db)
			schedules = repository.NewPostgresScheduleRepository(db)
			slots = repository.NewPostgresSlotRepository(db)
			bookings = repository.NewPostgresBookingRepository(db)
			users = repository.NewPostgresUserRepository(db)
		}
	}

	if rooms == nil {
		log.Printf("in-memory repositories are enabled")
		rooms = repository.NewMemoryRoomRepository()
		schedules = repository.NewMemoryScheduleRepository()
		slots = repository.NewMemorySlotRepository()
		bookings = repository.NewMemoryBookingRepository()
		users = repository.NewMemoryUserRepository()
	}

	conferenceProvider := booking.MockConferenceProvider{Fail: os.Getenv("CONFERENCE_SERVICE_FAIL") == "true"}
	service := booking.NewService(rooms, schedules, slots, bookings, users, conferenceProvider, time.Now, 30)
	ensureDummyUsers(users)
	h := api.NewHandler(service, os.Getenv("JWT_SECRET"))
	return h.Router()
}

func ensureDummyUsers(users domain.UserRepository) {
	seed := []domain.User{
		{
			ID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			Email:        "dummy-admin@local",
			PasswordHash: "dummy",
			Role:         domain.RoleAdmin,
			Registered:   time.Now().UTC(),
		},
		{
			ID:           uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Email:        "dummy-user@local",
			PasswordHash: "dummy",
			Role:         domain.RoleUser,
			Registered:   time.Now().UTC(),
		},
	}
	for _, user := range seed {
		if _, exists, err := users.GetByID(user.ID); err != nil {
			log.Printf("unable to check dummy user %s: %v", user.ID.String(), err)
			continue
		} else if exists {
			continue
		}
		if err := users.Create(user); err != nil && err != domain.ErrEmailAlreadyExists {
			log.Printf("unable to create dummy user %s: %v", user.ID.String(), err)
		}
	}
}
