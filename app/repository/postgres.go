package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	domain "meeting-booking/app/entities"
)

func OpenPostgresDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func weekdaysToArrayLiteral(days []time.Weekday) string {
	if len(days) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(days))
	for _, day := range days {
		parts = append(parts, strconv.Itoa(int(day)))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func parseWeekdaysCSV(csv string) ([]time.Weekday, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return []time.Weekday{}, nil
	}
	parts := strings.Split(csv, ",")
	result := make([]time.Weekday, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		result = append(result, time.Weekday(n))
	}
	return result, nil
}

func withTx(db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

type PostgresRoomRepository struct {
	db *sql.DB
}

func NewPostgresRoomRepository(db *sql.DB) *PostgresRoomRepository {
	return &PostgresRoomRepository{db: db}
}

func (r *PostgresRoomRepository) Create(room domain.Room) error {
	return withTx(r.db, func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO rooms (id, name, description, capacity, created_at) VALUES ($1, $2, $3, $4, $5)`,
			room.ID, room.Name, nullableString(room.Description), room.Capacity, room.CreatedAt.UTC(),
		)
		return err
	})
}

func (r *PostgresRoomRepository) List() ([]domain.Room, error) {
	rows, err := r.db.Query(`SELECT id, name, description, capacity, created_at FROM rooms ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Room, 0)
	for rows.Next() {
		var room domain.Room
		var description sql.NullString
		if err := rows.Scan(&room.ID, &room.Name, &description, &room.Capacity, &room.CreatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			room.Description = description.String
		}
		room.CreatedAt = room.CreatedAt.UTC()
		items = append(items, room)
	}
	return items, rows.Err()
}

func (r *PostgresRoomRepository) Get(id uuid.UUID) (domain.Room, bool, error) {
	var room domain.Room
	var description sql.NullString
	err := r.db.QueryRow(
		`SELECT id, name, description, capacity, created_at FROM rooms WHERE id = $1`,
		id,
	).Scan(&room.ID, &room.Name, &description, &room.Capacity, &room.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Room{}, false, nil
	}
	if err != nil {
		return domain.Room{}, false, err
	}
	if description.Valid {
		room.Description = description.String
	}
	room.CreatedAt = room.CreatedAt.UTC()
	return room, true, nil
}

type PostgresScheduleRepository struct {
	db *sql.DB
}

func NewPostgresScheduleRepository(db *sql.DB) *PostgresScheduleRepository {
	return &PostgresScheduleRepository{db: db}
}

func (r *PostgresScheduleRepository) Create(schedule domain.Schedule) error {
	err := withTx(r.db, func(tx *sql.Tx) error {
		_, txErr := tx.Exec(
			`INSERT INTO schedules (room_id, weekdays, start_minutes, end_minutes, created_at) VALUES ($1, $2::smallint[], $3, $4, $5)`,
			schedule.RoomID, weekdaysToArrayLiteral(schedule.Weekdays), schedule.StartMinutes, schedule.EndMinutes, schedule.CreatedAt.UTC(),
		)
		return txErr
	})
	if isUniqueViolation(err) {
		return domain.ErrScheduleAlreadyExists
	}
	return err
}

func (r *PostgresScheduleRepository) GetByRoomID(roomID uuid.UUID) (domain.Schedule, bool, error) {
	var s domain.Schedule
	var weekdaysCSV string
	err := r.db.QueryRow(
		`SELECT room_id, array_to_string(weekdays, ','), start_minutes, end_minutes, created_at FROM schedules WHERE room_id = $1`,
		roomID,
	).Scan(&s.RoomID, &weekdaysCSV, &s.StartMinutes, &s.EndMinutes, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Schedule{}, false, nil
	}
	if err != nil {
		return domain.Schedule{}, false, err
	}
	weekdays, err := parseWeekdaysCSV(weekdaysCSV)
	if err != nil {
		return domain.Schedule{}, false, err
	}
	s.Weekdays = weekdays
	s.CreatedAt = s.CreatedAt.UTC()
	return s, true, nil
}

type PostgresSlotRepository struct {
	db *sql.DB
}

func NewPostgresSlotRepository(db *sql.DB) *PostgresSlotRepository {
	return &PostgresSlotRepository{db: db}
}

func (r *PostgresSlotRepository) SaveBatch(slots []domain.Slot) error {
	if len(slots) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO slots (id, room_id, start_at, end_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (room_id, start_at, end_at) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, slot := range slots {
		if _, err := stmt.Exec(slot.ID, slot.RoomID, slot.StartAt.UTC(), slot.EndAt.UTC()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *PostgresSlotRepository) FindByRoomAndDate(roomID uuid.UUID, date time.Time) ([]domain.Slot, error) {
	start := time.Date(date.UTC().Year(), date.UTC().Month(), date.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	rows, err := r.db.Query(
		`SELECT id, room_id, start_at, end_at FROM slots WHERE room_id = $1 AND start_at >= $2 AND start_at < $3 ORDER BY start_at ASC`,
		roomID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Slot, 0)
	for rows.Next() {
		var slot domain.Slot
		if err := rows.Scan(&slot.ID, &slot.RoomID, &slot.StartAt, &slot.EndAt); err != nil {
			return nil, err
		}
		slot.StartAt = slot.StartAt.UTC()
		slot.EndAt = slot.EndAt.UTC()
		items = append(items, slot)
	}
	return items, rows.Err()
}

func (r *PostgresSlotRepository) FindByID(slotID uuid.UUID) (domain.Slot, bool, error) {
	var slot domain.Slot
	err := r.db.QueryRow(
		`SELECT id, room_id, start_at, end_at FROM slots WHERE id = $1`,
		slotID,
	).Scan(&slot.ID, &slot.RoomID, &slot.StartAt, &slot.EndAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Slot{}, false, nil
	}
	if err != nil {
		return domain.Slot{}, false, err
	}
	slot.StartAt = slot.StartAt.UTC()
	slot.EndAt = slot.EndAt.UTC()
	return slot, true, nil
}

func (r *PostgresSlotRepository) HasSlotsForRoomAndDate(roomID uuid.UUID, date time.Time) (bool, error) {
	start := time.Date(date.UTC().Year(), date.UTC().Month(), date.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	var exists bool
	err := r.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM slots WHERE room_id = $1 AND start_at >= $2 AND start_at < $3)`,
		roomID, start, end,
	).Scan(&exists)
	return exists, err
}

type PostgresBookingRepository struct {
	db *sql.DB
}

func NewPostgresBookingRepository(db *sql.DB) *PostgresBookingRepository {
	return &PostgresBookingRepository{db: db}
}

func (r *PostgresBookingRepository) Create(booking domain.Booking) error {
	err := withTx(r.db, func(tx *sql.Tx) error {
		_, txErr := tx.Exec(
			`INSERT INTO bookings (id, slot_id, room_id, user_id, status, conference_link, conference_status, created_at, cancelled_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			booking.ID, booking.SlotID, booking.RoomID, booking.UserID, booking.Status, booking.ConferenceLink, booking.ConferenceStatus, booking.CreatedAt.UTC(), booking.CancelledAt,
		)
		return txErr
	})
	if isUniqueViolation(err) {
		return domain.ErrSlotAlreadyBooked
	}
	return err
}

func (r *PostgresBookingRepository) GetByID(bookingID uuid.UUID) (domain.Booking, bool, error) {
	var b domain.Booking
	err := r.db.QueryRow(
		`SELECT id, slot_id, room_id, user_id, status, conference_link, conference_status, created_at, cancelled_at FROM bookings WHERE id = $1`,
		bookingID,
	).Scan(&b.ID, &b.SlotID, &b.RoomID, &b.UserID, &b.Status, &b.ConferenceLink, &b.ConferenceStatus, &b.CreatedAt, &b.CancelledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Booking{}, false, nil
	}
	if err != nil {
		return domain.Booking{}, false, err
	}
	b.CreatedAt = b.CreatedAt.UTC()
	if b.CancelledAt != nil {
		t := b.CancelledAt.UTC()
		b.CancelledAt = &t
	}
	return b, true, nil
}

func (r *PostgresBookingRepository) GetActiveBySlotID(slotID uuid.UUID) (domain.Booking, bool, error) {
	var b domain.Booking
	err := r.db.QueryRow(
		`SELECT id, slot_id, room_id, user_id, status, conference_link, conference_status, created_at, cancelled_at
		 FROM bookings WHERE slot_id = $1 AND status = 'active'`,
		slotID,
	).Scan(&b.ID, &b.SlotID, &b.RoomID, &b.UserID, &b.Status, &b.ConferenceLink, &b.ConferenceStatus, &b.CreatedAt, &b.CancelledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Booking{}, false, nil
	}
	if err != nil {
		return domain.Booking{}, false, err
	}
	b.CreatedAt = b.CreatedAt.UTC()
	return b, true, nil
}

func (r *PostgresBookingRepository) Save(booking domain.Booking) error {
	return withTx(r.db, func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`UPDATE bookings SET status = $2, conference_link = $3, conference_status = $4, cancelled_at = $5 WHERE id = $1`,
			booking.ID, booking.Status, booking.ConferenceLink, booking.ConferenceStatus, booking.CancelledAt,
		)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return domain.ErrBookingNotFound
		}
		return nil
	})
}

