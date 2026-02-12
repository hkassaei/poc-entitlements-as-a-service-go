package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestSubscriber defines a subscriber for seeding.
type TestSubscriber struct {
	IMSI string
	Ki   string // hex
	Op   string // hex
}

// EntitlementSeed defines an entitlement for seeding.
type EntitlementSeed struct {
	IMSI       string
	AppID      string
	Status     int
	ProvStatus int
	TcStatus   int
	ConfigData map[string]interface{}
}

// DefaultTestSubscribers returns the 3GPP TS 35.207 test subscribers.
var DefaultTestSubscribers = []TestSubscriber{
	{IMSI: "001010000000001", Ki: "465b5ce8b199b49faa5f0a2ee238a6bc", Op: "cdc202d5123e20f62b6d676ac72cb318"},
	{IMSI: "001010000000002", Ki: "0396eb317b6d1c36f19c1c84cd6ffd16", Op: "ff53bade17df5d4e793073ce9d7579fa"},
}

// SeedSubscribers seeds test subscribers into the database.
func SeedSubscribers(ctx context.Context, pool *pgxpool.Pool, kek []byte, subscribers []TestSubscriber) error {
	for _, sub := range subscribers {
		kiPlain, err := hex.DecodeString(sub.Ki)
		if err != nil {
			return fmt.Errorf("decode Ki hex: %w", err)
		}
		opPlain, err := hex.DecodeString(sub.Op)
		if err != nil {
			return fmt.Errorf("decode Op hex: %w", err)
		}

		// Generate a random DEK
		dek := make([]byte, 32)
		if _, err := rand.Read(dek); err != nil {
			return fmt.Errorf("generate DEK: %w", err)
		}

		// Encrypt Ki and OP with the DEK
		kiEncrypted, err := crypto.EncryptWithDEK(dek, kiPlain)
		if err != nil {
			return fmt.Errorf("encrypt Ki: %w", err)
		}
		opEncrypted, err := crypto.EncryptWithDEK(dek, opPlain)
		if err != nil {
			return fmt.Errorf("encrypt OP: %w", err)
		}

		// Wrap DEK with KEK
		dekWrapped, err := crypto.WrapDEK(kek, dek)
		if err != nil {
			return fmt.Errorf("wrap DEK: %w", err)
		}

		// Zero plaintext keys
		crypto.ZeroSlice(dek)
		crypto.ZeroSlice(kiPlain)
		crypto.ZeroSlice(opPlain)

		// Upsert subscriber
		_, err = pool.Exec(ctx, `
			INSERT INTO subscribers (imsi, ki_encrypted, op_encrypted, ki_dek_wrapped, sqn)
			VALUES ($1, $2, $3, $4, 0)
			ON CONFLICT (imsi) DO UPDATE SET
				ki_encrypted = EXCLUDED.ki_encrypted,
				op_encrypted = EXCLUDED.op_encrypted,
				ki_dek_wrapped = EXCLUDED.ki_dek_wrapped,
				updated_at = NOW()
		`, sub.IMSI, kiEncrypted, opEncrypted, dekWrapped)
		if err != nil {
			return fmt.Errorf("upsert subscriber %s: %w", sub.IMSI, err)
		}

		slog.Info("seeded subscriber", "imsi", sub.IMSI)
	}
	return nil
}

