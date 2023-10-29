package main

import (
	"log"
	"time"
	"time_wheel/wheel"
)

func main() {

	w := wheel.NewTimeWheel(8, time.Second)
	log.Printf("wheel: %+v\n", w)
	go w.AddTask("test", time.Now().Add(3*time.Second), func() {
		println("hello world")
	})

	time.Sleep(10 * time.Second)
}
