package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// Config
// ============================================================================

type Config struct {
	Port        string
	SMTPHost    string
	SMTPPort    string
	SMTPUser    string
	SMTPPass    string
	DestEmail   string
	DatabaseURL string
}

func LoadConfig() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		SMTPHost:    getEnv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:    getEnv("SMTP_PORT", "587"),
		SMTPUser:    getEnv("SMTP_USER", "alerts.provengo@gmail.com"),
		SMTPPass:    getEnv("SMTP_PASS", ""), // App Password do Gmail (16 chars)
		DestEmail:   getEnv("DEST_EMAIL", "alerts.provengo@gmail.com"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://sender_mail:AxdErFasSa1109012@198.18.12.76:5432/sender_mail?sslmode=disable"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ============================================================================
// Models
// ============================================================================

type EmailRequest struct {
	Origem   string `json:"origem"`
	Campanha string `json:"campanha"`
	EmailSrc string `json:"email_src"`
	Name     string `json:"name"`
	Assunto  string `json:"assunto"`
	Corpo    string `json:"corpo"`
	// To overrides the default DEST_EMAIL recipient when set (e.g. per-hostgroup
	// maintainer for FastNetMon alerts).
	To string `json:"to,omitempty"`
}

type EmailResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	ID      int64  `json:"id,omitempty"`
}

// ============================================================================
// Validações
// ============================================================================

var campanhasValidas = map[string]bool{
	"vendas":  true,
	"duvidas": true,
	"contato": true,
	"alertas": true, // alertas operacionais (ex: FastNetMon)
	"ataque":  true,
}

func (r *EmailRequest) Validate() error {
	if r.Origem == "" {
		return fmt.Errorf("campo 'origem' é obrigatório")
	}
	r.Origem = strings.ToLower(strings.TrimSpace(r.Origem))

	if r.Campanha == "" {
		return fmt.Errorf("campo 'campanha' é obrigatório")
	}
	r.Campanha = strings.ToLower(strings.TrimSpace(r.Campanha))
	if !campanhasValidas[r.Campanha] {
		return fmt.Errorf("campanha inválida: use vendas, duvidas, contato, alertas ou ataque")
	}
	r.To = strings.TrimSpace(r.To)

	if r.EmailSrc == "" {
		return fmt.Errorf("campo 'email_src' é obrigatório")
	}
	if !strings.Contains(r.EmailSrc, "@") {
		return fmt.Errorf("email_src inválido")
	}

	if r.Name == "" {
		return fmt.Errorf("campo 'name' é obrigatório")
	}
	if r.Assunto == "" {
		return fmt.Errorf("campo 'assunto' é obrigatório")
	}
	if r.Corpo == "" {
		return fmt.Errorf("campo 'corpo' é obrigatório")
	}

	return nil
}

// ============================================================================
// Service
// ============================================================================

type EmailService struct {
	config Config
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewEmailService(cfg Config, db *pgxpool.Pool) *EmailService {
	return &EmailService{
		config: cfg,
		db:     db,
		logger: slog.Default(),
	}
}

func (s *EmailService) SaveEmail(ctx context.Context, req *EmailRequest, status string, smtpError string) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO emails (origem, campanha, email_src, name, assunto, corpo, status, smtp_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, req.Origem, req.Campanha, req.EmailSrc, req.Name, req.Assunto, req.Corpo, status, smtpError).Scan(&id)
	return id, err
}

