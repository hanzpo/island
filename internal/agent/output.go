package agent

import "sync"

// RingBuffer is a thread-safe circular buffer for storing output lines.
type RingBuffer struct {
	mu    sync.Mutex
	lines []string
	cap   int
	head  int
	count int
}

// NewRingBuffer creates a new RingBuffer with the given capacity.
// If capacity <= 0, defaults to 10000.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 10000
	}
	return &RingBuffer{
		lines: make([]string, capacity),
		cap:   capacity,
	}
}

// Write appends a line to the ring buffer, overwriting the oldest if full.
func (rb *RingBuffer) Write(line string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	idx := (rb.head + rb.count) % rb.cap
	if rb.count == rb.cap {
		// Buffer is full; overwrite oldest and advance head.
		rb.lines[rb.head] = line
		rb.head = (rb.head + 1) % rb.cap
	} else {
		rb.lines[idx] = line
		rb.count++
	}
}

// Lines returns all lines in the buffer in order from oldest to newest.
func (rb *RingBuffer) Lines() []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	result := make([]string, rb.count)
	for i := 0; i < rb.count; i++ {
		result[i] = rb.lines[(rb.head+i)%rb.cap]
	}
	return result
}

// Last returns the last n lines in order. If n > Len(), returns all lines.
func (rb *RingBuffer) Last(n int) []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if n <= 0 {
		return nil
	}
	if n > rb.count {
		n = rb.count
	}

	result := make([]string, n)
	start := rb.count - n
	for i := 0; i < n; i++ {
		result[i] = rb.lines[(rb.head+start+i)%rb.cap]
	}
	return result
}

// Len returns the number of lines currently stored.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}
