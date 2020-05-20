package main

import "time"

func timeOperation(f func() error) (time.Duration, error) {
	start := time.Now()
	err := f()
	return time.Since(start), err
}
