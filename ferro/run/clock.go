package run

import "time"

type Clock interface {
	Now() time.Time
}

type CurrentTime struct{}

func (t *CurrentTime) Now() time.Time {
	return time.Now().UTC()
}
