package main

import (
	"fmt"
	"os"
	"sort"

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
	bypassed := 0
	var failedBypass []blockcheck.BulkResult

	for _, r := range report.Default {
		status := "✓ OK"
		if r.Blocked {
			status = fmt.Sprintf("✗ BLOCKED (%s)", r.Verdict)
			blocked++
			if r.Bypassed {
				status += " → bypassed"
				bypassed++
			} else {
				failedBypass = append(failedBypass, r)
			}
		}
		fmt.Printf("[%s] %s — %s\n", status, r.Name, r.URL)
		if r.Reason != "" {
			fmt.Printf("      %s\n", r.Reason)
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total: %d | Blocked: %d | Bypassed: %d | Failed bypass: %d\n",
		len(report.Default), blocked, bypassed, blocked-bypassed)

	if len(failedBypass) > 0 {
		fmt.Println("\n⚠ Resources that are blocked and NOT bypassed:")
		sort.Slice(failedBypass, func(i, j int) bool { return failedBypass[i].Name < failedBypass[j].Name })
		for _, r := range failedBypass {
			fmt.Printf("  ✗ %s (%s) — verdict: %s\n", r.Name, r.URL, r.Verdict)
		}
	}

	if blocked == 0 {
		fmt.Println("✓ No blocking detected")
	} else if blocked == bypassed {
		fmt.Println("✓ All blocked resources are bypassed")
	}

	os.Exit(0)
}
