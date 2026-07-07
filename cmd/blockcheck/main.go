package main

import (
	"fmt"
	"os"

	"zpui/internal/blockcheck"
)

func main() {
	targets := []blockcheck.BulkTarget{
		{Name: "rutracker.org", URL: "https://rutracker.org"},
		{Name: "google.com", URL: "https://www.google.com"},
		{Name: "youtube.com", URL: "https://www.youtube.com"},
		{Name: "discord.com", URL: "https://discord.com"},
		{Name: "github.com", URL: "https://github.com"},
		{Name: "x.com", URL: "https://x.com"},
		{Name: "wikipedia.org", URL: "https://ru.wikipedia.org"},
		{Name: "sberbank.ru", URL: "https://www.sberbank.ru"},
	}

	proxyAddr := "127.0.0.1:1080"
	checker := blockcheck.NewChecker(8, proxyAddr)
	report := checker.BulkCheck(targets, nil)

	fmt.Println("=== Resource Blocking Report ===")
	fmt.Println()

	blocked := 0
	ok := 0

	for _, r := range report.Default {
		status := "OK"
		if r.Blocked {
			status = fmt.Sprintf("BLOCKED (%s)", r.Verdict)
			blocked++
		} else {
			ok++
		}
		fmt.Printf("[%s] %s — %s\n", status, r.Name, r.URL)
		if r.Reason != "" {
			fmt.Printf("      %s\n", r.Reason)
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total: %d | OK: %d | Blocked: %d\n", len(report.Default), ok, blocked)

	if blocked == 0 {
		fmt.Println("All resources accessible")
	}

	os.Exit(0)
}
