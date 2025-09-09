package portal

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/AnteWall/go-wifiportal/pkg/network"
)

//go:embed templates/*.html
var templateFiles embed.FS

// Config represents the configuration for the WiFi setup portal server
type Config struct {
	Port        string `yaml:"port" json:"port"`
	Interface   string `yaml:"interface" json:"interface"`       // WiFi interface to manage
	SSID        string `yaml:"ssid" json:"ssid"`                 // SSID of the AP hosting this portal
	Gateway     string `yaml:"gateway" json:"gateway"`           // Gateway IP of the AP
	RedirectURL string `yaml:"redirect_url" json:"redirect_url"` // Optional redirect after setup
}

// Server represents the WiFi setup portal HTTP server
type Server struct {
	config           Config
	server           *http.Server
	router           *mux.Router
	logger           *slog.Logger
	interfaceManager network.InterfaceManager
	setupTemplate    *template.Template
}

// NewServer creates a new WiFi setup portal server
func NewServer(config Config) *Server {
	router := mux.NewRouter()

	// Pre-parse the setup template
	setupTemplate, err := template.ParseFS(templateFiles, "templates/setup.html")
	if err != nil {
		panic(fmt.Sprintf("failed to parse setup template: %v", err))
	}

	server := &Server{
		config:           config,
		router:           router,
		logger:           slog.Default().WithGroup("wifi_setup_portal"),
		interfaceManager: network.NewInterfaceManager(),
		setupTemplate:    setupTemplate,
		server: &http.Server{
			Addr:           fmt.Sprintf(":%s", config.Port),
			Handler:        router,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			IdleTimeout:    30 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
	}

	server.setupRoutes()
	return server
}

// Middleware

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Debug("HTTP request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// timeoutMiddleware handles request timeouts
func (s *Server) timeoutMiddleware(next http.Handler) http.Handler {
	return http.TimeoutHandler(next, 30*time.Second, "Request timeout")
}

// setupRoutes configures all the HTTP routes
func (s *Server) setupRoutes() {
	// Add middleware
	s.router.Use(s.loggingMiddleware)
	s.router.Use(s.timeoutMiddleware)

	// Captive portal detection endpoints - redirect to WiFi setup
	s.router.HandleFunc("/generate_204", s.handleCaptiveDetection).Methods("GET")
	s.router.HandleFunc("/hotspot-detect.html", s.handleCaptiveDetection).Methods("GET")
	s.router.HandleFunc("/connecttest.txt", s.handleCaptiveDetection).Methods("GET")
	s.router.HandleFunc("/canonical.html", s.handleCaptiveDetection).Methods("GET")
	s.router.HandleFunc("/success.txt", s.handleCaptiveDetection).Methods("GET")

	// Main WiFi setup pages
	s.router.HandleFunc("/", s.handleWiFiSetup).Methods("GET")
	s.router.HandleFunc("/setup", s.handleWiFiSetup).Methods("GET")
	s.router.HandleFunc("/connect", s.handleConnect).Methods("POST")
	s.router.HandleFunc("/success", s.handleSuccess).Methods("GET")

	// API endpoints
	s.router.HandleFunc("/api/networks", s.handleAPINetworks).Methods("GET")
	s.router.HandleFunc("/api/connect", s.handleAPIConnect).Methods("POST")
	s.router.HandleFunc("/api/status", s.handleAPIStatus).Methods("GET")
	s.router.HandleFunc("/api/interfaces", s.handleAPIInterfaces).Methods("GET")

	// Static files
	s.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	// Catch-all redirect to WiFi setup
	s.router.PathPrefix("/").HandlerFunc(s.handleCatchAll)
}

// Captive Portal Detection Handlers

// handleCaptiveDetection handles captive portal detection requests
func (s *Server) handleCaptiveDetection(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("captive portal detection request",
		slog.String("path", r.URL.Path),
		slog.String("user_agent", r.UserAgent()),
		slog.String("client_ip", r.RemoteAddr))

	// Redirect to WiFi setup page
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleWiFiSetup displays the WiFi setup page
func (s *Server) handleWiFiSetup(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.logger.Debug("WiFi setup page request", slog.String("client_ip", r.RemoteAddr))

	// Use configured interface or let the network API determine the best interface
	interfaceName := s.config.Interface
	if interfaceName == "" {
		// Don't call ListWirelessInterfaces here as it's slow due to AP mode checks
		// Instead, let the /api/networks endpoint handle interface detection
		interfaceName = "auto" // Placeholder that will be resolved in API call
	}

	data := map[string]interface{}{
		"Interface": interfaceName,
		"Error":     r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	templateStart := time.Now()
	if err := s.setupTemplate.Execute(w, data); err != nil {
		s.logger.Error("failed to execute setup template", slog.String("error", err.Error()))
		// Don't call http.Error here as headers are already written
		return
	}
	s.logger.Debug("template execution completed", 
		slog.Duration("template_duration", time.Since(templateStart)),
		slog.Duration("total_duration", time.Since(start)))
}

// handleConnect processes WiFi connection attempts
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.logger.Error("failed to parse connection form", slog.String("error", err.Error()))
		http.Redirect(w, r, "/setup?error=invalid_form", http.StatusSeeOther)
		return
	}

	ssid := r.FormValue("ssid")
	password := r.FormValue("password")
	interfaceName := r.FormValue("interface")

	s.logger.Info("connection attempt",
		slog.String("ssid", ssid),
		slog.String("interface", interfaceName),
		slog.String("client_ip", r.RemoteAddr))

	if ssid == "" {
		http.Redirect(w, r, "/setup?error=ssid_required", http.StatusSeeOther)
		return
	}

	if interfaceName == "" {
		http.Redirect(w, r, "/setup?error=interface_required", http.StatusSeeOther)
		return
	}

	// Attempt to connect to the network
	err := s.interfaceManager.ConnectToNetwork(interfaceName, ssid, password)
	if err != nil {
		s.logger.Error("failed to connect to network",
			slog.String("ssid", ssid),
			slog.String("error", err.Error()))
		http.Redirect(w, r, "/setup?error=connection_failed", http.StatusSeeOther)
		return
	}

	// Redirect to success page
	http.Redirect(w, r, "/success?ssid="+ssid, http.StatusSeeOther)
}

// handleSuccess serves the success page after connection
func (s *Server) handleSuccess(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("success page request", slog.String("client_ip", r.RemoteAddr))

	tmpl, err := template.ParseFS(templateFiles, "templates/success.html")
	if err != nil {
		s.logger.Error("failed to parse success template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"SSID": r.URL.Query().Get("ssid"),
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Error("failed to execute success template", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// API Handlers

// handleAPINetworks returns available networks as JSON
func (s *Server) handleAPINetworks(w http.ResponseWriter, r *http.Request) {
	interfaceName := r.URL.Query().Get("interface")
	if interfaceName == "" || interfaceName == "auto" {
		// Use configured interface or let nmcli scan all interfaces
		interfaceName = s.config.Interface
		// If no interface configured, let nmcli scan all interfaces (empty string)
		// This avoids the expensive ListWirelessInterfaces call
	}

	s.logger.Info("scanning for networks", slog.String("interface", interfaceName))
	
	networks, err := s.interfaceManager.ListAvailableNetworks(interfaceName)
	if err != nil {
		s.logger.Error("failed to list networks", slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "error",
			"error":     err.Error(),
			"networks":  []network.WirelessNetwork{},
			"interface": interfaceName,
		})
		return
	}

	s.logger.Info("network scan completed", 
		slog.String("interface", interfaceName),
		slog.Int("count", len(networks)))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "success",
		"networks":  networks,
		"interface": interfaceName,
		"count":     len(networks),
	})
}

// handleAPIInterfaces returns available wireless interfaces as JSON
func (s *Server) handleAPIInterfaces(w http.ResponseWriter, r *http.Request) {
	interfaces, err := s.interfaceManager.ListWirelessInterfaces()
	if err != nil {
		s.logger.Error("failed to list interfaces", slog.String("error", err.Error()))
		http.Error(w, "Failed to list interfaces", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"interfaces": interfaces,
	})
}

// handleAPIConnect handles API-based connection requests
func (s *Server) handleAPIConnect(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SSID      string `json:"ssid"`
		Password  string `json:"password"`
		Interface string `json:"interface"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"error":  "Invalid request body",
		})
		return
	}

	if request.Interface == "" {
		// Use configured interface, or let the connection method figure it out
		request.Interface = s.config.Interface
		// Note: ConnectToNetwork can handle empty interface name if needed
	}

	err := s.interfaceManager.ConnectToNetwork(request.Interface, request.SSID, request.Password)
	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "success",
		"message":   "Connected to WiFi network",
		"ssid":      request.SSID,
		"interface": request.Interface,
	})
}

// handleAPIStatus provides connection status
func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	interfaces, err := s.interfaceManager.ListWirelessInterfaces()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "active",
		"interfaces": interfaces,
	})
}

// handleCatchAll redirects any unmatched requests to WiFi setup
func (s *Server) handleCatchAll(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("catch-all redirect to WiFi setup",
		slog.String("path", r.URL.Path),
		slog.String("client_ip", r.RemoteAddr))

	http.Redirect(w, r, "/", http.StatusFound)
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("starting WiFi setup captive portal server",
		slog.String("address", s.server.Addr),
		slog.String("interface", s.config.Interface),
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
	s.logger.Info("stopping WiFi setup captive portal server")

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
