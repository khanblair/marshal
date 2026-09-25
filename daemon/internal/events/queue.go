package events

// initialQueueSize is where a queue starts. It grows by doubling, so a quiet subscriber costs
// almost nothing and a busy one still never reallocates on every event.
const initialQueueSize = 8

// queueGrowth is the factor a full queue grows by.
const queueGrowth = 2

// queue is a first-in first-out list of events that grows as needed. It is not safe for
// concurrent use: the owner holds a lock and enforces the bound before it pushes.
type queue struct {
	buf  []Event
	head int
	size int
}

func (q *queue) len() int { return q.size }

func (q *queue) push(ev Event) {
	if q.size == len(q.buf) {
		q.grow()
	}
	q.buf[(q.head+q.size)%len(q.buf)] = ev
	q.size++
}

func (q *queue) grow() {
	next := make([]Event, max(initialQueueSize, queueGrowth*len(q.buf)))
	for i := range q.size {
		next[i] = q.at(i)
	}
	q.buf, q.head = next, 0
}

// peek returns the oldest event without removing it.
func (q *queue) peek() (Event, bool) {
	if q.size == 0 {
		return Event{}, false
	}
	return q.buf[q.head], true
}

// pop removes and returns the oldest event.
func (q *queue) pop() (Event, bool) {
	ev, ok := q.peek()
	if !ok {
		return Event{}, false
	}
	// The slot is cleared so the queue does not keep the payload alive.
	q.buf[q.head] = Event{}
	q.head = (q.head + 1) % len(q.buf)
	q.size--
	return ev, true
}

// at returns the i-th oldest event. i must be below len.
func (q *queue) at(i int) Event {
	return q.buf[(q.head+i)%len(q.buf)]
}

func (q *queue) clear() {
	*q = queue{}
}
