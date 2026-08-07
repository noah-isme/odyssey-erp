package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

var (
	ErrUnauthorized = errors.New("api: unauthorized")
	ErrForbidden    = errors.New("api: forbidden")
)

type Handler struct {
	pool          *pgxpool.Pool
	encryptionKey []byte
}
type apiKey struct {
	ID, CompanyID, CreatedBy int64
	Hash                     string
}

func NewHandler(pool *pgxpool.Pool, key ...[]byte) *Handler {
	var encryptionKey []byte
	if len(key) > 0 {
		encryptionKey = key[0]
	}
	return &Handler{pool: pool, encryptionKey: encryptionKey}
}

func (h *Handler) encryptSecret(secret string) (string, error) {
	if len(h.encryptionKey) == 0 {
		return "", errors.New("api: webhook encryption key is not configured")
	}
	key := sha256.Sum256(h.encryptionKey)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(gcm.Seal(nonce, nonce, []byte(secret), nil)), nil
}

func HashKey(key string) string { sum := sha256.Sum256([]byte(key)); return hex.EncodeToString(sum[:]) }

func GenerateKey() (plain, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	plain = "odk_" + hex.EncodeToString(buf)
	return plain, HashKey(plain), nil
}

func SignWebhook(secret string, payload []byte, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyWebhookSignature(secret string, payload []byte, timestamp, signature string) bool {
	want := SignWebhook(secret, payload, timestamp)
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(want)), []byte(strings.ToLower(signature))) == 1
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/openapi.json", h.openapiV1)
		r.Get("/me", h.me)
		r.Get("/projects", h.listProjects)
		r.Post("/projects", h.createProject)
		r.Post("/keys", h.createKey)
		r.Post("/keys/bootstrap", h.bootstrapKey)
		r.Post("/keys/{id}/revoke", h.revokeKey)
		r.Post("/webhooks", h.createWebhook)
		r.Get("/webhooks", h.listWebhooks)
		r.Post("/webhooks/{id}/replay", h.replayWebhook)
	})

	// Public API v2 with robust rate limiting (60 requests/sec, burst 100)
	r.Route("/api/v2", func(r chi.Router) {
		r.Use(RateLimitMiddleware(60, 100))
		
		r.Get("/openapi.json", h.openapiV2)
		
		r.Get("/me", h.me)
		r.Get("/projects", h.listProjects)
		r.Post("/projects", h.createProject)
		r.Post("/keys", h.createKey)
		r.Post("/keys/bootstrap", h.bootstrapKey)
		r.Post("/keys/{id}/revoke", h.revokeKey)
		r.Post("/webhooks", h.createWebhook)
		r.Get("/webhooks", h.listWebhooks)
		r.Post("/webhooks/{id}/replay", h.replayWebhook)
	})
}

func (h *Handler) openapiV1(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"openapi": "3.0.3", "info": map[string]string{"title": "Odyssey Public API", "version": "1.0.0"},
		"security":   []map[string][]string{{"apiKey": {}}},
		"components": map[string]any{"securitySchemes": map[string]any{"apiKey": map[string]string{"type": "apiKey", "in": "header", "name": "X-API-Key"}}},
		"paths": map[string]any{
			"/api/v1/me":       map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]string{"description": "Authenticated integration"}}}},
			"/api/v1/projects": map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]string{"description": "Company projects"}}}, "post": map[string]any{"parameters": []map[string]any{{"name": "Idempotency-Key", "in": "header", "required": true}}, "responses": map[string]any{"201": map[string]string{"description": "Created project"}}}},
			"/api/v1/keys":     map[string]any{"post": map[string]any{"responses": map[string]any{"201": map[string]string{"description": "Created API key; plaintext returned once"}}}},
			"/api/v1/webhooks": map[string]any{"post": map[string]any{"responses": map[string]any{"201": map[string]string{"description": "Created webhook subscription"}}}},
		},
	})
}

func (h *Handler) openapiV2(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"openapi": "3.1.0", 
		"info": map[string]string{
			"title": "Odyssey Public API v2", 
			"version": "2.0.0",
			"description": "Rate-limited Odyssey ERP API.",
		},
		"security":   []map[string][]string{{"apiKey": {}}},
		"components": map[string]any{"securitySchemes": map[string]any{"apiKey": map[string]string{"type": "apiKey", "in": "header", "name": "X-API-Key"}}},
		"paths": map[string]any{
			"/api/v2/me":       map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]string{"description": "Authenticated integration"}}}},
			"/api/v2/projects": map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]string{"description": "Company projects"}}}, "post": map[string]any{"parameters": []map[string]any{{"name": "Idempotency-Key", "in": "header", "required": true}}, "responses": map[string]any{"201": map[string]string{"description": "Created project"}}}},
			"/api/v2/keys":     map[string]any{"post": map[string]any{"responses": map[string]any{"201": map[string]string{"description": "Created API key; plaintext returned once"}}}},
			"/api/v2/webhooks": map[string]any{"post": map[string]any{"responses": map[string]any{"201": map[string]string{"description": "Created webhook subscription"}}}},
		},
	})
}

