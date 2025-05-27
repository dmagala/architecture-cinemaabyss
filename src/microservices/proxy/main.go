package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type Config struct {
	Port                 string
	MonolithURL          string
	MoviesServiceURL     string
	EventsServiceURL     string
	GradualMigration     bool
	MoviesMigrationPercent int
}

var (
	config Config
	monolithProxy *httputil.ReverseProxy
	moviesServiceProxy *httputil.ReverseProxy
	eventsServiceProxy *httputil.ReverseProxy
)

func main() {
	// Load configuration
	loadConfig()

	// Initialize reverse proxies
	initProxies()

	// Set up routes
	router := mux.NewRouter()
	router.HandleFunc("/health", healthHandler).Methods("GET")
	router.HandleFunc("/api/movies", moviesHandler).Methods("GET", "POST", "PUT", "DELETE")
	router.HandleFunc("/api/events/{rest:.*}", eventsHandler).Methods("GET", "POST", "PUT", "DELETE")
	router.HandleFunc("/api/{rest:.*}", defaultHandler).Methods("GET", "POST", "PUT", "DELETE")

	// Start server
	log.Printf("Starting proxy service on port %s", config.Port)
	log.Printf("Migration settings: enabled=%v, percent=%d", config.GradualMigration, config.MoviesMigrationPercent)
	log.Fatal(http.ListenAndServe(":"+config.Port, router))
}

func loadConfig() {
	config.Port = getEnv("PORT", "8000")
	config.MonolithURL = getEnv("MONOLITH_URL", "http://monolith:8080")
	config.MoviesServiceURL = getEnv("MOVIES_SERVICE_URL", "http://movies-service:8081")
	config.EventsServiceURL = getEnv("EVENTS_SERVICE_URL", "http://events-service:8082")
	config.GradualMigration = getEnvBool("GRADUAL_MIGRATION", true)
	
	percent, err := strconv.Atoi(getEnv("MOVIES_MIGRATION_PERCENT", "50"))
	if err != nil {
		log.Printf("Invalid MOVIES_MIGRATION_PERCENT, using default 50: %v", err)
		percent = 50
	}
	config.MoviesMigrationPercent = percent
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return defaultValue
		}
		return b
	}
	return defaultValue
}

func initProxies() {
	monolithTarget, _ := url.Parse(config.MonolithURL)
	moviesTarget, _ := url.Parse(config.MoviesServiceURL)
	eventsTarget, _ := url.Parse(config.EventsServiceURL)

	monolithProxy = httputil.NewSingleHostReverseProxy(monolithTarget)
	moviesServiceProxy = httputil.NewSingleHostReverseProxy(moviesTarget)
	eventsServiceProxy = httputil.NewSingleHostReverseProxy(eventsTarget)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func moviesHandler(w http.ResponseWriter, r *http.Request) {
	if shouldRouteToNewService() {
		log.Printf("Routing to new movies service")
		moviesServiceProxy.ServeHTTP(w, r)
	} else {
		log.Printf("Routing to monolith")
		monolithProxy.ServeHTTP(w, r)
	}
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	eventsServiceProxy.ServeHTTP(w, r)
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {
	monolithProxy.ServeHTTP(w, r)
}

func shouldRouteToNewService() bool {
	if !config.GradualMigration {
		return true
	}

	// Simple random percentage-based routing
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(100) < config.MoviesMigrationPercent
}