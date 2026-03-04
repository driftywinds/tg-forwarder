package main

import "sync"

// session holds the pending messages for one admin mid-recording.
type session struct {
	messages []BundleMessage
}

// SessionStore is a thread-safe map of adminID → active recording session.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[int64]*session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[int64]*session)}
}

func (s *SessionStore) Begin(adminID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, active := s.sessions[adminID]; active {
		return false // already recording
	}
	s.sessions[adminID] = &session{}
	return true
}

func (s *SessionStore) IsRecording(adminID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.sessions[adminID]
	return ok
}

func (s *SessionStore) Append(adminID int64, msg BundleMessage) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[adminID]
	if !ok {
		return false
	}
	sess.messages = append(sess.messages, msg)
	return true
}

// End finalises and returns the collected messages, then clears the session.
// Returns nil if no session was active.
func (s *SessionStore) End(adminID int64) []BundleMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[adminID]
	if !ok {
		return nil
	}
	msgs := sess.messages
	delete(s.sessions, adminID)
	return msgs
}

// Cancel discards the session without saving.
func (s *SessionStore) Cancel(adminID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[adminID]; !ok {
		return false
	}
	delete(s.sessions, adminID)
	return true
}

// Count returns how many messages are buffered in the current session.
func (s *SessionStore) Count(adminID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[adminID]; ok {
		return len(sess.messages)
	}
	return 0
}