func (h *Handler) authenticate(r *http.Request, scope string) (apiKey, error) {
	if h == nil || h.pool == nil {
		return apiKey{}, ErrUnauthorized
	}
	plain := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if plain == "" {
		return apiKey{}, ErrUnauthorized
	}
	var key apiKey
	err := h.pool.QueryRow(r.Context(), `SELECT id, company_id, created_by, key_hash FROM api_keys WHERE key_hash=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > NOW())`, HashKey(plain)).Scan(&key.ID, &key.CompanyID, &key.CreatedBy, &key.Hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return apiKey{}, ErrUnauthorized
	}
	if err != nil {
		return apiKey{}, err
	}
	var allowed bool
	err = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM api_key_scopes WHERE api_key_id=$1 AND scope=$2)`, key.ID, scope).Scan(&allowed)
	if err != nil {
		return apiKey{}, err
	}
	if !allowed {
		return apiKey{}, ErrForbidden
	}
	return key, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func apiError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, ErrUnauthorized) {
		status = http.StatusUnauthorized
	}
	if errors.Is(err, ErrForbidden) {
		status = http.StatusForbidden
	}
	if strings.HasPrefix(err.Error(), "api: ") || errors.Is(err, pgx.ErrNoRows) {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": http.StatusText(status), "message": err.Error()}})
}

func (h *Handler) createKey(w http.ResponseWriter, r *http.Request) {
	key, err := h.authenticate(r, "api.manage")
	if err != nil {
		apiError(w, err)
		return
	}
	h.provisionKey(w, r, key.CompanyID, key.CreatedBy)
}

func (h *Handler) bootstrapKey(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		apiError(w, ErrUnauthorized)
		return
	}
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		apiError(w, ErrUnauthorized)
		return
	}
	userID, err := strconv.ParseInt(sess.User(), 10, 64)
	if err != nil || userID <= 0 {
		apiError(w, ErrUnauthorized)
		return
	}
	companyID, err := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	if err != nil || companyID <= 0 {
		apiError(w, ErrUnauthorized)
		return
	}
	var allowed bool
	err = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=$1 AND p.name='api.manage')`, userID).Scan(&allowed)
	if err != nil || !allowed {
		apiError(w, ErrForbidden)
		return
	}
	h.provisionKey(w, r, companyID, userID)
}

