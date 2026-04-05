package postgrestest

import (
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	domain "meeting-booking/app/entities"
	"meeting-booking/app/repository"
)

func TestPostgresRoomRepository_Create_WhenInsertSucceeds_ExpectCommit(t *testing.T) {
	// Arrange
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := repository.NewPostgresRoomRepository(db)
	room := domain.Room{ID: uuid.New(), Name: "room", Capacity: 4, CreatedAt: time.Now().UTC()}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO rooms").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Act
	err = repo.Create(room)

	// Assert
	if err != nil {
		t.Fatalf("repo.Create returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgresBookingRepository_Save_WhenBookingMissing_ExpectBookingNotFound(t *testing.T) {
	// Arrange
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := repository.NewPostgresBookingRepository(db)
	booking := domain.Booking{ID: uuid.New(), Status: domain.BookingCancelled}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE bookings SET status").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	// Act
	err = repo.Save(booking)

	// Assert
	if err != domain.ErrBookingNotFound {
		t.Fatalf("expected ErrBookingNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgresUserRepository_Create_WhenUniqueViolation_ExpectEmailAlreadyExists(t *testing.T) {
	// Arrange
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := repository.NewPostgresUserRepository(db)
	user := domain.User{ID: uuid.New(), Email: "user@example.com", PasswordHash: "hash", Role: domain.RoleUser, Registered: time.Now().UTC()}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").WillReturnError(&pgconn.PgError{Code: "23505"})
	mock.ExpectRollback()

	// Act
	err = repo.Create(user)

	// Assert
	if err != domain.ErrEmailAlreadyExists {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
