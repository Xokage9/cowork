package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	domain "meeting-booking/app/entities"
	booking "meeting-booking/app/usecase"
	"meeting-booking/app/view"
)

var (
	adminUserID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userUserID  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

type Handler struct {
	service   *booking.Service
	jwtSecret []byte
	tokenTTL  time.Duration
	now       func() time.Time
}

func NewHandler(service *booking.Service, jwtSecret string) *Handler {
	if jwtSecret == "" {
		jwtSecret = "dev-secret"
	}
	return &Handler{service: service, jwtSecret: []byte(jwtSecret), tokenTTL: 24 * time.Hour, now: time.Now}
}

type authContext struct {
	UserID uuid.UUID
	Role   domain.Role
}

type contextKey string

const authKey contextKey = "auth"

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/_info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	r.Post("/dummyLogin", h.dummyLogin)
	r.Post("/register", h.register)
	r.Post("/login", h.login)

	r.Group(func(pr chi.Router) {
		pr.Use(h.authMiddleware)

		pr.Get("/rooms/list", h.listRooms)
		pr.Post("/rooms/create", h.createRoom)
		pr.Post("/rooms/{roomId}/schedule/create", h.createSchedule)
		pr.Get("/rooms/{roomId}/slots/list", h.getSlots)
		pr.Post("/bookings/create", h.createBooking)
		pr.Get("/bookings/list", h.allBookings)

		pr.Get("/bookings/my", h.myBookings)
		pr.Post("/bookings/{bookingId}/cancel", h.cancelBooking)

		// Backward-compatible aliases.
		pr.Get("/rooms", h.listRooms)
		pr.Post("/rooms", h.createRoom)
		pr.Post("/rooms/{roomId}/schedule", h.createSchedule)
		pr.Get("/rooms/{roomId}/slots", h.getSlots)
		pr.Post("/bookings", h.createBooking)
		pr.Get("/bookings", h.allBookings)
	})

	return r
}

// dummyLogin godoc
// @Summary Dummy login
// @Description Returns JWT for fixed admin/user identities.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body object true "role payload"
// @Success 200 {object} map[string]string
// @Router /dummyLogin [post]
func (h *Handler) dummyLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role domain.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid json")
		return
	}

	var userID uuid.UUID
	switch req.Role {
	case domain.RoleAdmin:
		userID = adminUserID
	case domain.RoleUser:
		userID = userUserID
	default:
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "role must be admin or user")
		return
	}

	signed, err := h.issueToken(userID, req.Role)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": signed})
}

// register godoc
// @Summary Register user
// @Description Registers user by email/password and returns user payload.
// @Tags Auth
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Router /register [post]
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string      `json:"email"`
		Password string      `json:"password"`
		Role     domain.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid json")
		return
	}
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "email and password are required")
		return
	}
	if req.Role != domain.RoleAdmin && req.Role != domain.RoleUser {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "role must be admin or user")
		return
	}

	user, err := h.service.Register(booking.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
		Role:     req.Role,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) || errors.Is(err, domain.ErrEmailAlreadyExists) {
			writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		h.writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user": map[string]any{
			"id":        user.ID,
			"email":     user.Email,
			"role":      user.Role,
			"createdAt": user.Registered,
		},
	})
}

// login godoc
// @Summary Login user
// @Description Authorizes by email/password and returns JWT.
// @Tags Auth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /login [post]
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid json")
		return
	}
	user, err := h.service.Login(booking.LoginInput{Email: req.Email, Password: req.Password})
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	token, err := h.issueToken(user.ID, user.Role)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

func (h *Handler) issueToken(userID uuid.UUID, role domain.Role) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID.String(),
		"role":    string(role),
		"exp":     h.now().Add(h.tokenTTL).Unix(),
	})
	return token.SignedString(h.jwtSecret)
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
			return
		}
		parsed, err := jwt.Parse(parts[1], func(token *jwt.Token) (any, error) {
			return h.jwtSecret, nil
		})
		if err != nil || !parsed.Valid {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
			return
		}
		claims, ok := parsed.Claims.(jwt.MapClaims)
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid claims")
			return
		}
		userIDRaw, ok := claims["user_id"].(string)
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing user_id")
			return
		}
		userID, err := uuid.Parse(userIDRaw)
		if err != nil {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid user_id")
			return
		}
		roleRaw, ok := claims["role"].(string)
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing role")
			return
		}
		ctx := context.WithValue(r.Context(), authKey, authContext{UserID: userID, Role: domain.Role(roleRaw)})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authFromRequest(r *http.Request) authContext {
	v, _ := r.Context().Value(authKey).(authContext)
	return v
}

