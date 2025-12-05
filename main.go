package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-smtp"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/spf13/viper"
	"software.sslmate.com/src/go-pkcs12"
)

type Config struct {
	TenantID       string `mapstructure:"ms_graph_tenant_id"`
	ClientID       string `mapstructure:"ms_graph_client_id"`
	CertPath       string `mapstructure:"ms_graph_cert_path"`
	CertPassword   string `mapstructure:"ms_graph_cert_pass"`
	EmailFrom      string `mapstructure:"ms_graph_email_from"`
	SMTPPort       string `mapstructure:"smtp_port"`
	SMTPHost       string `mapstructure:"smtp_host"`
	RequireAuth    bool   `mapstructure:"require_auth"`
	AuthUsername   string `mapstructure:"smtp_auth_username"`
	AuthPassword   string `mapstructure:"smtp_auth_password"`
	HealthPort     string `mapstructure:"health_port"`
	LogLevel       string `mapstructure:"log_level"`
}

type Backend struct {
	config      *Config
	graphClient *msgraphsdk.GraphServiceClient
	logger      *slog.Logger
}

type Session struct {
	backend *Backend
	from    string
	to      []string
	logger  *slog.Logger
}

func loadConfig() (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("smtp_port", "8025")
	v.SetDefault("smtp_host", "0.0.0.0")
	v.SetDefault("require_auth", false)
	v.SetDefault("health_port", "8080")
	v.SetDefault("log_level", "info")

	// Bind environment variables
	v.AutomaticEnv()

	// Config file support
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/smtp-graph-bridge/")

	// Load .env file if present (for backward compatibility/dev)
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	if err := v.ReadInConfig(); err != nil {
		// It's okay if .env doesn't exist, we might be using env vars or yaml
		if !os.IsNotExist(err) {
			// If it exists but has error, log it (but don't fail yet, maybe env vars are enough)
			// Actually, viper.ReadInConfig returns error if file not found when SetConfigFile is used.
			// If we want to support multiple sources, we should try loading config.yaml too if .env fails or additionally.
			// For simplicity in this refactor: we try .env, if not found we rely on env vars.
			// Better approach for "hybrid":
			// Reset config file to look for yaml first
			v.SetConfigFile("") // Reset specific file
			v.SetConfigName("config")
			v.SetConfigType("yaml")
			_ = v.ReadInConfig() // Ignore error if config.yaml not found
			// .env is handled by AutomaticEnv if we used standard naming, but we have specific names.
			// To keep it simple and robust: we'll trust Viper's AutomaticEnv for strict env vars,
			// and try to read a config file if available.
		}
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	// Manual validation for required fields
	if config.TenantID == "" {
		return nil, fmt.Errorf("MS_GRAPH_TENANT_ID is required")
	}
	if config.ClientID == "" {
		return nil, fmt.Errorf("MS_GRAPH_CLIENT_ID is required")
	}
	if config.EmailFrom == "" {
		return nil, fmt.Errorf("MS_GRAPH_EMAIL_FROM is required")
	}
	if config.CertPath == "" {
		return nil, fmt.Errorf("MS_GRAPH_CERT_PATH is required")
	}

	return &config, nil
}

func initLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}

func loadPFXCertificate(certPath, password string) ([]byte, tls.Certificate, error) {
	pfxData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("failed to read certificate: %w", err)
	}

	privateKey, certificate, err := pkcs12.Decode(pfxData, password)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("failed to decode PFX: %w", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certificate.Raw},
		PrivateKey:  privateKey,
		Leaf:        certificate,
	}

	return pfxData, tlsCert, nil
}

