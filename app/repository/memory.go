package repository

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	domain "meeting-booking/app/entities"
)

type MemoryRoomRepository struct {
	mu    sync.RWMutex
	rooms map[uuid.UUID]domain.Room
}

func NewMemoryRoomRepository() *MemoryRoomRepository {
	return &MemoryRoomRepository{rooms: map[uuid.UUID]domain.Room{}}
}

func (r *MemoryRoomRepository) Create(room domain.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms[room.ID] = room
	return nil
}

func (r *MemoryRoomRepository) List() ([]domain.Room, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domain.Room, 0, len(r.rooms))
	for _, room := range r.rooms {
		items = append(items, room)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (r *MemoryRoomRepository) Get(id uuid.UUID) (domain.Room, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	room, ok := r.rooms[id]
	return room, ok, nil
}

type MemoryScheduleRepository struct {
	mu        sync.RWMutex
	schedules map[uuid.UUID]domain.Schedule
}

func NewMemoryScheduleRepository() *MemoryScheduleRepository {
	return &MemoryScheduleRepository{schedules: map[uuid.UUID]domain.Schedule{}}
}

func (r *MemoryScheduleRepository) Create(schedule domain.Schedule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.schedules[schedule.RoomID]; ok {
		return domain.ErrScheduleAlreadyExists
	}
	r.schedules[schedule.RoomID] = schedule
	return nil
}

func (r *MemoryScheduleRepository) GetByRoomID(roomID uuid.UUID) (domain.Schedule, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schedules[roomID]
	return s, ok, nil
}

type MemorySlotRepository struct {
	mu          sync.RWMutex
	slotsByID   map[uuid.UUID]domain.Slot
	slotsByDay  map[uuid.UUID]map[string][]uuid.UUID
	slotByRange map[string]uuid.UUID
}

func NewMemorySlotRepository() *MemorySlotRepository {
	return &MemorySlotRepository{
		slotsByID:   map[uuid.UUID]domain.Slot{},
		slotsByDay:  map[uuid.UUID]map[string][]uuid.UUID{},
		slotByRange: map[string]uuid.UUID{},
	}
}

func dayKey(t time.Time) string {
	t = t.UTC()
	return t.Format("2006-01-02")
}

func intervalKey(roomID uuid.UUID, startAt, endAt time.Time) string {
	return fmt.Sprintf("%s|%s|%s", roomID.String(), startAt.UTC().Format(time.RFC3339Nano), endAt.UTC().Format(time.RFC3339Nano))
}

func (r *MemorySlotRepository) SaveBatch(slots []domain.Slot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, slot := range slots {
		k := intervalKey(slot.RoomID, slot.StartAt, slot.EndAt)
		if _, exists := r.slotByRange[k]; exists {
			continue
		}
		if _, exists := r.slotsByID[slot.ID]; exists {
			continue
		}

		r.slotsByID[slot.ID] = slot
		r.slotByRange[k] = slot.ID
		if _, ok := r.slotsByDay[slot.RoomID]; !ok {
			r.slotsByDay[slot.RoomID] = map[string][]uuid.UUID{}
		}
		key := dayKey(slot.StartAt)
		r.slotsByDay[slot.RoomID][key] = append(r.slotsByDay[slot.RoomID][key], slot.ID)
	}
	return nil
}

func (r *MemorySlotRepository) FindByRoomAndDate(roomID uuid.UUID, date time.Time) ([]domain.Slot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	byDate, ok := r.slotsByDay[roomID]
	if !ok {
		return nil, nil
	}
	ids := byDate[dayKey(date)]
	slots := make([]domain.Slot, 0, len(ids))
	for _, id := range ids {
		slots = append(slots, r.slotsByID[id])
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].StartAt.Before(slots[j].StartAt) })
	return slots, nil
}

func (r *MemorySlotRepository) FindByID(slotID uuid.UUID) (domain.Slot, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.slotsByID[slotID]
	return s, ok, nil
}

func (r *MemorySlotRepository) HasSlotsForRoomAndDate(roomID uuid.UUID, date time.Time) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	byDate, ok := r.slotsByDay[roomID]
	if !ok {
		return false, nil
	}
	ids := byDate[dayKey(date)]
	return len(ids) > 0, nil
}

type MemoryBookingRepository struct {
	mu             sync.RWMutex
	bookingsByID   map[uuid.UUID]domain.Booking
	activeBySlotID map[uuid.UUID]uuid.UUID
}

func NewMemoryBookingRepository() *MemoryBookingRepository {
	return &MemoryBookingRepository{
		bookingsByID:   map[uuid.UUID]domain.Booking{},
		activeBySlotID: map[uuid.UUID]uuid.UUID{},
	}
}

func (r *MemoryBookingRepository) Create(booking domain.Booking) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if activeID, ok := r.activeBySlotID[booking.SlotID]; ok {
		active, activeExists := r.bookingsByID[activeID]
		if activeExists && active.Status == domain.BookingActive {
			return domain.ErrSlotAlreadyBooked
		}
	}
	r.bookingsByID[booking.ID] = booking
	r.activeBySlotID[booking.SlotID] = booking.ID
	return nil
}

func (r *MemoryBookingRepository) GetByID(bookingID uuid.UUID) (domain.Booking, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.bookingsByID[bookingID]
	return b, ok, nil
}

func (r *MemoryBookingRepository) GetActiveBySlotID(slotID uuid.UUID) (domain.Booking, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bookingID, ok := r.activeBySlotID[slotID]
	if !ok {
		return domain.Booking{}, false, nil
	}
	b, ok := r.bookingsByID[bookingID]
	if !ok || b.Status != domain.BookingActive {
		return domain.Booking{}, false, nil
	}
	return b, true, nil
}

func (r *MemoryBookingRepository) Save(booking domain.Booking) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bookingsByID[booking.ID] = booking
	if booking.Status == domain.BookingCancelled {
		delete(r.activeBySlotID, booking.SlotID)
	}
	return nil
}

func (r *MemoryBookingRepository) ListByUserIDFuture(userID uuid.UUID, now time.Time) ([]domain.Booking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domain.Booking, 0)
	for _, b := range r.bookingsByID {
		if b.UserID == userID {
			items = append(items, b)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (r *MemoryBookingRepository) ListAll(offset, limit int) ([]domain.Booking, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]domain.Booking, 0, len(r.bookingsByID))
	for _, b := range r.bookingsByID {
		all = append(all, b)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.Before(all[j].CreatedAt) })
	total := len(all)
	if offset >= total {
		return []domain.Booking{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

type MemoryUserRepository struct {
	mu      sync.RWMutex
	users   map[uuid.UUID]domain.User
	byEmail map[string]uuid.UUID
}

func NewMemoryUserRepository() *MemoryUserRepository {
	return &MemoryUserRepository{
		users:   map[uuid.UUID]domain.User{},
		byEmail: map[string]uuid.UUID{},
	}
}

func (r *MemoryUserRepository) Create(user domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byEmail[user.Email]; exists {
		return domain.ErrEmailAlreadyExists
	}
	r.users[user.ID] = user
	r.byEmail[user.Email] = user.ID
	return nil
}

func (r *MemoryUserRepository) GetByID(userID uuid.UUID) (domain.User, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.users[userID]
	return user, ok, nil
}

func (r *MemoryUserRepository) GetByEmail(email string) (domain.User, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byEmail[email]
	if !ok {
		return domain.User{}, false, nil
	}
	user, ok := r.users[id]
	return user, ok, nil
}