// createRoom godoc
// @Summary Create room
// @Tags Rooms
// @Security BearerAuth
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Router /rooms/create [post]
func (h *Handler) createRoom(w http.ResponseWriter, r *http.Request) {
	auth := authFromRequest(r)
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Capacity    int    `json:"capacity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid json")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "name is required")
		return
	}
	room, err := h.service.CreateRoom(booking.CreateRoomInput{Role: auth.Role, Name: req.Name, Description: req.Description, Capacity: req.Capacity})
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"room": room})
}

// listRooms godoc
// @Summary List rooms
// @Tags Rooms
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /rooms/list [get]
func (h *Handler) listRooms(w http.ResponseWriter, _ *http.Request) {
	rooms, err := h.service.ListRooms()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": view.MapRooms(rooms)})
}

// createSchedule godoc
// @Summary Create room schedule
// @Tags Schedules
// @Security BearerAuth
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Router /rooms/{roomId}/schedule/create [post]
func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	auth := authFromRequest(r)
	roomID, err := uuid.Parse(chi.URLParam(r, "roomId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid roomId")
		return
	}

	var req struct {
		RoomID     *uuid.UUID `json:"roomId,omitempty"`
		DaysOfWeek []int      `json:"daysOfWeek"`
		StartTime  string     `json:"startTime"`
		EndTime    string     `json:"endTime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid json")
		return
	}
	if req.RoomID == nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "roomId is required")
		return
	}
	if *req.RoomID != roomID {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "roomId in body must match path roomId")
		return
	}

	weekdays, err := weekdaysFromISO(req.DaysOfWeek)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	startMinutes, err := parseHHMM(req.StartTime)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid startTime")
		return
	}
	endMinutes, err := parseHHMM(req.EndTime)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid endTime")
		return
	}

	schedule, err := h.service.CreateSchedule(booking.CreateScheduleInput{
		Role: auth.Role, RoomID: roomID, Weekdays: weekdays, StartMinutes: startMinutes, EndMinutes: endMinutes,
	})
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"schedule": mapSchedule(schedule)})
}

// getSlots godoc
// @Summary Get available slots by room and date
// @Tags Slots
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /rooms/{roomId}/slots/list [get]
func (h *Handler) getSlots(w http.ResponseWriter, r *http.Request) {
	auth := authFromRequest(r)
	roomID, err := uuid.Parse(chi.URLParam(r, "roomId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid roomId")
		return
	}
	dateRaw := r.URL.Query().Get("date")
	if dateRaw == "" {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "date is required")
		return
	}
	date, err := time.ParseInLocation("2006-01-02", dateRaw, time.UTC)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid date format")
		return
	}
	slots, err := h.service.GetAvailableSlots(auth.Role, roomID, date)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(slots))
	for _, item := range slots {
		if !item.Available {
			continue
		}
		resp = append(resp, map[string]any{
			"id":     item.Slot.ID,
			"roomId": item.Slot.RoomID,
			"start":  item.Slot.StartAt,
			"end":    item.Slot.EndAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"slots": resp})
}

