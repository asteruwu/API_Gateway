package health

import "time"

type State uint8

const (
	StateHealthy State = iota
	StateProbing
	StateDead
)

type instanceStatus struct {
	addr      string
	state     State
	updatedAt time.Time
}