func (s *EmailService) SendEmail(req *EmailRequest) error {
	subject := fmt.Sprintf("[%s][%s] %s", strings.ToUpper(req.Campanha), req.Origem, req.Assunto)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
	<div style="background: #1a1a2e; color: #fff; padding: 20px; border-radius: 8px 8px 0 0;">
		<h2 style="margin: 0;">📧 Nova mensagem - %s</h2>
	</div>
	<div style="padding: 20px; border: 1px solid #e0e0e0; border-top: none;">
		<table style="width: 100%%; border-collapse: collapse;">
			<tr>
				<td style="padding: 8px; font-weight: bold; color: #555; width: 120px;">Origem:</td>
				<td style="padding: 8px;">%s</td>
			</tr>
			<tr style="background: #f9f9f9;">
				<td style="padding: 8px; font-weight: bold; color: #555;">Campanha:</td>
				<td style="padding: 8px;">%s</td>
			</tr>
			<tr>
				<td style="padding: 8px; font-weight: bold; color: #555;">Nome:</td>
				<td style="padding: 8px;">%s</td>
			</tr>
			<tr style="background: #f9f9f9;">
				<td style="padding: 8px; font-weight: bold; color: #555;">Email:</td>
				<td style="padding: 8px;"><a href="mailto:%s">%s</a></td>
			</tr>
		</table>
		<hr style="border: 1px solid #e0e0e0; margin: 16px 0;">
		<h3 style="color: #333;">%s</h3>
		<div style="background: #f5f5f5; padding: 16px; border-radius: 4px; line-height: 1.6; color: #333;">
			%s
		</div>
	</div>
	<div style="background: #f0f0f0; padding: 12px; text-align: center; font-size: 12px; color: #888; border-radius: 0 0 8px 8px;">
		Email Service | %s
	</div>
</body>
</html>`,
		strings.ToUpper(req.Campanha),
		req.Origem, req.Campanha, req.Name,
		req.EmailSrc, req.EmailSrc,
		req.Assunto,
		strings.ReplaceAll(req.Corpo, "\n", "<br>"),
		time.Now().Format("02/01/2006 15:04:05"),
	)

	// Recipient: per-request override, else the configured default.
	dest := s.config.DestEmail
	if req.To != "" {
		dest = req.To
	}

	// Monta headers MIME
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: Provengo Alerts <%s>\r\n", s.config.SMTPUser))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", dest))
	msg.WriteString(fmt.Sprintf("Reply-To: %s <%s>\r\n", req.Name, req.EmailSrc))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	msg.WriteString(fmt.Sprintf("X-Origin: %s\r\n", req.Origem))
	msg.WriteString(fmt.Sprintf("X-Campanha: %s\r\n", req.Campanha))
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	// Conecta via SMTP com STARTTLS
	addr := fmt.Sprintf("%s:%s", s.config.SMTPHost, s.config.SMTPPort)

	conn, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("dial smtp: %w", err)
	}
	defer conn.Close()

	// STARTTLS
	if err := conn.StartTLS(&tls.Config{ServerName: s.config.SMTPHost}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}

	// Auth
	auth := smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPass, s.config.SMTPHost)
	if err := conn.Auth(auth); err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	// Envelope
	if err := conn.Mail(s.config.SMTPUser); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := conn.Rcpt(dest); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}

	// Data
	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write([]byte(msg.String())); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}

	return conn.Quit()
}

// ============================================================================
// Handlers
// ============================================================================

func (s *EmailService) HandleSendEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, EmailResponse{
			Success: false,
			Message: "método não permitido, use POST",
		})
		return
	}

	var req EmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, EmailResponse{
			Success: false,
			Message: fmt.Sprintf("JSON inválido: %v", err),
		})
		return
	}

	if err := req.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, EmailResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	s.logger.Info("enviando email",
		"origem", req.Origem,
		"campanha", req.Campanha,
		"email_src", req.EmailSrc,
		"name", req.Name,
	)

	smtpErr := s.SendEmail(&req)

	var status, smtpError string
	if smtpErr != nil {
		status = "error"
		smtpError = smtpErr.Error()
		s.logger.Error("falha ao enviar email", "error", smtpErr)
	} else {
		status = "sent"
		s.logger.Info("email enviado com sucesso")
	}

	id, dbErr := s.SaveEmail(r.Context(), &req, status, smtpError)
	if dbErr != nil {
		s.logger.Error("falha ao salvar no banco", "error", dbErr)
	}

	if status != "sent" {
		writeJSON(w, http.StatusBadGateway, EmailResponse{
			Success: false,
			Message: fmt.Sprintf("falha ao enviar email: %s", smtpError),
			ID:      id,
		})
		return
	}

	writeJSON(w, http.StatusOK, EmailResponse{
		Success: true,
		Message: "email enviado com sucesso",
		ID:      id,
	})
}

func (s *EmailService) HandleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbOK := true
	if err := s.db.Ping(ctx); err != nil {
		dbOK = false
	}

	status := "ok"
	if !dbOK {
		status = "degraded"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   status,
		"database": dbOK,
		"smtp":     s.config.SMTPHost,
		"time":     time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *EmailService) HandleListEmails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, EmailResponse{Success: false, Message: "use GET"})
		return
	}

	rows, err := s.db.Query(r.Context(), `
		SELECT id, origem, campanha, email_src, name, assunto, status, created_at
		FROM emails ORDER BY created_at DESC LIMIT 100
	`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, EmailResponse{Success: false, Message: err.Error()})
		return
	}
	defer rows.Close()

	type emailRow struct {
		ID        int64     `json:"id"`
		Origem    string    `json:"origem"`
		Campanha  string    `json:"campanha"`
		EmailSrc  string    `json:"email_src"`
		Name      string    `json:"name"`
		Assunto   string    `json:"assunto"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
	}

	var results []emailRow
	for rows.Next() {
		var row emailRow
		if err := rows.Scan(&row.ID, &row.Origem, &row.Campanha, &row.EmailSrc, &row.Name, &row.Assunto, &row.Status, &row.CreatedAt); err != nil {
			continue
		}
		results = append(results, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{"emails": results, "count": len(results)})
}

// ============================================================================
// Helpers
// ============================================================================

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// ============================================================================
// Main
// ============================================================================

func main() {
	cfg := LoadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if cfg.SMTPPass == "" {
		logger.Error("SMTP_PASS não configurado - gere um App Password no Gmail")
		os.Exit(1)
	}

	// Conecta ao PostgreSQL
	ctx := context.Background()
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Error("falha ao parsear DATABASE_URL", "error", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2

	db, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		logger.Error("falha ao conectar no banco", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		logger.Error("banco inacessível", "error", err)
		os.Exit(1)
	}
	logger.Info("conectado ao PostgreSQL")

	svc := NewEmailService(cfg, db)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/send", svc.HandleSendEmail)
	mux.HandleFunc("/api/v1/emails", svc.HandleListEmails)
	mux.HandleFunc("/health", svc.HandleHealth)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("servidor iniciado", "port", cfg.Port, "smtp", cfg.SMTPHost)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("erro no servidor", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("desligando servidor...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
	logger.Info("servidor finalizado")
}
