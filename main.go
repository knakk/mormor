package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

type service interface {
	String() string
	Start() error
	Stop() error
}

// mormorMain represents the main program execution.
type mormorMain struct {
	services []service
}

func (m *mormorMain) Run() {

	errors := make(chan error)
	interrupt := make(chan os.Signal, 1)

	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	log.Println("Mormor is starting.")

	// Start all services.
	for _, s := range m.services {
		go func(s service) {
			if err := s.Start(); err != nil {
				log.Printf("error starting %v service: %v", s, err)
				errors <- err
			}
		}(s)
	}

	select {
	case <-interrupt:
		break
	case <-errors:
		break
	}

	log.Println("Mormor is shutting down.")

	// Shutdown all services.
	for _, s := range m.services {
		if err := s.Stop(); err != nil {
			log.Printf("error stopping %v service: %v", s, err)
		}
	}

}

func newMormorMain(services ...service) *mormorMain {
	m := mormorMain{
		services: services,
	}
	return &m
}

func main() {

	var (
		metadataAddr = flag.String("metadata-addr", ":7001", "metadata service listening address")
		metadataDB   = flag.String("metadata-db", "metadata.db", "metadata database")
		metadataNS   = flag.String("metadata-ns", "", "RDF namespace (resource base URI)")
	)

	flag.Parse()

	m := newMormorMain(
		newMetadataService(*metadataAddr, *metadataDB, *metadataNS),
	)
	m.Run()
}
