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

// AppendAndRank appends msg to the session and atomically returns the 1-based
// sorted position it will occupy and the new total count. Doing both under one
// lock prevents a race where concurrent bulk-forward goroutines interleave an
// Append and a separate rank query, which would produce a stale total.
//
// The second return value (ok) is false when no session is active.
func (s *SessionStore) AppendAndRank(adminID int64, msg BundleMessage) (rank, total int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, exists := s.sessions[adminID]
	if !exists {
		return 0, 0, false
	}

	sess.messages = append(sess.messages, msg)

	// Sort a shallow copy to find the sorted rank without mutating session order.
	// For typical bundle sizes (≤ a few hundred files) this is instantaneous.
	tmp := make([]BundleMessage, len(sess.messages))
	copy(tmp, sess.messages)
	sortBundleMessages(tmp)

	total = len(tmp)
	for i, m := range tmp {
		if m.FileName == msg.FileName && m.ChatID == msg.ChatID && m.MessageID == msg.MessageID {
			return i + 1, total, true
		}
	}
	// Fallback: should never be reached, but be safe.
	return total, total, true
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