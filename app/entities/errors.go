package domain

import "errors"

var (
	ErrForbidden             = errors.New("forbidden")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrRoomNotFound          = errors.New("room not found")
	ErrScheduleAlreadyExists = errors.New("schedule already exists")
	ErrInvalidSchedule       = errors.New("invalid schedule")
	ErrSlotNotFound          = errors.New("slot not found")
	ErrSlotAlreadyBooked     = errors.New("slot already booked")
	ErrCannotBookPastSlot    = errors.New("cannot book past slot")
	ErrBookingNotFound       = errors.New("booking not found")
	ErrEmailAlreadyExists    = errors.New("email already exists")
	ErrInvalidPagination     = errors.New("invalid pagination")
	ErrInvalidCredentials    = errors.New("invalid credentials")
)
