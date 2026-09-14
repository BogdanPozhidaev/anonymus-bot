package repository

import "errors"

var (
	ErrNotFound       = errors.New("record not found")
	ErrNotParticipant = errors.New("user is not a participant of this session")
)