func (r *PostgresBookingRepository) ListByUserIDFuture(userID uuid.UUID, now time.Time) ([]domain.Booking, error) {
	rows, err := r.db.Query(
		`SELECT b.id, b.slot_id, b.room_id, b.user_id, b.status, b.conference_link, b.conference_status, b.created_at, b.cancelled_at
		 FROM bookings b
		 JOIN slots s ON s.id = b.slot_id
		 WHERE b.user_id = $1 AND s.end_at > $2
		 ORDER BY b.created_at ASC`,
		userID, now.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Booking, 0)
	for rows.Next() {
		var b domain.Booking
		if err := rows.Scan(&b.ID, &b.SlotID, &b.RoomID, &b.UserID, &b.Status, &b.ConferenceLink, &b.ConferenceStatus, &b.CreatedAt, &b.CancelledAt); err != nil {
			return nil, err
		}
		b.CreatedAt = b.CreatedAt.UTC()
		if b.CancelledAt != nil {
			t := b.CancelledAt.UTC()
			b.CancelledAt = &t
		}
		items = append(items, b)
	}
	return items, rows.Err()
}

func (r *PostgresBookingRepository) ListAll(offset, limit int) ([]domain.Booking, int, error) {
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM bookings`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(
		`SELECT id, slot_id, room_id, user_id, status, conference_link, conference_status, created_at, cancelled_at
		 FROM bookings
		 ORDER BY created_at ASC
		 OFFSET $1 LIMIT $2`,
		offset, limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.Booking, 0)
	for rows.Next() {
		var b domain.Booking
		if err := rows.Scan(&b.ID, &b.SlotID, &b.RoomID, &b.UserID, &b.Status, &b.ConferenceLink, &b.ConferenceStatus, &b.CreatedAt, &b.CancelledAt); err != nil {
			return nil, 0, err
		}
		b.CreatedAt = b.CreatedAt.UTC()
		if b.CancelledAt != nil {
			t := b.CancelledAt.UTC()
			b.CancelledAt = &t
		}
		items = append(items, b)
	}
	return items, total, rows.Err()
}

func nullableString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func EnsureSchemaVersion(db *sql.DB) error {
	var one int
	if err := db.QueryRow(`SELECT 1`).Scan(&one); err != nil {
		return fmt.Errorf("postgres ping query failed: %w", err)
	}
	return nil
}

type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) Create(user domain.User) error {
	err := withTx(r.db, func(tx *sql.Tx) error {
		_, txErr := tx.Exec(
			`INSERT INTO users (id, email, password_hash, role, registered_at) VALUES ($1, $2, $3, $4, $5)`,
			user.ID, user.Email, user.PasswordHash, user.Role, user.Registered.UTC(),
		)
		return txErr
	})
	if isUniqueViolation(err) {
		return domain.ErrEmailAlreadyExists
	}
	return err
}

func (r *PostgresUserRepository) GetByID(userID uuid.UUID) (domain.User, bool, error) {
	var user domain.User
	err := r.db.QueryRow(
		`SELECT id, email, password_hash, role, registered_at FROM users WHERE id = $1`,
		userID,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.Registered)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	user.Registered = user.Registered.UTC()
	return user, true, nil
}

func (r *PostgresUserRepository) GetByEmail(email string) (domain.User, bool, error) {
	var user domain.User
	err := r.db.QueryRow(
		`SELECT id, email, password_hash, role, registered_at FROM users WHERE email = $1`,
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.Registered)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	user.Registered = user.Registered.UTC()
	return user, true, nil
}