// DefaultEntitlementSeeds returns the default entitlement seed data.
func DefaultEntitlementSeeds() []EntitlementSeed {
	return []EntitlementSeed{
		// Alice (001010000000001)
		{IMSI: "001010000000001", AppID: "ap2004", Status: 1, ProvStatus: 3, TcStatus: 1, ConfigData: map[string]interface{}{
			"addresses": []map[string]string{{"addrType": "1", "addr": "epdg.operator.com"}, {"addrType": "1", "addr": "pcscf.operator.com"}},
		}},
		{IMSI: "001010000000001", AppID: "ap2003", Status: 1, ProvStatus: 3, TcStatus: 1, ConfigData: map[string]interface{}{
			"addresses": []map[string]string{{"addrType": "1", "addr": "pcscf.operator.com"}}, "volteEntitled": "1", "vonrEntitled": "1",
		}},
		{IMSI: "001010000000001", AppID: "ap2005", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"addresses": []map[string]string{{"addrType": "1", "addr": "smsc.operator.com"}},
		}},
		{IMSI: "001010000000001", AppID: "ap2006", Status: 1, ProvStatus: 3, TcStatus: 1, ConfigData: map[string]interface{}{
			"subscriptionState": "active", "smdpAddress": "smdp.operator.com", "profileType": "companion",
		}},
		{IMSI: "001010000000001", AppID: "ap2009", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"subscriptionState": "eligible", "serviceFlowUrl": "https://operator.com/plans/select", "planId": "PLAN-UNLIMITED-001", "planName": "Unlimited Plus",
		}},
		{IMSI: "001010000000001", AppID: "ap2010", Status: 1, ProvStatus: 3, TcStatus: 1, ConfigData: map[string]interface{}{
			"planName": "Unlimited Plus", "planId": "PLAN-UNLIMITED-001", "dataAllowanceBytes": 107374182400, "dataUsedBytes": 21474836480,
			"billingCycleEnd": "2026-03-01", "accessType": "5G", "dataType": "metered", "boostEligible": true, "serviceFlowUrl": "https://operator.com/plans/boost",
		}},
		{IMSI: "001010000000001", AppID: "ap2011", Status: 1, ProvStatus: 3, TcStatus: 1, ConfigData: map[string]interface{}{
			"enterpriseId": "ENT-001", "smdpAddress": "smdp.operator.com", "profileType": "default", "subscriptionState": "active",
		}},
		{IMSI: "001010000000001", AppID: "ap2012", Status: 1, ProvStatus: 0, TcStatus: 1, ConfigData: map[string]interface{}{}},
		{IMSI: "001010000000001", AppID: "ap2013", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"pseudonym": "anon-alice-7x9k2", "identityType": "PSEUDONYM",
		}},
		{IMSI: "001010000000001", AppID: "ap2014", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"msisdn": "+15551234567", "displayName": "Alice", "homeCarrier": "Test Operator",
		}},
		{IMSI: "001010000000001", AppID: "ap2015", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"operatorTokenUrl": "https://auth.operator.com/token", "appTokenScope": "carrier.entitlement",
		}},
		{IMSI: "001010000000001", AppID: "ap2016", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"plmnAllow": []string{"00101", "00102"}, "plmnBarred": []string{"99999"}, "serviceConstraints": "sos-only",
		}},

		// Bob (001010000000002)
		{IMSI: "001010000000002", AppID: "ap2004", Status: 0, ProvStatus: 1, TcStatus: 2, ConfigData: map[string]interface{}{
			"serviceFlowUrl": "https://operator.com/terms/vowifi",
		}},
		{IMSI: "001010000000002", AppID: "ap2003", Status: 1, ProvStatus: 3, TcStatus: 1, ConfigData: map[string]interface{}{
			"addresses": []map[string]string{{"addrType": "1", "addr": "pcscf.operator.com"}}, "volteEntitled": "1", "vonrEntitled": "0",
		}},
		{IMSI: "001010000000002", AppID: "ap2006", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"subscriptionState": "eligible", "serviceFlowUrl": "https://operator.com/companion/setup",
		}},
		{IMSI: "001010000000002", AppID: "ap2009", Status: 1, ProvStatus: 3, TcStatus: 1, ConfigData: map[string]interface{}{
			"subscriptionState": "active", "smdpAddress": "smdp.operator.com", "profileType": "default",
		}},
		{IMSI: "001010000000002", AppID: "ap2010", Status: 0, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{}},
		{IMSI: "001010000000002", AppID: "ap2011", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"enterpriseId": "ENT-002", "subscriptionState": "eligible", "serviceFlowUrl": "https://operator.com/enterprise/setup",
		}},
		{IMSI: "001010000000002", AppID: "ap2012", Status: 0, ProvStatus: 0, TcStatus: 2, ConfigData: map[string]interface{}{
			"serviceFlowUrl": "https://operator.com/terms/dcb",
		}},
		{IMSI: "001010000000002", AppID: "ap2013", Status: 0, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{}},
		{IMSI: "001010000000002", AppID: "ap2014", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"msisdn": "+15559876543", "displayName": "Bob", "homeCarrier": "Test Operator",
		}},
		{IMSI: "001010000000002", AppID: "ap2015", Status: 1, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{
			"operatorTokenUrl": "https://auth.operator.com/token", "appTokenScope": "carrier.basic",
		}},
		{IMSI: "001010000000002", AppID: "ap2016", Status: 0, ProvStatus: 0, TcStatus: 0, ConfigData: map[string]interface{}{}},
	}
}

// SeedEntitlements seeds entitlement records for test subscribers.
func SeedEntitlements(ctx context.Context, pool *pgxpool.Pool, seeds []EntitlementSeed) error {
	for _, entry := range seeds {
		// Look up subscriber ID by IMSI
		var subscriberID string
		err := pool.QueryRow(ctx, `SELECT id FROM subscribers WHERE imsi = $1`, entry.IMSI).Scan(&subscriberID)
		if err != nil {
			slog.Warn("subscriber not found, skipping", "imsi", entry.IMSI)
			continue
		}

		configJSON, err := json.Marshal(entry.ConfigData)
		if err != nil {
			return fmt.Errorf("marshal config data: %w", err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO entitlements (subscriber_id, app_id, status, prov_status, tc_status, config_data)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (subscriber_id, app_id) DO UPDATE SET
				status = EXCLUDED.status,
				prov_status = EXCLUDED.prov_status,
				tc_status = EXCLUDED.tc_status,
				config_data = EXCLUDED.config_data,
				updated_at = NOW()
		`, subscriberID, entry.AppID, entry.Status, entry.ProvStatus, entry.TcStatus, configJSON)
		if err != nil {
			return fmt.Errorf("upsert entitlement %s/%s: %w", entry.IMSI, entry.AppID, err)
		}

		slog.Info("seeded entitlement", "imsi", entry.IMSI, "appId", entry.AppID, "status", entry.Status)
	}
	return nil
}
