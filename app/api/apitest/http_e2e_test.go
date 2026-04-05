package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"meeting-booking/app/di"
)

func doJSON(t *testing.T, client *http.Client, method, url, token string, payload any) (*http.Response, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("encode payload: %v", err)
		}
	}

	req, err := http.NewRequest(method, url, &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	parsed := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	return resp, parsed
}

func nestedMap(t *testing.T, body map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := body[key].(map[string]any)
	if !ok {
		t.Fatalf("expected object field %q, body=%v", key, body)
	}
	return value
}

func login(t *testing.T, client *http.Client, baseURL, role string) string {
	t.Helper()
	resp, body := doJSON(t, client, http.MethodPost, baseURL+"/dummyLogin", "", map[string]any{"role": role})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dummyLogin status=%d body=%v", resp.StatusCode, body)
	}
	token, ok := body["token"].(string)
	if !ok || token == "" {
		t.Fatalf("missing token in response: %v", body)
	}
	return token
}

func pickFutureDate() string {
	return time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
}

func TestAPI_CreateBookingFlow_WhenRoomAndScheduleExist_ExpectCreated(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	userToken := login(t, client, server.URL, "user")

	resp, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "Big Meeting", "description": "Main room", "capacity": 12,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create room status=%d body=%v", resp.StatusCode, roomBody)
	}
	room := nestedMap(t, roomBody, "room")
	roomID := room["id"].(string)

	resp, schBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7}, "startTime": "09:00", "endTime": "17:00",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create schedule status=%d body=%v", resp.StatusCode, schBody)
	}

	resp, slotsBody := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list?date="+pickFutureDate(), userToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get slots status=%d body=%v", resp.StatusCode, slotsBody)
	}
	slotsAny, ok := slotsBody["slots"].([]any)
	if !ok || len(slotsAny) == 0 {
		t.Fatalf("expected non-empty slots body=%v", slotsBody)
	}
	firstSlot, ok := slotsAny[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid slot payload: %v", slotsAny[0])
	}
	slotID := firstSlot["id"].(string)
	if _, err := uuid.Parse(slotID); err != nil {
		t.Fatalf("invalid slot uuid: %v", err)
	}

	resp, bookingBody := doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{"slotId": slotID})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create booking status=%d body=%v", resp.StatusCode, bookingBody)
	}
}

func TestAPI_CancelBooking_WhenCalledTwice_ExpectIdempotentStatus(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	userToken := login(t, client, server.URL, "user")

	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{"name": "Room 2", "capacity": 8})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7}, "startTime": "10:00", "endTime": "19:00",
	})

	_, slotsBody := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list?date="+pickFutureDate(), userToken, nil)
	slots := slotsBody["slots"].([]any)
	slotID := slots[0].(map[string]any)["id"].(string)

	_, bookingBody := doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{"slotId": slotID})
	bookingID := nestedMap(t, bookingBody, "booking")["id"].(string)

	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/bookings/"+bookingID+"/cancel", userToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first cancel status=%d body=%v", resp.StatusCode, body)
	}
	if nestedMap(t, body, "booking")["status"].(string) != "cancelled" {
		t.Fatalf("unexpected status after first cancel: %v", body)
	}

	resp, body = doJSON(t, client, http.MethodPost, server.URL+"/bookings/"+bookingID+"/cancel", userToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second cancel status=%d body=%v", resp.StatusCode, body)
	}
	if nestedMap(t, body, "booking")["status"].(string) != "cancelled" {
		t.Fatalf("unexpected status after second cancel: %v", body)
	}
}

func TestAPI_InfoEndpoint_WhenCalled_ExpectOK(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/_info")
	if err != nil {
		t.Fatalf("request /_info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /_info, got %d", resp.StatusCode)
	}
}

