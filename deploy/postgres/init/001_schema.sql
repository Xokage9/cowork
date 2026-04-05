-- PostgreSQL schema for runtime repositories.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS rooms (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    capacity INTEGER NOT NULL CHECK (capacity > 0),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    registered_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS schedules (
    room_id UUID PRIMARY KEY REFERENCES rooms(id) ON DELETE CASCADE,
    weekdays SMALLINT[] NOT NULL,
    start_minutes INTEGER NOT NULL,
    end_minutes INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (start_minutes >= 0),
    CHECK (end_minutes <= 1440),
    CHECK (end_minutes > start_minutes),
    CHECK ((end_minutes - start_minutes) % 30 = 0)
);

CREATE TABLE IF NOT EXISTS slots (
    id UUID PRIMARY KEY,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_at > start_at),
    UNIQUE (room_id, start_at, end_at)
);

CREATE TABLE IF NOT EXISTS bookings (
    id UUID PRIMARY KEY,
    slot_id UUID NOT NULL REFERENCES slots(id) ON DELETE RESTRICT,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('active', 'cancelled')),
    conference_link TEXT NULL,
    conference_status TEXT NOT NULL DEFAULT 'not_requested',
    created_at TIMESTAMPTZ NOT NULL,
    cancelled_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_active_booking_per_slot
    ON bookings (slot_id)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_slots_room_date
    ON slots (room_id, start_at);

CREATE INDEX IF NOT EXISTS idx_bookings_user
    ON bookings (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_users_email
    ON users (email);
