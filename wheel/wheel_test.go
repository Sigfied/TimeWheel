package wheel

import (
	"testing"
	"time"
)

func GetTask() {

}
func TestNewTimeWheel(t *testing.T) {
	wheel := NewTimeWheel(8, time.Second)
	go wheel.AddTask("test", time.Now().Add(3*time.Second), GetTask)

	time.Sleep(10 * time.Second)
}
