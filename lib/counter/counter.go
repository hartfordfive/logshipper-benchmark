package counter

type Counter struct {
	accumulator chan int64
	count       int64
	terminate   chan bool
	reset       chan bool
}

func NewCounter() *Counter {
	cnt := &Counter{
		accumulator: make(chan int64, 1000000),
		terminate:   make(chan bool, 1),
		reset:       make(chan bool, 1),
	}
	go cnt.start()
	return cnt
}

func (c *Counter) start() {
	for {
		select {
		case <-c.reset:
			c.count = 0
		case <-c.terminate:
			close(c.accumulator)
			close(c.reset)
			close(c.terminate)
			return
		case incr := <-c.accumulator:
			c.count += int64(incr)
		}
	}
}

func (c *Counter) Incr(val int64) {
	c.accumulator <- val
}

func (c *Counter) Value() int64 {
	return c.count
}

func (c *Counter) Reset() {
	c.reset <- true
}

func (c *Counter) Cancel() {
	c.terminate <- true
}
