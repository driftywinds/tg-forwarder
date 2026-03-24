package main

import (
	"sync"
	"time"
)

// session holds the pending messages for one admin mid-recording.
type session struct {
	messages      []BundleMessage
	debounceTimer *time.Timer
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

// AppendAndDebounce appends msg to the session, resets the debounce timer to
// delay, and returns false if no session is active.
//
// The first file in a bulk forward starts a 1-second countdown. Every
// subsequent file that arrives before the countdown expires cancels it and
// starts a fresh one. Only when a full second passes with no new file does
// onFlush fire — receiving a sorted snapshot of everything collected so far.
//
// This means 20 files forwarded in rapid succession produce exactly one
// feedback message to the admin, not 20.
//
// onFlush is called from a background goroutine; it must not hold s.mu.
func (s *SessionStore) AppendAndDebounce(
	adminID int64,
	msg BundleMessage,
	delay time.Duration,
	onFlush func(msgs []BundleMessage),
) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[adminID]
	if !ok {
		return false
	}

	sess.messages = append(sess.messages, msg)

	// Cancel any in-flight timer before starting a new one.
	if sess.debounceTimer != nil {
		sess.debounceTimer.Stop()
	}

	sess.debounceTimer = time.AfterFunc(delay, func() {
		// Take a sorted snapshot under the lock, then call onFlush outside it.
		s.mu.Lock()
		sess2, exists := s.sessions[adminID]
		if !exists {
			s.mu.Unlock()
			return
		}
		snapshot := make([]BundleMessage, len(sess2.messages))
		copy(snapshot, sess2.messages)
		sess2.debounceTimer = nil
		s.mu.Unlock()

		sortBundleMessages(snapshot)
		onFlush(snapshot)
	})

	return true
}

// End finalises and returns the collected messages, then clears the session.
// Any pending debounce timer is stopped so it cannot fire after /done.
// Returns nil if no session was active.
func (s *SessionStore) End(adminID int64) []BundleMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[adminID]
	if !ok {
		return nil
	}
	if sess.debounceTimer != nil {
		sess.debounceTimer.Stop()
	}
	msgs := sess.messages
	delete(s.sessions, adminID)
	return msgs
}

// Cancel discards the session without saving.
// Any pending debounce timer is stopped.
func (s *SessionStore) Cancel(adminID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[adminID]
	if !ok {
		return false
	}
	if sess.debounceTimer != nil {
		sess.debounceTimer.Stop()
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