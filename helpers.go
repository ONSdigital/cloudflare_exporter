package main

import "time"

func timeOperation(f func() error) (float64, error) {
	start := time.Now()
	err := f()
	return float64(time.Since(start)) / float64(time.Second), err
}

func keys(dict map[string]string) []string {
	var keys []string
	for key := range dict {
		keys = append(keys, key)
	}
	return keys
}
