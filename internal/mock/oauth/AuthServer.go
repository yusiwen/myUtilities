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

// 注意：嵌入路径是相对于当前文件的路径
//
//go:embed templates/*.html static/*.css
var embeddedFiles embed.FS

// 客户端信息
type Client struct {
	ID           string
	Name         string
	Secret       string
	RedirectURIs []string
}

// 授权码
type AuthorizationCode struct {
	Code        string
	ClientID    string
	RedirectURI string
	ExpiresAt   time.Time
	Scope       string
	UserID      string
}

// 访问令牌
type AccessToken struct {
	Token     string
	Type      string
	ExpiresIn int64
	Scope     string
	UserID    string
	ClientID  string
}

// JWT 声明结构
type JwtCustomClaims struct {
	UserID   string `json:"user_id"`
	ClientID string `json:"client_id"`
	Scope    string `json:"scope"`
	jwt.RegisteredClaims
}

// 用户信息
type User struct {
	ID       string
	Username string
	Password string
}

// 授权请求会话
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

// AuthServer 结构体，包含所有服务器状态
type AuthServer struct {
	clients      map[string]*Client
	users        map[string]*User
	authCodes    map[string]*AuthorizationCode
	accessTokens map[string]*AccessToken
	authRequests map[string]*AuthRequest
	sessions     map[string]string
	templates    *template.Template
	staticFS     http.FileSystem
	jwtSecret    []byte // 用于签名JWT的密钥
}

// NewAuthServer 创建并初始化一个新的认证服务器实例
func NewAuthServer() *AuthServer {
	server := &AuthServer{
		clients:      make(map[string]*Client),
		users:        make(map[string]*User),
		authCodes:    make(map[string]*AuthorizationCode),
		accessTokens: make(map[string]*AccessToken),
		authRequests: make(map[string]*AuthRequest),
		sessions:     make(map[string]string),
		jwtSecret:    []byte("your-256-bit-secret"), // 请使用更安全的密钥
	}

	// 初始化示例数据
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

	// 解析模板
	templates, err := parseTemplates()
	if err != nil {
		log.Fatal("Failed to parse templates:", err)
	}
	server.templates = templates

	// 创建静态文件系统
	staticFS, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		log.Fatal("Failed to create static filesystem:", err)
	}
	server.staticFS = http.FS(staticFS)

	return server
}

// parseTemplates 从嵌入的文件系统中解析模板
func parseTemplates() (*template.Template, error) {
	tmpl := template.New("")

	// 遍历嵌入的模板文件
	templateDir, err := embeddedFiles.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}

	for _, file := range templateDir {
		if file.IsDir() {
			continue
		}

		// 读取模板文件内容
		filePath := "templates/" + file.Name()
		content, err := embeddedFiles.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read template file %s: %w", filePath, err)
		}

		// 解析模板
		tmpl, err = tmpl.New(file.Name()).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", file.Name(), err)
		}
	}

	return tmpl, nil
}

// SetupRoutes 设置HTTP路由处理
func (s *AuthServer) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.homeHandler)
	mux.HandleFunc("/clients", s.clientsHandler)
	mux.HandleFunc("/login", s.loginHandler)
	mux.HandleFunc("/auth", s.authHandler)
	mux.HandleFunc("/authorize", s.authorizeHandler)
	mux.HandleFunc("/token", s.tokenHandler)
	mux.HandleFunc("/userinfo", s.userInfoHandler)
	mux.HandleFunc("/verify", s.verifyTokenHandler)

	// 静态文件服务
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(s.staticFS)))
}

// 首页处理器
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

// 登录页面处理器
func (s *AuthServer) loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// 显示登录页面
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

	// 处理登录表单提交
	r.ParseForm()
	username := r.FormValue("username")
	password := r.FormValue("password")
	authRequestID := r.FormValue("request_id")
	//clientID := r.FormValue("client_id")

	// 验证用户凭据
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

	// 创建会话
	sessionID, _ := generateRandomString(32)
	s.sessions[sessionID] = user.ID

	// 设置会话cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
	})

	// 如果存在授权请求，重定向到授权页面
	if authRequestID != "" {
		authRequest, exists := s.authRequests[authRequestID]
		if exists {
			authRequest.UserID = user.ID
			http.Redirect(w, r, fmt.Sprintf("/auth?request_id=%s", authRequestID), http.StatusFound)
			return
		}
	}

	// 如果没有特定授权请求，重定向到首页
	http.Redirect(w, r, "/", http.StatusFound)
}