func TestAPI_CreateBooking_WhenCallerIsAdmin_ExpectForbidden(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")

	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", adminToken, map[string]any{"slotId": uuid.New().String()})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for admin booking, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAPI_RegisterAndLogin_WhenCredentialsValid_ExpectTokenIssued(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/register", "", map[string]any{
		"email":    "test.user@example.com",
		"password": "StrongPass123!",
		"role":     "user",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status=%d body=%v", resp.StatusCode, body)
	}
	if _, ok := nestedMap(t, body, "user")["id"].(string); !ok {
		t.Fatalf("register should return user body=%v", body)
	}

	resp, body = doJSON(t, client, http.MethodPost, server.URL+"/login", "", map[string]any{
		"email":    "test.user@example.com",
		"password": "StrongPass123!",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d body=%v", resp.StatusCode, body)
	}
	if _, ok := body["token"].(string); !ok {
		t.Fatalf("login should return token body=%v", body)
	}
}

func TestAPI_Register_WhenEmailAlreadyExists_ExpectBadRequest(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/register", "", map[string]any{
		"email":    "dup@example.com",
		"password": "StrongPass123!",
		"role":     "user",
	})
	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/register", "", map[string]any{
		"email":    "dup@example.com",
		"password": "StrongPass123!",
		"role":     "user",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on duplicate register, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAPI_Login_WhenPasswordInvalid_ExpectUnauthorized(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/register", "", map[string]any{
		"email":    "auth@example.com",
		"password": "StrongPass123!",
		"role":     "user",
	})
	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/login", "", map[string]any{
		"email":    "auth@example.com",
		"password": "WrongPassword!",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 on invalid login, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAPI_CreateBooking_WhenConferenceLinkRequested_ExpectConferenceLinkInResponse(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	userToken := login(t, client, server.URL, "user")

	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "Conference Room", "description": "with conference", "capacity": 6,
	})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7}, "startTime": "09:00", "endTime": "17:00",
	})

	_, slotsBody := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list?date="+pickFutureDate(), userToken, nil)
	slotID := slotsBody["slots"].([]any)[0].(map[string]any)["id"].(string)

	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{
		"slotId":               slotID,
		"createConferenceLink": true,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create booking status=%d body=%v", resp.StatusCode, body)
	}
	if _, ok := nestedMap(t, body, "booking")["conferenceLink"]; !ok {
		t.Fatalf("expected conferenceLink in response, body=%v", body)
	}
}

func TestAPI_CreateBooking_WhenConferenceLinkNotRequested_ExpectNoConferenceLink(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	userToken := login(t, client, server.URL, "user")

	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "No Conference Room", "capacity": 4,
	})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7}, "startTime": "09:00", "endTime": "10:00",
	})

	_, slotsBody := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list?date="+pickFutureDate(), userToken, nil)
	slotID := slotsBody["slots"].([]any)[0].(map[string]any)["id"].(string)

	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{
		"slotId": slotID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create booking status=%d body=%v", resp.StatusCode, body)
	}
	if _, ok := nestedMap(t, body, "booking")["conferenceLink"]; ok {
		t.Fatalf("did not expect conferenceLink in response, body=%v", body)
	}
}

func TestAPI_CreateBooking_WhenConferenceProviderFails_ExpectBookingStillCreated(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	t.Setenv("CONFERENCE_SERVICE_FAIL", "true")

	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	userToken := login(t, client, server.URL, "user")

	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "Failure Conference Room", "capacity": 4,
	})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7}, "startTime": "09:00", "endTime": "10:00",
	})

	_, slotsBody := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list?date="+pickFutureDate(), userToken, nil)
	slotID := slotsBody["slots"].([]any)[0].(map[string]any)["id"].(string)

	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{
		"slotId":               slotID,
		"createConferenceLink": true,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create booking status=%d body=%v", resp.StatusCode, body)
	}
	if _, ok := nestedMap(t, body, "booking")["conferenceLink"]; ok {
		t.Fatalf("did not expect conferenceLink on provider failure, body=%v", body)
	}
}

func TestAPI_ListRooms_WhenAuthorized_ExpectRoomsArray(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")

	resp, _ := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "List Room", "capacity": 4,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create room status=%d", resp.StatusCode)
	}

	resp, body := doJSON(t, client, http.MethodGet, server.URL+"/rooms/list", adminToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list rooms status=%d body=%v", resp.StatusCode, body)
	}
	if _, ok := body["rooms"].([]any); !ok {
		t.Fatalf("expected rooms array body=%v", body)
	}
}

func TestAPI_ListAllBookings_WhenAdminRequests_ExpectPaginationPayload(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	userToken := login(t, client, server.URL, "user")

	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "Bookings List Room", "capacity": 6,
	})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7}, "startTime": "09:00", "endTime": "10:00",
	})
	_, slotsBody := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list?date="+pickFutureDate(), userToken, nil)
	slotID := slotsBody["slots"].([]any)[0].(map[string]any)["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{"slotId": slotID})

	resp, body := doJSON(t, client, http.MethodGet, server.URL+"/bookings/list?page=1&pageSize=20", adminToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bookings list status=%d body=%v", resp.StatusCode, body)
	}
	if _, ok := body["bookings"].([]any); !ok {
		t.Fatalf("expected bookings array body=%v", body)
	}
	if _, ok := nestedMap(t, body, "pagination")["total"].(float64); !ok {
		t.Fatalf("expected pagination.total body=%v", body)
	}
}

