package clickhouse

import (
	"context"
	"crypto/tls"
	"errors"
	"os"
	"strings"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/provider"
)

var ErrInvalidConfiguration = errors.New("invalid clickhouse configuration")

type Config struct {
	Addresses []string
	Database  string
	Username  string
	Password  string
	Secure    bool
}

type nativeInserter struct {
	conn driver.Conn
}

func ConfigFromEnv() (Config, error) {
	addresses := splitAddresses(os.Getenv("CLICKHOUSE_ADDR"))
	database := os.Getenv("CLICKHOUSE_DATABASE")
	username := os.Getenv("CLICKHOUSE_USERNAME")
	password := os.Getenv("CLICKHOUSE_PASSWORD")
	secure := strings.EqualFold(os.Getenv("CLICKHOUSE_SECURE"), "true")
	if len(addresses) == 0 || database == "" || username == "" || password == "" {
		return Config{}, ErrInvalidConfiguration
	}
	return Config{Addresses: addresses, Database: database, Username: username, Password: password, Secure: secure}, nil
}

func OpenFromEnv(ctx context.Context) (*Store, error) {
	config, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	var tlsConfig *tls.Config
	if config.Secure {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:         config.Addresses,
		Auth:         clickhouse.Auth{Database: config.Database, Username: config.Username, Password: config.Password},
		TLS:          tlsConfig,
		DialTimeout:  5 * time.Second,
		MaxOpenConns: 2,
		MaxIdleConns: 1,
	})
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, ErrStorageUnavailable
	}
	return New(&nativeInserter{conn: conn}, time.Now), nil
}

func splitAddresses(value string) []string {
	var addresses []string
	for _, address := range strings.Split(value, ",") {
		if address = strings.TrimSpace(address); address != "" {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

func (c *nativeInserter) InsertBatch(ctx context.Context, records []normalize.CostRecord, ingestedAt time.Time) error {
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO cost_records (provider, billing_account_id, source_record_id, cost_basis, amount, currency, usage_start, usage_end, version, ingested_at)")
	if err != nil {
		return err
	}
	defer batch.Close()
	for _, record := range records {
		if err := batch.Append(
			record.Provider,
			record.BillingAccountID,
			record.SourceRecordID,
			string(record.CostBasis),
			record.Amount,
			record.Currency,
			record.UsageStart.UTC(),
			record.UsageEnd.UTC(),
			uint64(ingestedAt.UTC().UnixNano()),
			ingestedAt.UTC(),
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

func (c *nativeInserter) ReadCosts(ctx context.Context, accounts []AccountScope, start, end time.Time) ([]normalize.CostRecord, error) {
	accountTuples := make([]string, len(accounts))
	args := make([]any, 0, 2+2*len(accounts))
	args = append(args, start.UTC(), end.UTC())
	for i, account := range accounts {
		accountTuples[i] = "(?, ?)"
		args = append(args, account.Provider, account.BillingAccountID)
	}
	query := `SELECT provider, billing_account_id, source_record_id, cost_basis, amount, currency, usage_start, usage_end
FROM cost_records FINAL
WHERE usage_start >= ? AND usage_start < ?
AND (provider, billing_account_id) IN (` + strings.Join(accountTuples, ", ") + ")"
	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]normalize.CostRecord, 0)
	for rows.Next() {
		var record normalize.CostRecord
		var costBasis string
		if err := rows.Scan(
			&record.Provider,
			&record.BillingAccountID,
			&record.SourceRecordID,
			&costBasis,
			&record.Amount,
			&record.Currency,
			&record.UsageStart,
			&record.UsageEnd,
		); err != nil {
			return nil, err
		}
		record.CostBasis = provider.CostBasis(costBasis)
		record.UsageStart = record.UsageStart.UTC()
		record.UsageEnd = record.UsageEnd.UTC()
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (c *nativeInserter) Close() error {
	return c.conn.Close()
}