// 授权页面处理器
func (s *AuthServer) authHandler(w http.ResponseWriter, r *http.Request) {
	// 检查会话
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
		// 显示授权页面
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

	// 处理授权决定
	r.ParseForm()
	decision := r.FormValue("decision")

	if decision != "allow" {
		// 用户拒绝授权
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

	// 用户同意授权，生成授权码
	code, err := generateRandomString(32)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// 存储授权码
	authCode := &AuthorizationCode{
		Code:        code,
		ClientID:    authRequest.ClientID,
		RedirectURI: authRequest.RedirectURI,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
		Scope:       authRequest.Scope,
		UserID:      authRequest.UserID,
	}
	s.authCodes[code] = authCode

	// 构建重定向URL
	redirectURL, _ := url.Parse(authRequest.RedirectURI)
	params := redirectURL.Query()
	params.Add("code", code)
	if authRequest.State != "" {
		params.Add("state", authRequest.State)
	}
	redirectURL.RawQuery = params.Encode()

	// 清理授权请求
	delete(s.authRequests, authRequestID)

	// 重定向到客户端
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// 授权端点处理器
func (s *AuthServer) authorizeHandler(w http.ResponseWriter, r *http.Request) {
	// 解析查询参数
	query := r.URL.Query()
	clientID := query.Get("client_id")
	redirectURI := query.Get("redirect_uri")
	responseType := query.Get("response_type")
	state := query.Get("state")
	scope := query.Get("scope")

	// 验证必要参数
	if clientID == "" || redirectURI == "" || responseType != "code" {
		http.Error(w, "Invalid request parameters", http.StatusBadRequest)
		return
	}

	// 验证客户端是否存在
	client, exists := s.clients[clientID]
	if !exists {
		http.Error(w, "Client not found", http.StatusBadRequest)
		return
	}

	// 验证重定向URI是否已注册
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

	// 创建授权请求
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

	// 检查用户是否已登录
	sessionID, err := r.Cookie("oauth_session")
	if err != nil {
		// 未登录，重定向到登录页面
		http.Redirect(w, r, fmt.Sprintf("/login?request_id=%s&client_id=%s", authRequestID, clientID), http.StatusFound)
		return
	}

	userID, exists := s.sessions[sessionID.Value]
	if !exists {
		// 会话无效，重定向到登录页面
		http.Redirect(w, r, fmt.Sprintf("/login?request_id=%s&client_id=%s", authRequestID, clientID), http.StatusFound)
		return
	}

	// 用户已登录，设置用户ID并重定向到授权页面
	s.authRequests[authRequestID].UserID = userID
	http.Redirect(w, r, fmt.Sprintf("/auth?request_id=%s", authRequestID), http.StatusFound)
}

// 令牌端点处理器
func (s *AuthServer) tokenHandler(w http.ResponseWriter, r *http.Request) {
	// 只接受POST请求
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
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

	// 验证授权类型
	if grantType != "authorization_code" {
		http.Error(w, "Unsupported grant type", http.StatusBadRequest)
		return
	}

	// 验证客户端凭据
	client, exists := s.clients[clientID]
	if !exists || client.Secret != clientSecret {
		http.Error(w, "Invalid client credentials", http.StatusUnauthorized)
		return
	}

	// 查找授权码
	authCode, exists := s.authCodes[code]
	if !exists {
		http.Error(w, "Invalid authorization code", http.StatusBadRequest)
		return
	}

	// 检查授权码是否过期
	if time.Now().After(authCode.ExpiresAt) {
		delete(s.authCodes, code) // 清理过期代码
		http.Error(w, "Authorization code expired", http.StatusBadRequest)
		return
	}

	// 验证重定向URI
	if authCode.RedirectURI != redirectURI {
		http.Error(w, "Redirect URI mismatch", http.StatusBadRequest)
		return
	}

	// 验证客户端ID
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

	// 生成访问令牌
	accessToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		http.Error(w, "Token generation error", http.StatusInternalServerError)
		return
	}

	// 存储访问令牌
	cachedToken := &AccessToken{
		Token:     accessToken,
		Type:      "Bearer",
		ExpiresIn: 3600, // 1小时有效期
		Scope:     authCode.Scope,
		UserID:    authCode.UserID,
		ClientID:  clientID,
	}
	s.accessTokens[accessToken] = cachedToken

	// 清理已使用的授权码
	delete(s.authCodes, code)

	log.Printf("Generated token for user %s: %s", authCode.UserID, accessToken)

	// 返回令牌响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"scope":        authCode.Scope,
	})
}

// 用户信息端点处理器
func (s *AuthServer) userInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accessToken := r.URL.Query().Get("access_token")
	if accessToken == "" {
		// 从Authorization头获取访问令牌
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

	// 检查令牌是否过期（简化处理，实际应该检查时间）
	user, exists := s.users[token.UserID]
	if !exists {
		http.Error(w, "User not found", http.StatusInternalServerError)
		return
	}

	// 返回用户信息
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sub":  user.ID,
		"name": user.Username,
	})
}

// verifyHandler 验证JWT Token的接口
func (s *AuthServer) verifyTokenHandler(w http.ResponseWriter, r *http.Request) {
	// 支持GET和POST请求
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从查询参数或请求头中获取token
	var tokenString string
	if r.Method == "GET" {
		tokenString = r.URL.Query().Get("token")
	} else {
		// 从POST请求体中获取
		r.ParseForm()
		tokenString = r.FormValue("token")
	}

	// 如果查询参数中没有，尝试从Authorization头获取
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

	// 解析和验证Token
	claims := &JwtCustomClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	// 处理验证结果
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

	// 返回验证结果
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// 生成随机字符串
func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
