package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/crypto"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/db"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := config.LoadConfig()
	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBPoolSize)
	if err != nil {
		slog.Error("Failed to connect to Postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	queries := db.NewQueries(pool)
	kekBytes, err := hex.DecodeString(cfg.LocalKEKHex)
	if err != nil {
		slog.Error("Failed to decode LOCAL_KEK_HEX", "err", err)
		os.Exit(1)
	}
	keyManager, err := crypto.CreateKeyManager(kekBytes, cfg.GCPProjectID, cfg.KMSLocation, cfg.KMSKeyRing, cfg.KMSKeyName)
	if err != nil {
		slog.Error("Failed to create key manager", "err", err)
		os.Exit(1)
	}

	r := chi.NewRouter()

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// POST /vectors — generate authentication vectors
	r.Post("/vectors", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IMSI string `json:"imsi"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		sub, err := queries.FindSubscriberByIMSI(r.Context(), req.IMSI)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Subscriber not found"})
			return
		}

		// Unwrap DEK and decrypt Ki/OP
		dek, err := keyManager.UnwrapDEK(r.Context(), sub.KiDEKWrapped)
		if err != nil {
			slog.Error("Failed to unwrap DEK", "err", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		defer crypto.ZeroSlice(dek)

		ki, err := crypto.DecryptWithDEK(dek, sub.KiEncrypted)
		if err != nil {
			slog.Error("Failed to decrypt Ki", "err", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		defer crypto.ZeroSlice(ki)

		op, err := crypto.DecryptWithDEK(dek, sub.OpEncrypted)
		if err != nil {
			slog.Error("Failed to decrypt OP", "err", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		defer crypto.ZeroSlice(op)

		// Build SQN (6 bytes, big-endian)
		sqnBuf := make([]byte, 6)
		binary.BigEndian.PutUint16(sqnBuf[0:2], 0)
		binary.BigEndian.PutUint32(sqnBuf[2:6], uint32(sub.SQN))

		// AMF: 0x8000 (separation bit set for LTE/5G)
		amf := []byte{0x80, 0x00}

		vectors, err := crypto.GenerateVectors(ki, op, sqnBuf, amf)
		if err != nil {
			slog.Error("Failed to generate vectors", "err", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		// Increment SQN
		_ = queries.UpdateSubscriberSQN(r.Context(), sub.IMSI, sub.SQN+1)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"rand": base64.StdEncoding.EncodeToString(vectors.RAND),
			"autn": base64.StdEncoding.EncodeToString(vectors.AUTN),
			"xres": base64.StdEncoding.EncodeToString(vectors.XRES),
			"ck":   base64.StdEncoding.EncodeToString(vectors.CK),
			"ik":   base64.StdEncoding.EncodeToString(vectors.IK),
		})
	})

	// POST /resync — resynchronize SQN after SYNC_FAILURE
	r.Post("/resync", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IMSI string `json:"imsi"`
			RAND string `json:"rand"`
			AUTS string `json:"auts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		randBytes, err := base64.StdEncoding.DecodeString(req.RAND)
		if err != nil || len(randBytes) != 16 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "RAND must be 16 bytes"})
			return
		}

		autsBytes, err := base64.StdEncoding.DecodeString(req.AUTS)
		if err != nil || len(autsBytes) != 14 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "AUTS must be 14 bytes"})
			return
		}

		sub, err := queries.FindSubscriberByIMSI(r.Context(), req.IMSI)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Subscriber not found"})
			return
		}

		dek, err := keyManager.UnwrapDEK(r.Context(), sub.KiDEKWrapped)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		defer crypto.ZeroSlice(dek)

		ki, err := crypto.DecryptWithDEK(dek, sub.KiEncrypted)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		defer crypto.ZeroSlice(ki)

		op, err := crypto.DecryptWithDEK(dek, sub.OpEncrypted)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		defer crypto.ZeroSlice(op)

		// Validate AUTS
		result := crypto.ValidateAUTS(ki, randBytes, autsBytes, op)
		if !result.Valid {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "AUTS validation failed"})
			return
		}

		// Extract SQN_MS and advance
		sqnMsValue := int64(binary.BigEndian.Uint16(result.SqnMs[0:2]))<<32 | int64(binary.BigEndian.Uint32(result.SqnMs[2:6]))
		newSQN := sqnMsValue + 32

		_ = queries.UpdateSubscriberSQN(r.Context(), sub.IMSI, newSQN)

		// Generate fresh vectors
		sqnBuf := make([]byte, 6)
		binary.BigEndian.PutUint16(sqnBuf[0:2], uint16(newSQN>>32))
		binary.BigEndian.PutUint32(sqnBuf[2:6], uint32(newSQN))

		amf := []byte{0x80, 0x00}
		vectors, err := crypto.GenerateVectors(ki, op, sqnBuf, amf)
		if err != nil {
			slog.Error("Failed to generate vectors", "err", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"rand": base64.StdEncoding.EncodeToString(vectors.RAND),
			"autn": base64.StdEncoding.EncodeToString(vectors.AUTN),
			"xres": base64.StdEncoding.EncodeToString(vectors.XRES),
			"ck":   base64.StdEncoding.EncodeToString(vectors.CK),
			"ik":   base64.StdEncoding.EncodeToString(vectors.IK),
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh
		slog.Info("Shutting down mock-hss")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("Mock HSS starting", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Mock HSS failed", "err", err)
		os.Exit(1)
	}
}
