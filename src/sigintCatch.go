package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var shutdownHandlers = make(map[string]func(os.Signal))

var shutdownLock = new(sync.Mutex)

func delShutdownHandler(name string) {
	shutdownLock.Lock()
	delete(shutdownHandlers, name)
	shutdownLock.Unlock()
}

func addShutdownHandler(name string, f func(os.Signal)) {
	shutdownLock.Lock()
	shutdownHandlers[name] = f
	shutdownLock.Unlock()
}

func init() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go handleShutdown(c)
}

func handleShutdown(c chan os.Signal) {
	shutdownWait := new(sync.WaitGroup)
	for sig := range c {
		log.Print("Interrupted: Shutting Down")
		shutdownLock.Lock()
		for _, h := range shutdownHandlers {
			shutdownWait.Add(1)
			go func(hh func(os.Signal)) {
				hh(sig)
				shutdownWait.Done()
			}(h)
		}
		shutdownWait.Wait()
		log.Print("Exiting")
		syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}
}