func initGraphClient(config *Config, logger *slog.Logger) (*msgraphsdk.GraphServiceClient, error) {
	pfxData, _, err := loadPFXCertificate(config.CertPath, config.CertPassword)
	if err != nil {
		return nil, err
	}

	var password []byte
	if config.CertPassword != "" {
		password = []byte(config.CertPassword)
	}

	certs, key, err := azidentity.ParseCertificates(pfxData, password)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	cred, err := azidentity.NewClientCertificateCredential(
		config.TenantID,
		config.ClientID,
		certs,
		key,
		&azidentity.ClientCertificateCredentialOptions{
			ClientOptions: policy.ClientOptions{
				Retry: policy.RetryOptions{
					MaxRetries: 3,
				},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(
		cred,
		[]string{"https://graph.microsoft.com/.default"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Graph client: %w", err)
	}

	logger.Info("Graph client initialized", "email_from", config.EmailFrom)
	return client, nil
}

// SMTP Backend implementation
func (b *Backend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &Session{
		backend: b,
		logger:  b.logger.WithGroup("session"),
	}, nil
}

func (s *Session) AuthPlain(username, password string) error {
	if !s.backend.config.RequireAuth {
		return nil
	}
	if username == s.backend.config.AuthUsername && password == s.backend.config.AuthPassword {
		return nil
	}
	s.logger.Warn("Authentication failed", "username", username)
	return fmt.Errorf("invalid credentials")
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	// Parse email using go-message
	mr, err := mail.CreateReader(r)
	if err != nil {
		s.logger.Error("Failed to create mail reader", "error", err)
		return err
	}

	// Read header
	header := mr.Header
	subject, err := header.Subject()
	if err != nil {
		// Subject is optional, but good to have
		subject = "(No Subject)"
	}

	s.logger.Info("Processing email", "from", s.from, "to", s.to, "subject", subject)

	var bodyText, bodyHTML string

	// Process parts
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			s.logger.Error("Failed to read part", "error", err)
			break
		}

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			// This is the message body
			b, _ := io.ReadAll(p.Body)
			contentType, _, _ := h.ContentType()
			
			if contentType == "text/html" {
				bodyHTML = string(b)
			} else {
				bodyText = string(b)
			}
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			s.logger.Warn("Attachment detected but not supported yet. Skipping.", "filename", filename)
		}
	}

	// Determine which body to send (prefer HTML)
	finalBody := bodyText
	contentType := "text"
	if bodyHTML != "" {
		finalBody = bodyHTML
		contentType = "html"
	}
	
	// Send via Graph API
	err = s.sendViaGraph(s.to, subject, finalBody, contentType)
	if err != nil {
		s.logger.Error("Failed to send email via Graph", "error", err)
		return err
	}

	s.logger.Info("Email sent successfully", "recipient_count", len(s.to))
	return nil
}

func (s *Session) Reset() {
	s.from = ""
	s.to = nil
}

func (s *Session) Logout() error {
	return nil
}

func (s *Session) sendViaGraph(toAddresses []string, subject, body, contentType string) error {
	ctx := context.Background()

	// Build recipients
	recipients := []models.Recipientable{}
	for _, addr := range toAddresses {
		recipient := models.NewRecipient()
		emailAddr := models.NewEmailAddress()
		emailAddr.SetAddress(&addr)
		recipient.SetEmailAddress(emailAddr)
		recipients = append(recipients, recipient)
	}

	// Build message
	message := models.NewMessage()
	message.SetSubject(&subject)

	messageBody := models.NewItemBody()
	if contentType == "html" {
		bodyType := models.HTML_BODYTYPE
		messageBody.SetContentType(&bodyType)
	} else {
		bodyType := models.TEXT_BODYTYPE
		messageBody.SetContentType(&bodyType)
	}
	messageBody.SetContent(&body)
	message.SetBody(messageBody)
	message.SetToRecipients(recipients)

	// Send email
	requestBody := users.NewItemSendMailPostRequestBody()
	requestBody.SetMessage(message)
	saveToSentItems := true
	requestBody.SetSaveToSentItems(&saveToSentItems)

	err := s.backend.graphClient.Users().
		ByUserId(s.backend.config.EmailFrom).
		SendMail().
		Post(ctx, requestBody, nil)

	return err
}

func startHealthServer(port string, logger *slog.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	logger.Info("Health server starting", "port", port)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("Health server failed", "error", err)
	}
}

func main() {
	// Initial logger (will be updated after config load if needed)
	logger := initLogger("info")
	logger.Info("Starting SMTP-Graph Bridge", "version", "0.1.0")

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		logger.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	// Re-init logger with configured level
	logger = initLogger(config.LogLevel)
	logger.Info("Configuration loaded", 
		"tenant_id", config.TenantID[:8]+"...", 
		"email_from", config.EmailFrom,
		"smtp_port", config.SMTPPort,
	)

	// Initialize Graph client
	graphClient, err := initGraphClient(config, logger)
	if err != nil {
		logger.Error("Failed to initialize Graph client", "error", err)
		os.Exit(1)
	}

	// Start Health Check Server
	go startHealthServer(config.HealthPort, logger)

	// Create SMTP backend
	backend := &Backend{
		config:      config,
		graphClient: graphClient,
		logger:      logger,
	}

	// Create SMTP server
	server := smtp.NewServer(backend)
	server.Addr = fmt.Sprintf("%s:%s", config.SMTPHost, config.SMTPPort)
	server.Domain = "localhost"
	server.ReadTimeout = 30 * time.Second
	server.WriteTimeout = 30 * time.Second
	server.MaxMessageBytes = 10 * 1024 * 1024 // 10MB
	server.MaxRecipients = 50
	server.AllowInsecureAuth = true
	
	// We don't need to log this via Printf anymore, the logger handles it structured
	if config.RequireAuth {
		logger.Info("SMTP authentication enabled")
	} else {
		logger.Info("SMTP authentication disabled")
	}

	logger.Info("SMTP server listening", "address", server.Addr)

	if err := server.ListenAndServe(); err != nil {
		logger.Error("SMTP server error", "error", err)
		os.Exit(1)
	}
}