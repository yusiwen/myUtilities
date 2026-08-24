package oauth

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// embeddedFiles embeds the HTML templates and CSS static files.
// Embed paths are relative to this file's directory.
//
//go:embed templates/*.html static/*.css
var embeddedFiles embed.FS

// Client represents an OAuth2 registered client application.
type Client struct {
	ID           string
	Name         string
	Secret       string
	RedirectURIs []string
}

// AuthorizationCode represents an OAuth2 authorization code.
type AuthorizationCode struct {
	Code        string
	ClientID    string
	RedirectURI string
	ExpiresAt   time.Time
	Scope       string
	UserID      string
}

// AccessToken represents an OAuth2 access token.
type AccessToken struct {
	Token     string
	Type      string
	ExpiresIn int64
	Scope     string
	UserID    string
	ClientID  string
}

// JwtCustomClaims extends jwt.RegisteredClaims with custom JWT payload fields.
type JwtCustomClaims struct {
	UserID   string `json:"user_id"`
	ClientID string `json:"client_id"`
	Scope    string `json:"scope"`
	jwt.RegisteredClaims
}

// User represents an authenticated user.
type User struct {
	ID       string
	Username string
	Password string
}

// AuthRequest represents a pending OAuth2 authorization request.
type AuthRequest struct {
	ID           string
	ClientID     string
	RedirectURI  string
	ResponseType string
	State        string
	Scope        string
	UserID       string
	ExpiresAt    time.Time
}

// AuthServer holds all state for the OAuth2 mock server.
type AuthServer struct {
	clients      map[string]*Client
	users        map[string]*User
	authCodes    map[string]*AuthorizationCode
	accessTokens map[string]*AccessToken
	authRequests map[string]*AuthRequest
	sessions     map[string]string
	templates    *template.Template
	staticFS     http.FileSystem
	jwtSecret    []byte // Secret key used to sign JWTs
}

// NewAuthServer creates and initializes a new AuthServer instance.
func NewAuthServer() *AuthServer {
	server := &AuthServer{
		clients:      make(map[string]*Client),
		users:        make(map[string]*User),
		authCodes:    make(map[string]*AuthorizationCode),
		accessTokens: make(map[string]*AccessToken),
		authRequests: make(map[string]*AuthRequest),
		sessions:     make(map[string]string),
		jwtSecret:    []byte("your-256-bit-secret"), // Use a more secure secret in production
	}

	// Seed demo data
	server.clients["client1"] = &Client{
		ID:           "client1",
		Name:         "示例应用",
		Secret:       "secret1",
		RedirectURIs: []string{"http://localhost:8080/login/oauth2/code/custom-auth-server"},
	}

	server.users["user1"] = &User{
		ID:       "user1",
		Username: "alice",
		Password: "password123",
	}

	// Parse templates
	templates, err := parseTemplates()
	if err != nil {
		log.Fatal("Failed to parse templates:", err)
	}
	server.templates = templates

	// Create static file filesystem
	staticFS, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		log.Fatal("Failed to create static filesystem:", err)
	}
	server.staticFS = http.FS(staticFS)

	return server
}

// parseTemplates parses HTML templates from the embedded filesystem.
func parseTemplates() (*template.Template, error) {
	tmpl := template.New("")

	// Iterate over embedded template files
	templateDir, err := embeddedFiles.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}

	for _, file := range templateDir {
		if file.IsDir() {
			continue
		}

		// Read template file content
		filePath := "templates/" + file.Name()
		content, err := embeddedFiles.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read template file %s: %w", filePath, err)
		}

		// Parse the template
		tmpl, err = tmpl.New(file.Name()).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", file.Name(), err)
		}
	}

	return tmpl, nil
}

// SetupRoutes registers HTTP route handlers on the given mux.
func (s *AuthServer) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.homeHandler)
	mux.HandleFunc("/clients", s.clientsHandler)
	mux.HandleFunc("/login", s.loginHandler)
	mux.HandleFunc("/auth", s.authHandler)
	mux.HandleFunc("/authorize", s.authorizeHandler)
	mux.HandleFunc("/token", s.tokenHandler)
	mux.HandleFunc("/userinfo", s.userInfoHandler)
	mux.HandleFunc("/verify", s.verifyTokenHandler)

	// Serve static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(s.staticFS)))
}

