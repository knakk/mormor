package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// mormorMain represents the main program execution.
type mormorMain struct{}

func (m *mormorMain) Run() {

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	log.Println("Mormor is up and running.")

	<-shutdown

	log.Println("Mormor is shutting down.")

	m.cleanup()
}

func (m *mormorMain) cleanup() {
	// TODO perform any cleanup operations here
}

func main() {

	m := &mormorMain{}
	m.Run()

}
