package portal

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

//go:embed templates/*.html
var templateFiles embed.FS

// Config represents the configuration for the captive portal server
type Config struct {
	Port        string `yaml:"port" json:"port"`
	Gateway     string `yaml:"gateway" json:"gateway"`
	SSID        string `yaml:"ssid" json:"ssid"`
	RedirectURL string `yaml:"redirect_url" json:"redirectUrl"`
}

// Server represents the captive portal HTTP server
type Server struct {
	config Config
	server *http.Server
	router *mux.Router
	logger *slog.Logger
}

// NewServer creates a new captive portal server
func NewServer(config Config) *Server {
	router := mux.NewRouter()

	server := &Server{
		config: config,
		router: router,
		logger: slog.Default().WithGroup("portal_server"),
		server: &http.Server{
			Addr:         fmt.Sprintf(":%s", config.Port),
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}

	server.setupRoutes()
	return server
}

// setupRoutes configures all the HTTP routes
func (s *Server) setupRoutes() {
	// Captive portal detection endpoints
	s.router.HandleFunc("/generate_204", s.handleCaptiveDetection).Methods("GET")
	s.router.HandleFunc("/hotspot-detect.html", s.handleCaptiveDetection).Methods("GET")
	s.router.HandleFunc("/connecttest.txt", s.handleCaptiveDetection).Methods("GET")
	s.router.HandleFunc("/redirect", s.handleCaptiveDetection).Methods("GET")

	// Main captive portal pages
	s.router.HandleFunc("/", s.handlePortalHome).Methods("GET")
	s.router.HandleFunc("/login", s.handleLogin).Methods("GET", "POST")
	s.router.HandleFunc("/success", s.handleSuccess).Methods("GET")
	s.router.HandleFunc("/terms", s.handleTerms).Methods("GET")

	// API endpoints
	s.router.HandleFunc("/api/connect", s.handleAPIConnect).Methods("POST")
	s.router.HandleFunc("/api/status", s.handleAPIStatus).Methods("GET")

	// Static files
	s.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	// Catch-all redirect to portal
	s.router.PathPrefix("/").HandlerFunc(s.handleCatchAll)
}

// handleCaptiveDetection handles captive portal detection requests
func (s *Server) handleCaptiveDetection(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("captive portal detection request",
		slog.String("path", r.URL.Path),
		slog.String("user_agent", r.UserAgent()),
		slog.String("client_ip", r.RemoteAddr))

	// Redirect to portal login page
	http.Redirect(w, r, "/login", http.StatusFound)
}

// handlePortalHome serves the main portal home page
func (s *Server) handlePortalHome(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("portal home request", slog.String("client_ip", r.RemoteAddr))

	tmpl, err := template.ParseFS(templateFiles, "templates/home.html")
	if err != nil {
		s.logger.Error("failed to parse home template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"SSID":    s.config.SSID,
		"Gateway": s.config.Gateway,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Error("failed to execute home template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleLogin serves the login page and processes login attempts
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.serveLoginPage(w, r)
		return
	}

	// Handle POST - process login
	s.processLogin(w, r)
}

// serveLoginPage serves the login form
func (s *Server) serveLoginPage(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("login page request", slog.String("client_ip", r.RemoteAddr))

	tmpl, err := template.ParseFS(templateFiles, "templates/login.html")
	if err != nil {
		s.logger.Error("failed to parse login template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"SSID":    s.config.SSID,
		"Gateway": s.config.Gateway,
		"Error":   r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Error("failed to execute login template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// processLogin handles login form submission
func (s *Server) processLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.logger.Error("failed to parse login form", slog.String("error", err.Error()))
		http.Redirect(w, r, "/login?error=invalid_form", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	termsAccepted := r.FormValue("terms") == "on"

	s.logger.Info("login attempt",
		slog.String("username", username),
		slog.String("password", password), // In production, avoid logging passwords
		slog.String("client_ip", r.RemoteAddr),
		slog.Bool("terms_accepted", termsAccepted))

	// Simple validation - you can customize this
	if !termsAccepted {
		http.Redirect(w, r, "/login?error=terms_required", http.StatusSeeOther)
		return
	}

	if username == "" {
		http.Redirect(w, r, "/login?error=username_required", http.StatusSeeOther)
		return
	}

	// For demo purposes, accept any username/password combination
	// In production, you'd validate against a database or external service

	// Redirect to success page
	http.Redirect(w, r, "/success", http.StatusSeeOther)
}

// handleSuccess serves the success page after login
func (s *Server) handleSuccess(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("success page request", slog.String("client_ip", r.RemoteAddr))

	tmpl, err := template.ParseFS(templateFiles, "templates/success.html")
	if err != nil {
		s.logger.Error("failed to parse success template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"SSID":        s.config.SSID,
		"Gateway":     s.config.Gateway,
		"RedirectURL": s.config.RedirectURL,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Error("failed to execute success template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleTerms serves the terms and conditions page
func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("terms page request", slog.String("client_ip", r.RemoteAddr))

	tmpl, err := template.ParseFS(templateFiles, "templates/terms.html")
	if err != nil {
		s.logger.Error("failed to parse terms template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"SSID":    s.config.SSID,
		"Gateway": s.config.Gateway,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Error("failed to execute terms template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleAPIConnect handles API-based connection requests
func (s *Server) handleAPIConnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Simple JSON response for API clients
	response := `{"status": "success", "message": "Connected to WiFi portal", "redirect": "/success"}`
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}

// handleAPIStatus provides connection status
func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := fmt.Sprintf(`{"status": "active", "ssid": "%s", "gateway": "%s"}`,
		s.config.SSID, s.config.Gateway)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}

// handleCatchAll redirects any unmatched requests to the portal
func (s *Server) handleCatchAll(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("catch-all redirect",
		slog.String("path", r.URL.Path),
		slog.String("client_ip", r.RemoteAddr))

	http.Redirect(w, r, "/login", http.StatusFound)
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("starting captive portal server",
		slog.String("address", s.server.Addr),
		slog.String("ssid", s.config.SSID))

	// Start server in goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", slog.String("error", err.Error()))
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("stopping captive portal server")

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// Router returns the underlying mux router for custom route registration
func (s *Server) Router() *mux.Router {
	return s.router
}

// AddRoute adds a custom route to the server
func (s *Server) AddRoute(path string, handler http.HandlerFunc) *mux.Route {
	return s.router.HandleFunc(path, handler)
}

// AddRoutes allows external packages to register multiple routes
func (s *Server) AddRoutes(setupFunc func(*mux.Router)) {
	setupFunc(s.router)
}