// homeHandler serves the landing page.
func (s *AuthServer) homeHandler(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Clients": s.clients,
	}
	err := s.templates.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *AuthServer) clientsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		data := map[string]interface{}{
			"Clients": s.clients,
		}
		err := s.templates.ExecuteTemplate(w, "clients.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	case "POST":
		s.addClients(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *AuthServer) addClients(w http.ResponseWriter, r *http.Request) {
	type Input struct {
		ClientID     string `json:"clientId"`
		ClientName   string `json:"clientName"`
		ClientSecret string `json:"clientSecret"`
		RedirectURI  string `json:"redirectUri"`
	}

	var input Input
	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	if s.clients[input.ClientID] != nil {
		http.Error(w, "Client ID already exists", http.StatusBadRequest)
		return
	}

	client := &Client{
		ID:           input.ClientID,
		Name:         input.ClientName,
		Secret:       input.ClientSecret,
		RedirectURIs: []string{input.RedirectURI},
	}
	s.clients[client.ID] = client
}

// loginHandler serves the login page and processes login submissions.
func (s *AuthServer) loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Render login page
		authRequestID := r.URL.Query().Get("request_id")
		clientID := r.URL.Query().Get("client_id")

		data := map[string]interface{}{
			"AuthRequestID": authRequestID,
			"ClientID":      clientID,
			"Client":        s.clients[clientID],
		}
		err := s.templates.ExecuteTemplate(w, "login.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Process login form submission
	r.ParseForm()
	username := r.FormValue("username")
	password := r.FormValue("password")
	authRequestID := r.FormValue("request_id")
	//clientID := r.FormValue("client_id")

	// Validate user credentials
	var user *User
	for _, u := range s.users {
		if u.Username == username && u.Password == password {
			user = u
			break
		}
	}

	if user == nil {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	// Create session
	sessionID, _ := generateRandomString(32)
	s.sessions[sessionID] = user.ID

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
	})

	// Redirect to authorization page if there's a pending auth request
	if authRequestID != "" {
		authRequest, exists := s.authRequests[authRequestID]
		if exists {
			authRequest.UserID = user.ID
			http.Redirect(w, r, fmt.Sprintf("/auth?request_id=%s", authRequestID), http.StatusFound)
			return
		}
	}

	// Redirect to home if no specific auth request
	http.Redirect(w, r, "/", http.StatusFound)
}

// authHandler serves the authorization page and processes authorization decisions.
func (s *AuthServer) authHandler(w http.ResponseWriter, r *http.Request) {
	// Check session
	sessionID, err := r.Cookie("oauth_session")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	userID, exists := s.sessions[sessionID.Value]
	if !exists {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	authRequestID := r.URL.Query().Get("request_id")
	authRequest, exists := s.authRequests[authRequestID]
	if !exists {
		http.Error(w, "Invalid authorization request", http.StatusBadRequest)
		return
	}

	if r.Method == "GET" {
		// Render authorization page
		data := map[string]interface{}{
			"AuthRequest": authRequest,
			"Client":      s.clients[authRequest.ClientID],
			"User":        s.users[userID],
		}
		err := s.templates.ExecuteTemplate(w, "auth.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Process authorization decision
	r.ParseForm()
	decision := r.FormValue("decision")

	if decision != "allow" {
		// User denied authorization
		redirectURL, _ := url.Parse(authRequest.RedirectURI)
		params := redirectURL.Query()
		params.Add("error", "access_denied")
		if authRequest.State != "" {
			params.Add("state", authRequest.State)
		}
		redirectURL.RawQuery = params.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		return
	}

	// User authorized: generate authorization code
	code, err := generateRandomString(32)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Store authorization code
	authCode := &AuthorizationCode{
		Code:        code,
		ClientID:    authRequest.ClientID,
		RedirectURI: authRequest.RedirectURI,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
		Scope:       authRequest.Scope,
		UserID:      authRequest.UserID,
	}
	s.authCodes[code] = authCode

	// Build redirect URL
	redirectURL, _ := url.Parse(authRequest.RedirectURI)
	params := redirectURL.Query()
	params.Add("code", code)
	if authRequest.State != "" {
		params.Add("state", authRequest.State)
	}
	redirectURL.RawQuery = params.Encode()

	// Clean up the auth request
	delete(s.authRequests, authRequestID)

	// Redirect to the client
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// authorizeHandler handles the OAuth2 authorization endpoint.
func (s *AuthServer) authorizeHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	clientID := query.Get("client_id")
	redirectURI := query.Get("redirect_uri")
	responseType := query.Get("response_type")
	state := query.Get("state")
	scope := query.Get("scope")

	// Validate required parameters
	if clientID == "" || redirectURI == "" || responseType != "code" {
		http.Error(w, "Invalid request parameters", http.StatusBadRequest)
		return
	}

	// Verify the client exists
	client, exists := s.clients[clientID]
	if !exists {
		http.Error(w, "Client not found", http.StatusBadRequest)
		return
	}

	// Verify the redirect URI is registered
	validRedirectURI := false
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			validRedirectURI = true
			break
		}
	}

	if !validRedirectURI {
		http.Error(w, "Invalid redirect URI", http.StatusBadRequest)
		return
	}

	// Create authorization request
	authRequestID, _ := generateRandomString(32)
	s.authRequests[authRequestID] = &AuthRequest{
		ID:           authRequestID,
		ClientID:     clientID,
		RedirectURI:  redirectURI,
		ResponseType: responseType,
		State:        state,
		Scope:        scope,
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	// Check if user is logged in
	sessionID, err := r.Cookie("oauth_session")
	if err != nil {
		// Not logged in, redirect to login page
		http.Redirect(w, r, fmt.Sprintf("/login?request_id=%s&client_id=%s", authRequestID, clientID), http.StatusFound)
		return
	}

	userID, exists := s.sessions[sessionID.Value]
	if !exists {
		// Session invalid, redirect to login page
		http.Redirect(w, r, fmt.Sprintf("/login?request_id=%s&client_id=%s", authRequestID, clientID), http.StatusFound)
		return
	}

	// User is logged in, set user ID and redirect to auth page
	s.authRequests[authRequestID].UserID = userID
	http.Redirect(w, r, fmt.Sprintf("/auth?request_id=%s", authRequestID), http.StatusFound)
}

// tokenHandler handles the OAuth2 token endpoint.
func (s *AuthServer) tokenHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form body
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	// Validate grant type
	if grantType != "authorization_code" {
		http.Error(w, "Unsupported grant type", http.StatusBadRequest)
		return
	}

	// Validate client credentials
	client, exists := s.clients[clientID]
	if !exists || client.Secret != clientSecret {
		http.Error(w, "Invalid client credentials", http.StatusUnauthorized)
		return
	}

	// Look up the authorization code
	authCode, exists := s.authCodes[code]
	if !exists {
		http.Error(w, "Invalid authorization code", http.StatusBadRequest)
		return
	}

	// Check if authorization code has expired
	if time.Now().After(authCode.ExpiresAt) {
		delete(s.authCodes, code) // Clean up expired code
		http.Error(w, "Authorization code expired", http.StatusBadRequest)
		return
	}

	// Validate redirect URI
	if authCode.RedirectURI != redirectURI {
		http.Error(w, "Redirect URI mismatch", http.StatusBadRequest)
		return
	}

	// Validate client ID
	if authCode.ClientID != clientID {
		http.Error(w, "Client ID mismatch", http.StatusBadRequest)
		return
	}

	expirationTime := time.Now().Add(time.Hour)
	claims := &JwtCustomClaims{
		UserID:   authCode.UserID,
		ClientID: clientID,
		Scope:    authCode.Scope,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "http://localhost",
			Subject:   authCode.UserID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Generate access token
	accessToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		http.Error(w, "Token generation error", http.StatusInternalServerError)
		return
	}

	// Cache access token
	cachedToken := &AccessToken{
		Token:     accessToken,
		Type:      "Bearer",
		ExpiresIn: 3600, // 1-hour validity
		Scope:     authCode.Scope,
		UserID:    authCode.UserID,
		ClientID:  clientID,
	}
	s.accessTokens[accessToken] = cachedToken

	// Clean up used authorization code
	delete(s.authCodes, code)

	log.Printf("Generated token for user %s: %s", authCode.UserID, accessToken)

	// Return token response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"scope":        authCode.Scope,
	})
}

