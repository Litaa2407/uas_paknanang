package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	eventExchange            = "crowdfunding.events"
	eventQueue               = "notification.events"
	outboxWorkerLockID int64 = 84202611
)

type rabbitPublisher struct {
	url      string
	mu       sync.Mutex
	conn     *amqp.Connection
	channel  *amqp.Channel
	confirms chan amqp.Confirmation
}

func ensureOutboxSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS outbox_events (
			id BIGSERIAL PRIMARY KEY,
			event_type VARCHAR(100) NOT NULL,
			payload JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			published_at TIMESTAMPTZ,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT
		)
	`)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, "CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox_events(id) WHERE published_at IS NULL")
	return err
}

func addOutboxEvent(ctx context.Context, tx pgx.Tx, eventType string, payload map[string]interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO outbox_events(event_type,payload) VALUES($1,$2::jsonb)", eventType, string(body))
	return err
}

func runOutboxPublisher(ctx context.Context, pool *pgxpool.Pool, rabbitURL string) {
	publisher := &rabbitPublisher{url: rabbitURL}
	defer publisher.Close()

	for ctx.Err() == nil {
		processed, err := publishNextOutboxEvent(ctx, pool, publisher)
		if err != nil {
			log.Printf("outbox publisher: %v", err)
			if !sleepContext(ctx, 3*time.Second) {
				return
			}
			continue
		}
		if !processed && !sleepContext(ctx, time.Second) {
			return
		}
	}
}

func publishNextOutboxEvent(ctx context.Context, pool *pgxpool.Pool, publisher *rabbitPublisher) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	// Hanya satu Campaign Service yang mem-publish outbox pada satu waktu.
	// Transaction-level advisory lock menjaga urutan event berdasarkan ID.
	var lockAcquired bool
	if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", outboxWorkerLockID).Scan(&lockAcquired); err != nil {
		return false, err
	}
	if !lockAcquired {
		return false, nil
	}

	var id int64
	var eventType string
	var payload []byte
	err = tx.QueryRow(ctx, `
		SELECT id,event_type,payload
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT 1
		FOR UPDATE
	`).Scan(&id, &eventType, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	var message map[string]json.RawMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		_, _ = tx.Exec(ctx, "UPDATE outbox_events SET attempts=attempts+1,last_error=$2 WHERE id=$1", id, "payload outbox bukan JSON valid")
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return true, commitErr
		}
		return true, err
	}
	idJSON, _ := json.Marshal(id)
	message["event_id"] = idJSON
	body, err := json.Marshal(message)
	if err != nil {
		return false, err
	}

	if err := publisher.Publish(ctx, eventType, id, body); err != nil {
		_, updateErr := tx.Exec(ctx, "UPDATE outbox_events SET attempts=attempts+1,last_error=$2 WHERE id=$1", id, err.Error())
		if updateErr != nil {
			return true, updateErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return true, commitErr
		}
		return true, err
	}

	_, err = tx.Exec(ctx, "UPDATE outbox_events SET published_at=NOW(),attempts=attempts+1,last_error=NULL WHERE id=$1", id)
	if err != nil {
		return true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return true, err
	}
	return true, nil
}

func (p *rabbitPublisher) ensureConnectedLocked() error {
	if p.conn != nil && !p.conn.IsClosed() && p.channel != nil && !p.channel.IsClosed() {
		return nil
	}
	p.closeLocked()

	conn, err := amqp.DialConfig(p.url, amqp.Config{Heartbeat: 10 * time.Second, Locale: "en_US", Dial: amqp.DefaultDial(5 * time.Second)})
	if err != nil {
		return err
	}
	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}
	if err := channel.ExchangeDeclare(eventExchange, "topic", true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return err
	}
	queue, err := channel.QueueDeclare(eventQueue, true, false, false, false, nil)
	if err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return err
	}
	if err := channel.QueueBind(queue.Name, "#", eventExchange, false, nil); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return err
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return err
	}

	p.conn = conn
	p.channel = channel
	p.confirms = channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	return nil
}

func (p *rabbitPublisher) Publish(ctx context.Context, eventType string, eventID int64, body []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureConnectedLocked(); err != nil {
		return err
	}
	if err := p.channel.PublishWithContext(ctx, eventExchange, eventType, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    strconv.FormatInt(eventID, 10),
		Type:         eventType,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	}); err != nil {
		p.closeLocked()
		return err
	}

	select {
	case confirmation, ok := <-p.confirms:
		if !ok || !confirmation.Ack {
			p.closeLocked()
			return errors.New("RabbitMQ tidak mengonfirmasi publikasi event")
		}
		return nil
	case <-ctx.Done():
		p.closeLocked()
		return ctx.Err()
	case <-time.After(5 * time.Second):
		p.closeLocked()
		return errors.New("timeout menunggu konfirmasi RabbitMQ")
	}
}

func (p *rabbitPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

func (p *rabbitPublisher) closeLocked() {
	if p.channel != nil {
		_ = p.channel.Close()
		p.channel = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
	p.confirms = nil
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