func (h *Handler) provisionKey(w http.ResponseWriter, r *http.Request, companyID, createdBy int64) {
	var in struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		apiError(w, errors.New("api: name is required"))
		return
	}
	plain, hash, err := GenerateKey()
	if err != nil {
		apiError(w, err)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var id int64
	if err = tx.QueryRow(r.Context(), `INSERT INTO api_keys(company_id,name,key_hash,expires_at,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id`, companyID, in.Name, hash, in.ExpiresAt, createdBy).Scan(&id); err != nil {
		apiError(w, err)
		return
	}
	for _, scope := range in.Scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO api_key_scopes(api_key_id,scope) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, scope); err != nil {
			apiError(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": in.Name, "key": plain, "scopes": in.Scopes, "warning": "The plaintext key is shown only once."})
}

func (h *Handler) revokeKey(w http.ResponseWriter, r *http.Request) {
	key, err := h.authenticate(r, "api.manage")
	if err != nil {
		apiError(w, err)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		apiError(w, errors.New("api: invalid key id"))
		return
	}
	result, err := h.pool.Exec(r.Context(), `UPDATE api_keys SET revoked_at=NOW() WHERE id=$1 AND company_id=$2 AND revoked_at IS NULL`, id, key.CompanyID)
	if err != nil {
		apiError(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		apiError(w, pgx.ErrNoRows)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "revoked": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	key, err := h.authenticate(r, "api.read")
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"company_id": key.CompanyID, "key_id": key.ID})
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	key, err := h.authenticate(r, "projects.read")
	if err != nil {
		apiError(w, err)
		return
	}
	limit, offset := 100, 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, e := strconv.Atoi(raw); e == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, e := strconv.Atoi(raw); e == nil && n >= 0 {
			offset = n
		}
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id, code, name, currency, status FROM projects WHERE company_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`, key.CompanyID, limit, offset)
	if err != nil {
		apiError(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID                           int64 `json:"id"`
		Code, Name, Currency, Status string
	}
	items := []item{}
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.ID, &x.Code, &x.Name, &x.Currency, &x.Status); err != nil {
			apiError(w, err)
			return
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "pagination": map[string]int{"limit": limit, "offset": offset, "count": len(items)}})
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	key, err := h.authenticate(r, "projects.write")
	if err != nil {
		apiError(w, err)
		return
	}
	idempotency := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotency == "" {
		apiError(w, errors.New("api: Idempotency-Key is required"))
		return
	}
	var input struct{ Code, Name, Currency string }
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		apiError(w, err)
		return
	}
	if input.Code == "" || input.Name == "" {
		apiError(w, errors.New("api: code and name are required"))
		return
	}
	if input.Currency == "" {
		input.Currency = "IDR"
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var claimed bool
	err = tx.QueryRow(r.Context(), `INSERT INTO horizon_idempotency_keys(company_id,actor_id,idempotency_key,operation,response) VALUES($1,$2,$3,'api.projects.create','{}'::jsonb) ON CONFLICT DO NOTHING RETURNING TRUE`, key.CompanyID, key.CreatedBy, idempotency).Scan(&claimed)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		apiError(w, err)
		return
	}
	if !claimed {
		var cached []byte
		if err = tx.QueryRow(r.Context(), `SELECT response FROM horizon_idempotency_keys WHERE company_id=$1 AND operation='api.projects.create' AND idempotency_key=$2`, key.CompanyID, idempotency).Scan(&cached); err != nil {
			apiError(w, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			apiError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cached)
		return
	}
	var project struct {
		ID                           int64 `json:"id"`
		Code, Name, Currency, Status string
	}
	err = tx.QueryRow(r.Context(), `INSERT INTO projects(company_id,code,name,currency,status,created_by) VALUES($1,$2,$3,$4,'OPEN',$5) RETURNING id,code,name,currency,status`, key.CompanyID, input.Code, input.Name, input.Currency, key.CreatedBy).Scan(&project.ID, &project.Code, &project.Name, &project.Currency, &project.Status)
	if err != nil {
		apiError(w, err)
		return
	}
	body, _ := json.Marshal(map[string]any{"data": project})
	_, err = tx.Exec(r.Context(), `UPDATE horizon_idempotency_keys SET response=$1 WHERE company_id=$2 AND operation='api.projects.create' AND idempotency_key=$3`, body, key.CompanyID, idempotency)
	if err != nil {
		apiError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		apiError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(body)
	// The source write has completed before the delivery row is created.
	_ = h.Publish(r.Context(), key.CompanyID, "project.created", project)
}

func (h *Handler) createWebhook(w http.ResponseWriter, r *http.Request) {
	key, err := h.authenticate(r, "webhooks.write")
	if err != nil {
		apiError(w, err)
		return
	}
	var input struct{ EventType, Endpoint, Secret string }
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		apiError(w, err)
		return
	}
	if input.EventType == "" || input.Endpoint == "" || input.Secret == "" {
		apiError(w, errors.New("api: event_type, endpoint, and secret are required"))
		return
	}
	endpoint, err := url.Parse(input.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
		apiError(w, errors.New("api: webhook endpoint must be an HTTPS URL"))
		return
	}
	secretHash := HashKey(input.Secret)
	secretCiphertext, err := h.encryptSecret(input.Secret)
	if err != nil {
		apiError(w, err)
		return
	}
	var id int64
	err = h.pool.QueryRow(r.Context(), `INSERT INTO webhook_subscriptions(company_id,event_type,endpoint,secret_hash,secret_ciphertext,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, key.CompanyID, input.EventType, input.Endpoint, secretHash, secretCiphertext, key.CreatedBy).Scan(&id)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "event_type": input.EventType, "endpoint": input.Endpoint, "created_at": time.Now().UTC().Format(time.RFC3339)})
}

func (h *Handler) listWebhooks(w http.ResponseWriter, r *http.Request) {
	key, err := h.authenticate(r, "webhooks.manage")
	if err != nil {
		apiError(w, err)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id,event_type,endpoint,active,created_at FROM webhook_subscriptions WHERE company_id=$1 ORDER BY id DESC LIMIT 100`, key.CompanyID)
	if err != nil {
		apiError(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID                  int64 `json:"id"`
		EventType, Endpoint string
		Active              bool
		CreatedAt           time.Time `json:"created_at"`
	}
	items := []item{}
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.ID, &x.EventType, &x.Endpoint, &x.Active, &x.CreatedAt); err != nil {
			apiError(w, err)
			return
		}
		items = append(items, x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) replayWebhook(w http.ResponseWriter, r *http.Request) {
	key, err := h.authenticate(r, "webhooks.manage")
	if err != nil {
		apiError(w, err)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		apiError(w, errors.New("api: invalid webhook id"))
		return
	}
	result, err := h.pool.Exec(r.Context(), `UPDATE webhook_deliveries d SET next_attempt_at=NOW(),dead_lettered_at=NULL WHERE d.subscription_id=$1 AND EXISTS(SELECT 1 FROM webhook_subscriptions s WHERE s.id=d.subscription_id AND s.company_id=$2) AND d.delivered_at IS NULL`, id, key.CompanyID)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription_id": id, "replayed": result.RowsAffected()})
}

func (h *Handler) String() string { return fmt.Sprintf("api handler %p", h) }