// userInfoHandler handles the OAuth2 user info endpoint.
func (s *AuthServer) userInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accessToken := r.URL.Query().Get("access_token")
	if accessToken == "" {
		// Get access token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
			return
		}

		accessToken = authHeader[7:]
	}

	token, exists := s.accessTokens[accessToken]

	if !exists {
		http.Error(w, "Invalid access token", http.StatusUnauthorized)
		return
	}

	// Check token expiration (simplified; in production, check actual timestamp)
	user, exists := s.users[token.UserID]
	if !exists {
		http.Error(w, "User not found", http.StatusInternalServerError)
		return
	}

	// Return user info
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sub":  user.ID,
		"name": user.Username,
	})
}

// verifyTokenHandler handles JWT token verification.
func (s *AuthServer) verifyTokenHandler(w http.ResponseWriter, r *http.Request) {
	// Accept GET and POST requests
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get token from query param or body
	var tokenString string
	if r.Method == "GET" {
		tokenString = r.URL.Query().Get("token")
	} else {
		// Get from POST request body
		r.ParseForm()
		tokenString = r.FormValue("token")
	}

	// If not in query params, try Authorization header
	if tokenString == "" {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		}
	}

	if tokenString == "" {
		http.Error(w, "Token required", http.StatusBadRequest)
		return
	}

	// Parse and validate the token
	claims := &JwtCustomClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	// Handle verification result
	response := map[string]interface{}{}
	if err != nil {
		response["valid"] = false
		response["error"] = err.Error()
		w.WriteHeader(http.StatusUnauthorized)
	} else if !token.Valid {
		response["valid"] = false
		response["error"] = "Invalid token"
		w.WriteHeader(http.StatusUnauthorized)
	} else {
		response["valid"] = true
		response["user_id"] = claims.UserID
		response["client_id"] = claims.ClientID
		response["scope"] = claims.Scope
		response["expires_at"] = claims.ExpiresAt.Time.Unix()
	}

	// Return verification result
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// generateRandomString generates a cryptographically secure random string of the given length.
func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
