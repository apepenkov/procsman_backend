package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

/*
#include <stdlib.h>
void allocateAndFreeAfter(size_t size, int seconds);
*/
import "C"

func randomActions() {
	for {
		time.Sleep(time.Duration(rand.Intn(20)) * time.Second)

		// Randomly choose an action (ram, cpu)
		action := rand.Intn(3)
		switch action {
		case 0:
			// Load RAM (1-10GB, 5-30 seconds)
			amount := rand.Intn(5) + 1
			seconds := rand.Intn(25) + 5
			fmt.Printf("\033[32mLoading RAM: %dGB for %d seconds\033[0m\n", amount, seconds)
			loadRAM(amount*1024*1024*1024, seconds)
			time.Sleep(time.Duration(seconds+2) * time.Second)
		case 1:
			// Load CPU (1-10M, 1-5 threads)
			amount := rand.Intn(100000000) + 10000000
			threads := rand.Intn(5) + 1
			fmt.Printf("\033[31mLoading CPU: %d hashes for %d threads\033[0m\n", amount, threads)
			loadCPU(amount, threads)
		case 2:
			ramAmount := rand.Intn(5) + 1
			ramSeconds := rand.Intn(25) + 5
			cpuAmount := rand.Intn(100000000) + 10000000
			cpuThreads := rand.Intn(2) + 1

			fmt.Printf("Loading RAM: %dGB for %d seconds and CPU: %d hashes for %d threads\n", ramAmount, ramSeconds, cpuAmount, cpuThreads)
			go loadRAM(ramAmount*1024*1024*1024, ramSeconds)
			loadCPU(cpuAmount, cpuThreads)
		}
	}
}

func main() {

	go func() {
		for {
			var input string
			_, err := fmt.Scanln(&input)
			if err != nil {
				//time.Sleep(1 * time.Second)
				fmt.Println("Error reading from stdin:", err)
				continue
			}
			fmt.Println("Read from stdin:", input)
		}
	}()
	//go randomActions()
	var doRandomActions bool
	flag.BoolVar(&doRandomActions, "random", false, "Enable random actions")
	flag.Parse()
	if doRandomActions {
		fmt.Println("Random actions enabled")
		go randomActions()
	} else {
		fmt.Println("Random actions disabled")
	}

	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Stopping server")
		os.Exit(0)
	})

	http.HandleFunc("/crash", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Crashing server")
		os.Exit(1)
	})

	http.HandleFunc("/stdin", func(w http.ResponseWriter, r *http.Request) {
		// This example doesn't implement a stdin reader for brevity.
		fmt.Fprintln(w, "stdin functionality not implemented")
	})

	http.HandleFunc("/stdout", func(w http.ResponseWriter, r *http.Request) {
		incomingBody, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Fprintln(w, "Error reading request body")
			return
		}
		fmt.Println(string(incomingBody))
		log.Println(string(incomingBody))
		fmt.Fprintln(w, "Request body printed to stdout")
	})

	http.HandleFunc("/stderr", func(w http.ResponseWriter, r *http.Request) {
		incomingBody, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Fprintln(w, "Error reading request body")
			return
		}
		fmt.Fprintln(os.Stderr, string(incomingBody))
		fmt.Fprintln(w, "Request body printed to stderr")
	})

	http.HandleFunc("/load_cpu", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		n, err := strconv.Atoi(query.Get("n"))
		if err != nil || n <= 0 {
			n = 1000000 // default
		}
		threads, err := strconv.Atoi(query.Get("threads"))
		if err != nil || threads <= 0 {
			threads = 1 // default
		}

		loadCPU(n, threads)

		fmt.Fprintln(w, "CPU loaded")
	})

	http.HandleFunc("/load_ram", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		nBytes, err := strconv.Atoi(query.Get("n_bytes"))
		if err != nil || nBytes <= 0 {
			nBytes = 1024 * 1024 * 1024 // 1GB default
		}
		seconds, err := strconv.Atoi(query.Get("seconds"))
		if err != nil || seconds <= 0 {
			seconds = 10 // default
		}

		go loadRAM(nBytes, seconds)

		fmt.Fprintln(w, "RAM loaded")
	})

	port := "15432"
	fmt.Println("[FMT] Starting server on port", port)
	log.Printf("Starting server on port %s\n", port)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, nil))
}

func loadCPU(n, threads int) {
	var wg sync.WaitGroup
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hashes(n)
		}()
	}
	wg.Wait()
}

func hashes(n int) string {
	curr := []byte("0")
	var currTmp [32]byte
	for i := 0; i < n; i++ {
		currTmp = sha256.Sum256(curr)
		curr = currTmp[:]
	}
	return fmt.Sprintf("%x", curr)
}

func _loadRAM(nBytes, seconds int) {
	data := make([]byte, nBytes)
	fmt.Println("Allocated", len(data), "bytes")
	rand.Read(data)
	time.Sleep(time.Duration(seconds) * time.Second)
	if len(data) == -1 {
		// needed for data reference to not be optimized away
		fmt.Println("This line will never be printed")
	}
	// deallocate data
	data = nil
	runtime.GC()
	fmt.Println("Deallocated")
}

func loadRAM(nBytes, seconds int) {
	C.allocateAndFreeAfter(C.size_t(nBytes), C.int(seconds))
}
