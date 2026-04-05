package application

import (
	"github.com/google/uuid"
	domain "meeting-booking/app/entities"
)

func (s *Service) CancelBooking(role domain.Role, userID, bookingID uuid.UUID) (domain.Booking, error) {
	if role != domain.RoleUser {
		return domain.Booking{}, domain.ErrForbidden
	}
	booking, ok, err := s.bookings.GetByID(bookingID)
	if err != nil {
		return domain.Booking{}, err
	}
	if !ok {
		return domain.Booking{}, domain.ErrBookingNotFound
	}
	if booking.UserID != userID {
		return domain.Booking{}, domain.ErrForbidden
	}
	if booking.Status == domain.BookingCancelled {
		return booking, nil
	}
	now := s.nowUTC()
	booking.Status = domain.BookingCancelled
	booking.CancelledAt = &now
	if err := s.bookings.Save(booking); err != nil {
		return domain.Booking{}, err
	}
	return booking, nil
}
