// Package wa wraps go.mau.fi/whatsmeow with the subset of behaviour cogriaclaw
// needs: open a SQLite-backed session store, run QR login when no auth exists,
// dispatch inbound messages, and send text replies. Everything else stays in
// whatsmeow.
package wa

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type Config struct {
	DataDir    string // session DB lives here; created if missing
	DeviceName string // shown in WhatsApp's Linked Devices list (currently informational only)
	LogLevel   string // whatsmeow's internal log level: DEBUG/INFO/WARN/ERROR
}

// MessageHandler is called for each inbound message whatsmeow surfaces.
// It runs on whatsmeow's event goroutine — kick long work to its own goroutine.
type MessageHandler func(ctx context.Context, msg InboundMessage)

type Client struct {
	cfg     Config
	wm      *whatsmeow.Client
	store   *sqlstore.Container
	handler MessageHandler
	dedup   *dedup
}

func New(cfg Config) (*Client, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("wa: DataDir is required")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("wa: mkdir data dir: %w", err)
	}
	logLevel := cfg.LogLevel
	if logLevel == "" {
		logLevel = "INFO"
	}

	dbPath := filepath.Join(cfg.DataDir, "whatsmeow.db")
	// modernc.org/sqlite DSN pragma form: ?_pragma=key(value). whatsmeow's
	// sqlstore aborts the schema upgrade if foreign_keys isn't enabled.
	dsn := "file:" + dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	dbLog := waLog.Stdout("DB", logLevel, true)
	store, err := sqlstore.New(context.Background(), "sqlite", dsn, dbLog)
	if err != nil {
		return nil, fmt.Errorf("wa: open store: %w", err)
	}

	device, err := store.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("wa: get device: %w", err)
	}

	clientLog := waLog.Stdout("WA", logLevel, true)
	wm := whatsmeow.NewClient(device, clientLog)

	return &Client{cfg: cfg, wm: wm, store: store, dedup: newDedup(4096)}, nil
}

// Start runs the QR login flow (if no saved session), connects, and dispatches
// inbound messages to handler until ctx is cancelled.
func (c *Client) Start(ctx context.Context, handler MessageHandler) error {
	c.handler = handler

	c.wm.AddEventHandler(func(evt any) {
		if m, ok := evt.(*events.Message); ok {
			c.dispatchMessage(ctx, m)
		}
	})

	if c.wm.Store.ID == nil {
		if err := c.qrLogin(ctx); err != nil {
			return err
		}
	} else if err := c.wm.Connect(); err != nil {
		return fmt.Errorf("wa: connect: %w", err)
	}

	<-ctx.Done()
	c.wm.Disconnect()
	return ctx.Err()
}

// SendText sends a plain text message to to (a JID — group or DM).
func (c *Client) SendText(ctx context.Context, to types.JID, text string) error {
	_, err := c.wm.SendMessage(ctx, to, &waE2E.Message{
		Conversation: proto.String(text),
	})
	return err
}

// HasSession reports whether a paired WhatsApp session already exists in the
// given data dir, without connecting. Used by `install` to decide between
// prompting a QR login and reporting "already running".
func HasSession(dataDir string) bool {
	dbPath := filepath.Join(dataDir, "whatsmeow.db")
	if _, err := os.Stat(dbPath); err != nil {
		return false
	}
	store, err := sqlstore.New(context.Background(), "sqlite",
		"file:"+dbPath+"?_pragma=foreign_keys(1)", waLog.Noop)
	if err != nil {
		return false
	}
	device, err := store.GetFirstDevice(context.Background())
	if err != nil {
		return false
	}
	return device.ID != nil
}

// IsConnected reports whether the underlying socket is currently connected.
func (c *Client) IsConnected() bool { return c.wm.IsConnected() }

// Self returns the bot's own phone-number JID (Store.ID), or zero before login.
func (c *Client) Self() types.JID {
	if c.wm.Store == nil || c.wm.Store.ID == nil {
		return types.JID{}
	}
	return c.wm.Store.ID.ToNonAD()
}

// ParseTarget turns a user-supplied target string into a JID:
//   - "...@g.us"            → group JID
//   - "+447700900123" / digits / "447...@s.whatsapp.net" → user JID
func ParseTarget(s string) (types.JID, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return types.JID{}, errors.New("empty target")
	}
	if strings.Contains(t, "@") {
		return types.ParseJID(t)
	}
	digits := strings.Builder{}
	for _, r := range t {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	if digits.Len() == 0 {
		return types.JID{}, fmt.Errorf("not a phone number or JID: %q", s)
	}
	return types.NewJID(digits.String(), types.DefaultUserServer), nil
}

func (c *Client) qrLogin(ctx context.Context) error {
	qrChan, err := c.wm.GetQRChannel(ctx)
	if err != nil {
		return fmt.Errorf("wa: qr channel: %w", err)
	}
	if err := c.wm.Connect(); err != nil {
		return fmt.Errorf("wa: connect: %w", err)
	}
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			PrintQR(os.Stderr, evt.Code)
		case "success":
			fmt.Fprintln(os.Stderr, "[wa] logged in")
		case "timeout":
			return errors.New("wa: QR login timed out — restart to retry")
		case "err-client-outdated":
			return errors.New("wa: client outdated — upgrade whatsmeow")
		}
	}
	return nil
}

func (c *Client) dispatchMessage(ctx context.Context, evt *events.Message) {
	if c.handler == nil {
		return
	}
	msg := extractInbound(evt, c.selfJIDs())
	if msg.Text == "" {
		return // ignore receipts, reactions, non-text
	}
	if c.dedup.seenBefore(msg.ID) {
		return // redelivered message (e.g. post-reconnect) — already handled
	}
	c.handler(ctx, msg)
}

// selfJIDs returns the bot's own JIDs (PN and LID), zero values omitted.
// Used for "@me" detection across addressing modes.
func (c *Client) selfJIDs() []types.JID {
	var out []types.JID
	if c.wm.Store.ID != nil && !c.wm.Store.ID.IsEmpty() {
		out = append(out, *c.wm.Store.ID)
	}
	if !c.wm.Store.LID.IsEmpty() {
		out = append(out, c.wm.Store.LID)
	}
	return out
}
