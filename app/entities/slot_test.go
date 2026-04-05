package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	domain "meeting-booking/app/entities"
)

func TestValidateSchedule(t *testing.T) {
	if err := domain.ValidateSchedule([]time.Weekday{time.Monday, time.Wednesday}, 540, 660); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := domain.ValidateSchedule([]time.Weekday{time.Monday, time.Monday}, 540, 660); err == nil {
		t.Fatal("expected duplicate weekdays validation error")
	}

	if err := domain.ValidateSchedule([]time.Weekday{}, 540, 660); err == nil {
		t.Fatal("expected empty weekdays validation error")
	}

	if err := domain.ValidateSchedule([]time.Weekday{time.Monday}, 550, 660); err == nil {
		t.Fatal("expected non-30-minute alignment error")
	}
}

func TestGenerateSlotsForDate(t *testing.T) {
	roomID := uuid.New()
	schedule := domain.Schedule{RoomID: roomID, Weekdays: []time.Weekday{time.Monday}, StartMinutes: 540, EndMinutes: 600}

	slots := domain.GenerateSlotsForDate(roomID, schedule, time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC))
	if len(slots) != 2 {
		t.Fatalf("expected 2 slots got %d", len(slots))
	}
	if !slots[0].StartAt.Before(slots[1].StartAt) {
		t.Fatal("expected slots sorted by start time")
	}

	none := domain.GenerateSlotsForDate(roomID, schedule, time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC))
	if len(none) != 0 {
		t.Fatal("expected empty slots on non-working day")
	}
}
