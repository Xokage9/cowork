package domain

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

const SlotDurationMinutes = 30

func ValidateSchedule(weekdays []time.Weekday, startMinutes, endMinutes int) error {
	if len(weekdays) == 0 {
		return ErrInvalidSchedule
	}
	seen := map[time.Weekday]struct{}{}
	for _, d := range weekdays {
		if d < time.Sunday || d > time.Saturday {
			return ErrInvalidSchedule
		}
		seen[d] = struct{}{}
	}
	if len(seen) != len(weekdays) {
		return ErrInvalidSchedule
	}
	if startMinutes < 0 || endMinutes > 24*60 || startMinutes >= endMinutes {
		return ErrInvalidSchedule
	}
	if (endMinutes-startMinutes)%SlotDurationMinutes != 0 {
		return ErrInvalidSchedule
	}
	return nil
}

func GenerateSlotsForDate(roomID uuid.UUID, schedule Schedule, date time.Time) []Slot {
	if date.Location() != time.UTC {
		date = date.UTC()
	}
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	weekday := dayStart.Weekday()
	allowed := false
	for _, wd := range schedule.Weekdays {
		if wd == weekday {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil
	}

	slots := make([]Slot, 0, (schedule.EndMinutes-schedule.StartMinutes)/SlotDurationMinutes)
	for minute := schedule.StartMinutes; minute+SlotDurationMinutes <= schedule.EndMinutes; minute += SlotDurationMinutes {
		start := dayStart.Add(time.Duration(minute) * time.Minute)
		slots = append(slots, Slot{
			ID:      uuid.New(),
			RoomID:  roomID,
			StartAt: start,
			EndAt:   start.Add(SlotDurationMinutes * time.Minute),
		})
	}

	sort.Slice(slots, func(i, j int) bool {
		return slots[i].StartAt.Before(slots[j].StartAt)
	})

	return slots
}
