package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/pkg/api"
	"github.com/page-replacement-cow/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	port := flag.Int("port", 8080, "Server port")
	numFrames := flag.Int("frames", 128, "Number of physical memory frames")
	tlbSize := flag.Int("tlb", 16, "TLB size")
	algo := flag.String("algorithm", "LRU", "Page replacement algorithm (LRU, CLOCK, LFU, FIFO, Optimal, Random, ARC, CAR, WSClock, PFF, OPT+, NRU)")
	corsDomains := flag.String("cors", "localhost", "Allowed CORS domains (comma-separated, use * for all)")
	flag.Parse()

	if *numFrames < 4 || *numFrames > 1<<20 {
		log.Fatalf("--frames must be between 4 and 1048576, got %d", *numFrames)
	}
	if *tlbSize < 1 || *tlbSize > 65536 {
		log.Fatalf("--tlb must be between 1 and 65536, got %d", *tlbSize)
	}

	algType, ok := algorithms.ParseAlgorithmType(*algo)
	if !ok {
		log.Fatalf("Invalid algorithm: %s. Valid values: %s",
			*algo, strings.Join(algorithms.ValidAlgorithmNames, ", "))
	}

	allowedDomains := strings.Split(*corsDomains, ",")
	server := api.NewServer(int32(*numFrames), *tlbSize, algType)
	server.SetAllowedOrigins(allowedDomains)

	router := api.SetupRoutes(server)

	staticHandler, err := web.FileServer()
	if err != nil {
		log.Fatalf("Failed to load embedded static files: %v", err)
	}
	router.PathPrefix("/").Handler(staticHandler)

	chain := panicRecoveryMiddleware(
		buildCORSMiddleware(allowedDomains)(
			requestBodyLimitMiddleware(router),
		),
	)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           chain,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		slog.Info("shutting down server")
		server.Shutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("page replacement simulator started",
		"port", *port,
		"frames", *numFrames,
		"tlb", *tlbSize,
		"algorithm", *algo,
		"cors", *corsDomains,
	)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

// panicRecoveryMiddleware catches any panic in a handler, logs the stack trace,
// and returns a 500 to the client without crashing the process.
func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"recovery", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// requestBodyLimitMiddleware caps incoming request bodies to prevent OOM from crafted payloads.
func requestBodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, api.MaxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
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
					writeJSONError(w, http.StatusForbidden, "cross-origin request denied")
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

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}