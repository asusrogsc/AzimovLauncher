//go:build !windows

// Permet de compiler et tester le cœur hors Windows (go test).
package main

func diskFree(dir string) (uint64, bool) { return 0, false }

func main() {}
