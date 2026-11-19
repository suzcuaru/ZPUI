package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/windows/svc"

	"zpui/internal/zsvc"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	console := flag.Bool("console", false, "run without SCM (console mode, Ctrl+C to stop)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	srv := zsvc.New(version)

	if *console {
		runConsole(srv)
		return
	}

	if err := svc.Run(zsvc.ServiceName, srv); err != nil {
		fmt.Fprintf(os.Stderr, "svc.Run: %v\n", err)
		fmt.Fprintln(os.Stderr, "подсказка: запуск вне SCM — используйте -console")
		os.Exit(1)
	}
}

func runConsole(s *zsvc.Service) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()
	s.StartWorkers(ctx)
	fmt.Printf("ZPUI Service v%s (console mode)\npipes: %s\nCtrl+C — stop\n", version, zsvc.PipeName)
	<-ctx.Done()
	s.StopWorkers()
}
