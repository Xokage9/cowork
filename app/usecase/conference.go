package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ConferenceProvider interface {
	CreateLink(ctx context.Context, bookingID, roomID, userID uuid.UUID, startAt, endAt time.Time) (string, error)
}

type MockConferenceProvider struct {
	Fail bool
}

func (m MockConferenceProvider) CreateLink(_ context.Context, bookingID, roomID, userID uuid.UUID, _ time.Time, _ time.Time) (string, error) {
	if m.Fail {
		return "", fmt.Errorf("conference service unavailable")
	}
	return fmt.Sprintf("https://conference.local/%s/%s/%s", roomID.String(), userID.String(), bookingID.String()), nil
}
