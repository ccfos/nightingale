package semaphore

import ()

type Semaphore struct {
	bufSize int
	channel chan int8
}

func NewSemaphore(concurrencyNum int) *Semaphore {
	return &Semaphore{channel: make(chan int8, concurrencyNum), bufSize: concurrencyNum}
}

func (this *Semaphore) TryAcquire() bool {
	select {
	case this.channel <- int8(0):
		return true
	default:
		return false
	}
}

func (this *Semaphore) Acquire() {
	this.channel <- int8(0)
}

func (this *Semaphore) Release() {
	<-this.channel
}

func (this *Semaphore) AvailablePermits() int {
	return this.bufSize - len(this.channel)
}
