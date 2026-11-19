//go:build !windows

package main

func defaultWindowSize() (int, int) {
	return 1280, 720
}
