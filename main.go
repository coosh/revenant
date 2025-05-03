package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os/exec"
	"sync"
	"time"
)

var (
	listenAddr   = flag.String("listen", ":8080", "proxy listen address")
	targetAddr   = flag.String("target", "127.0.0.1:7860", "target address")
	startCmd     = flag.String("start-cmd", "", "command to start the service")
	stopCmd      = flag.String("stop-cmd", "", "command to stop the service")
	checkHost    = flag.String("check-url", "", "host:port to check service readiness (defaults to target)")
	idleTimeout  = flag.Duration("idle-timeout", 10*time.Minute, "idle time before shutdown")
	startTimeout = flag.Duration("start-timeout", 30*time.Second, "max time to wait for startup")

	lastAccess time.Time
	mu         sync.Mutex

	starting   bool
	startMutex sync.Mutex
)

func init() {
	flag.Usage = func() {
		log.Println("Usage of lazy-tcp-proxy:")
		flag.PrintDefaults()
	}
}

func isRunning() bool {
	addr := *checkHost
	if addr == "" {
		addr = *targetAddr
	}
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func startService() {
	log.Println("Starting service...")
	cmd := exec.Command("bash", "-c", *startCmd)
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start service: %v", err)
		return
	}
	go func() {
		_ = cmd.Wait()
	}()
}

func stopService() {
	if *stopCmd != "" {
		log.Println("Stopping service...")
		_ = exec.Command("bash", "-c", *stopCmd).Run()
	}
}

func waitForReady() {
	deadline := time.Now().Add(*startTimeout)
	for time.Now().Before(deadline) {
		if isRunning() {
			log.Println("Service is now reachable.")
			return
		}
		log.Println("Waiting for service to become ready...")
		time.Sleep(1 * time.Second)
	}
	log.Println("Warning: service may not have started in time.")
}

func idleMonitor() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		mu.Lock()
		idle := time.Since(lastAccess) > *idleTimeout
		mu.Unlock()
		if idle && isRunning() {
			log.Println("Idle timeout reached. Shutting down service...")
			stopService()
		}
	}
}

func handleTCPProxy(client net.Conn) {
	defer client.Close()
	target, err := net.Dial("tcp", *targetAddr)
	if err != nil {
		log.Printf("Failed to connect to target: %v", err)
		return
	}
	defer target.Close()

	go io.Copy(target, client)
	io.Copy(client, target)
}

func ensureStarted() {
	startMutex.Lock()
	if !starting {
		starting = true
		startMutex.Unlock()
		startService()
		waitForReady()
		startMutex.Lock()
		starting = false
	}
	startMutex.Unlock()
}

func main() {
	flag.Parse()

	if len(flag.Args()) == 0 &&
		*startCmd == "" &&
		*targetAddr == "127.0.0.1:7860" &&
		*listenAddr == ":8080" {
		log.Println("No options specified.")
		flag.Usage()
		return
	}

	lastAccess = time.Now()
	go idleMonitor()

	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	log.Printf("Lazy TCP proxy listening on %s → %s", *listenAddr, *targetAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Accept failed: %v", err)
			continue
		}

		go func(c net.Conn) {
			defer c.Close()

			mu.Lock()
			lastAccess = time.Now()
			mu.Unlock()

			startMutex.Lock()
			isStarting := starting
			startMutex.Unlock()
			if isStarting {
				log.Printf("Query queued while awaiting app to start")
			}

			if !isRunning() && *startCmd != "" {
				ensureStarted()
				if !isRunning() {
					log.Printf("Service still unavailable after startup. Dropping connection.")
					return
				}
			}

			handleTCPProxy(c)
		}(conn)
	}
}

