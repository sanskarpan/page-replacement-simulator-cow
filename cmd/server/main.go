package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/pkg/api"
	"github.com/page-replacement-cow/web"
)

func main() {
	port := flag.Int("port", 8080, "Server port")
	numFrames := flag.Int("frames", 128, "Number of physical memory frames")
	tlbSize := flag.Int("tlb", 16, "TLB size")
	algo := flag.String("algorithm", "LRU", "Page replacement algorithm (LRU, CLOCK, LFU, FIFO, Optimal, Random)")
	corsDomains := flag.String("cors", "localhost", "Allowed CORS domains (comma-separated, use * for all)")
	flag.Parse()

	var algType algorithms.AlgorithmType
	switch *algo {
	case "LRU":
		algType = algorithms.AlgorithmLRU
	case "CLOCK":
		algType = algorithms.AlgorithmCLOCK
	case "LFU":
		algType = algorithms.AlgorithmLFU
	case "FIFO":
		algType = algorithms.AlgorithmFIFO
	case "Optimal":
		algType = algorithms.AlgorithmOptimal
	case "Random":
		algType = algorithms.AlgorithmRandom
	default:
		log.Fatalf("Invalid algorithm: %s", *algo)
	}

	server := api.NewServer(int32(*numFrames), *tlbSize, algType)
	router := api.SetupRoutes(server)

	staticHandler, err := web.FileServer()
	if err != nil {
		log.Fatalf("Failed to load embedded static files: %v", err)
	}
	router.PathPrefix("/").Handler(staticHandler)

	corsMiddleware := buildCORSMiddleware(strings.Split(*corsDomains, ","))

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: corsMiddleware(router),
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("\nShutting down server...")
		server.Shutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Printf("Starting Page Replacement Simulator + CoW Server")
	log.Printf("  Port: %d", *port)
	log.Printf("  Frames: %d", *numFrames)
	log.Printf("  TLB Size: %d", *tlbSize)
	log.Printf("  Algorithm: %s", *algo)
	log.Printf("  CORS domains: %s", *corsDomains)
	log.Printf("\nServer running at http://localhost:%d", *port)
	log.Printf("Open http://localhost:%d in your browser to access the UI\n", *port)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

func buildCORSMiddleware(allowedDomains []string) func(http.Handler) http.Handler {
	allowAll := false
	allowedSet := make(map[string]bool)
	for _, d := range allowedDomains {
		d = strings.TrimSpace(d)
		if d == "*" {
			allowAll = true
		} else {
			allowedSet[d] = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				if allowAll || allowedSet[extractHost(origin)] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
				if origin != "" && !allowAll && !allowedSet[extractHost(origin)] {
					writeError(w, http.StatusForbidden, "cross-origin request denied")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractHost(origin string) string {
	origin = strings.TrimPrefix(origin, "http://")
	origin = strings.TrimPrefix(origin, "https://")
	if idx := strings.IndexByte(origin, ':'); idx >= 0 {
		origin = origin[:idx]
	}
	return origin
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":"%s"}`, message)
}