func TestAPI_MyBookings_WhenUserRequests_ExpectOnlyOwnBookingsPayload(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	userToken := login(t, client, server.URL, "user")

	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "My Bookings Room", "capacity": 6,
	})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7}, "startTime": "09:00", "endTime": "10:00",
	})
	_, slotsBody := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list?date="+pickFutureDate(), userToken, nil)
	slotID := slotsBody["slots"].([]any)[0].(map[string]any)["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{"slotId": slotID})

	resp, body := doJSON(t, client, http.MethodGet, server.URL+"/bookings/my", userToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("my bookings status=%d body=%v", resp.StatusCode, body)
	}
	if _, ok := body["bookings"].([]any); !ok {
		t.Fatalf("expected bookings array body=%v", body)
	}
}

func TestAPI_DummyLogin_WhenRoleInvalid_ExpectBadRequest(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/dummyLogin", "", map[string]any{"role": "manager"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid role, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAPI_Slots_WhenDateMissing_ExpectBadRequest(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "Slots Date Required", "capacity": 5,
	})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)

	resp, body := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list", adminToken, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing date, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAPI_ScheduleCreate_WhenWeekdayInvalid_ExpectBadRequest(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "Schedule Validation", "capacity": 5,
	})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)

	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{0}, "startTime": "09:00", "endTime": "10:00",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid daysOfWeek, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAPI_CreateBooking_WhenSlotAlreadyBooked_ExpectConflict(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	userToken := login(t, client, server.URL, "user")

	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "Double Booking", "capacity": 6,
	})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7}, "startTime": "09:00", "endTime": "10:00",
	})
	_, slotsBody := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list?date="+pickFutureDate(), userToken, nil)
	slotID := slotsBody["slots"].([]any)[0].(map[string]any)["id"].(string)

	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{"slotId": slotID})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first booking status=%d body=%v", resp.StatusCode, body)
	}

	resp, body = doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{"slotId": slotID})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 on second booking, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAPI_CancelBooking_WhenBookingBelongsToAnotherUser_ExpectForbidden(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	adminToken := login(t, client, server.URL, "admin")
	userToken := login(t, client, server.URL, "user")

	_, roomBody := doJSON(t, client, http.MethodPost, server.URL+"/rooms/create", adminToken, map[string]any{
		"name": "Ownership Check", "capacity": 6,
	})
	roomID := nestedMap(t, roomBody, "room")["id"].(string)
	_, _ = doJSON(t, client, http.MethodPost, server.URL+"/rooms/"+roomID+"/schedule/create", adminToken, map[string]any{
		"roomId": roomID, "daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7}, "startTime": "09:00", "endTime": "10:00",
	})
	_, slotsBody := doJSON(t, client, http.MethodGet, server.URL+"/rooms/"+roomID+"/slots/list?date="+pickFutureDate(), userToken, nil)
	slotID := slotsBody["slots"].([]any)[0].(map[string]any)["id"].(string)
	_, bookingBody := doJSON(t, client, http.MethodPost, server.URL+"/bookings/create", userToken, map[string]any{"slotId": slotID})
	bookingID := nestedMap(t, bookingBody, "booking")["id"].(string)

	otherUserToken := mustIssueForeignUserToken(t, client, server.URL)
	resp, body := doJSON(t, client, http.MethodPost, server.URL+"/bookings/"+bookingID+"/cancel", otherUserToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for foreign booking, got %d body=%v", resp.StatusCode, body)
	}
}

func TestAPI_AuthMiddleware_WhenBearerMissing_ExpectUnauthorized(t *testing.T) {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()
	client := server.Client()

	resp, body := doJSON(t, client, http.MethodGet, server.URL+"/rooms/list", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d body=%v", resp.StatusCode, body)
	}
}

func mustIssueForeignUserToken(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()

	foreignEmail := "foreign.user@example.com"
	foreignPassword := "StrongPass123!"

	_, _ = doJSON(t, client, http.MethodPost, baseURL+"/register", "", map[string]any{
		"email": foreignEmail, "password": foreignPassword, "role": "user",
	})
	resp, body := doJSON(t, client, http.MethodPost, baseURL+"/login", "", map[string]any{
		"email": foreignEmail, "password": foreignPassword,
	})
	if resp.StatusCode == http.StatusOK {
		token, ok := body["token"].(string)
		if ok && token != "" {
			return token
		}
	}

	t.Fatalf("failed to obtain foreign user token: status=%d body=%v", resp.StatusCode, body)
	return ""
}