// createBooking godoc
// @Summary Create booking
// @Tags Bookings
// @Security BearerAuth
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Router /bookings/create [post]
func (h *Handler) createBooking(w http.ResponseWriter, r *http.Request) {
	auth := authFromRequest(r)
	var req struct {
		SlotID               uuid.UUID `json:"slotId"`
		CreateConferenceLink bool      `json:"createConferenceLink"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid json")
		return
	}
	if req.SlotID == uuid.Nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "slotId is required")
		return
	}

	bookingEntity, err := h.service.CreateBooking(booking.CreateBookingInput{
		Role:                 auth.Role,
		UserID:               auth.UserID,
		SlotID:               req.SlotID,
		CreateConferenceLink: req.CreateConferenceLink,
	})
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"booking": mapBooking(bookingEntity)})
}

// cancelBooking godoc
// @Summary Cancel booking
// @Tags Bookings
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /bookings/{bookingId}/cancel [post]
func (h *Handler) cancelBooking(w http.ResponseWriter, r *http.Request) {
	auth := authFromRequest(r)
	bookingID, err := uuid.Parse(chi.URLParam(r, "bookingId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid bookingId")
		return
	}
	bookingEntity, err := h.service.CancelBooking(auth.Role, auth.UserID, bookingID)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"booking": mapBooking(bookingEntity)})
}

// myBookings godoc
// @Summary List my future bookings
// @Tags Bookings
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /bookings/my [get]
func (h *Handler) myBookings(w http.ResponseWriter, r *http.Request) {
	auth := authFromRequest(r)
	items, err := h.service.ListMyBookings(auth.Role, auth.UserID)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookings": mapBookings(items)})
}

// allBookings godoc
// @Summary List all bookings (admin)
// @Tags Bookings
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /bookings/list [get]
func (h *Handler) allBookings(w http.ResponseWriter, r *http.Request) {
	auth := authFromRequest(r)

	page, err := parsePositiveQueryInt(r, "page", 1)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid page")
		return
	}
	pageSize, err := parsePositiveQueryInt(r, "pageSize", 20)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid pageSize")
		return
	}

	items, total, err := h.service.ListAllBookings(auth.Role, page, pageSize)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookings": mapBookings(items),
		"pagination": map[string]any{
			"page":     page,
			"pageSize": pageSize,
			"total":    total,
		},
	})
}

func parsePositiveQueryInt(r *http.Request, key string, def int) (int, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return 0, errors.New("invalid")
	}
	return v, nil
}

func mapSchedule(s domain.Schedule) map[string]any {
	return map[string]any{
		"roomId":     s.RoomID,
		"daysOfWeek": weekdaysToISO(s.Weekdays),
		"startTime":  formatHHMM(s.StartMinutes),
		"endTime":    formatHHMM(s.EndMinutes),
	}
}

func mapBooking(b domain.Booking) map[string]any {
	m := map[string]any{
		"id":     b.ID,
		"slotId": b.SlotID,
		"userId": b.UserID,
		"status": b.Status,
	}
	if b.ConferenceLink != nil {
		m["conferenceLink"] = *b.ConferenceLink
	}
	if !b.CreatedAt.IsZero() {
		m["createdAt"] = b.CreatedAt
	}
	return m
}

func mapBookings(items []booking.BookingWithSlot) []map[string]any {
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, mapBooking(item.Booking))
	}
	return resp
}

func weekdaysFromISO(days []int) ([]time.Weekday, error) {
	if len(days) == 0 {
		return nil, errors.New("daysOfWeek is required")
	}
	seen := map[int]struct{}{}
	weekdays := make([]time.Weekday, 0, len(days))
	for _, d := range days {
		if d < 1 || d > 7 {
			return nil, errors.New("daysOfWeek must contain values from 1 to 7")
		}
		if _, ok := seen[d]; ok {
			return nil, errors.New("daysOfWeek must not contain duplicates")
		}
		seen[d] = struct{}{}
		switch d {
		case 1:
			weekdays = append(weekdays, time.Monday)
		case 2:
			weekdays = append(weekdays, time.Tuesday)
		case 3:
			weekdays = append(weekdays, time.Wednesday)
		case 4:
			weekdays = append(weekdays, time.Thursday)
		case 5:
			weekdays = append(weekdays, time.Friday)
		case 6:
			weekdays = append(weekdays, time.Saturday)
		case 7:
			weekdays = append(weekdays, time.Sunday)
		}
	}
	return weekdays, nil
}

func weekdaysToISO(days []time.Weekday) []int {
	resp := make([]int, 0, len(days))
	for _, d := range days {
		switch d {
		case time.Monday:
			resp = append(resp, 1)
		case time.Tuesday:
			resp = append(resp, 2)
		case time.Wednesday:
			resp = append(resp, 3)
		case time.Thursday:
			resp = append(resp, 4)
		case time.Friday:
			resp = append(resp, 5)
		case time.Saturday:
			resp = append(resp, 6)
		case time.Sunday:
			resp = append(resp, 7)
		}
	}
	return resp
}

func parseHHMM(value string) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, errors.New("invalid hh:mm")
	}
	if len(parts[0]) < 1 || len(parts[0]) > 2 || len(parts[1]) != 2 {
		return 0, errors.New("invalid hh:mm")
	}
	hh, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	mm, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, errors.New("invalid hh:mm")
	}
	return hh*60 + mm, nil
}

func formatHHMM(minutes int) string {
	h := minutes / 60
	m := minutes % 60
	return strconv.FormatInt(int64(h/10), 10) + strconv.FormatInt(int64(h%10), 10) + ":" + strconv.FormatInt(int64(m/10), 10) + strconv.FormatInt(int64(m%10), 10)
}

func (h *Handler) writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrInvalidCredentials):
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		writeAPIError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
	case errors.Is(err, domain.ErrRoomNotFound):
		writeAPIError(w, http.StatusNotFound, "ROOM_NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrSlotNotFound):
		writeAPIError(w, http.StatusNotFound, "SLOT_NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrBookingNotFound):
		writeAPIError(w, http.StatusNotFound, "BOOKING_NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrScheduleAlreadyExists):
		writeAPIError(w, http.StatusConflict, "SCHEDULE_EXISTS", err.Error())
	case errors.Is(err, domain.ErrSlotAlreadyBooked):
		writeAPIError(w, http.StatusConflict, "SLOT_ALREADY_BOOKED", err.Error())
	case errors.Is(err, domain.ErrCannotBookPastSlot), errors.Is(err, domain.ErrInvalidSchedule), errors.Is(err, domain.ErrEmailAlreadyExists), errors.Is(err, domain.ErrInvalidPagination):
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
