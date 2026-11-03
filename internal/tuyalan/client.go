// Package tuyalan implements the small part of Tuya LAN 3.5 used by this service.
// Wire format reference: https://github.com/jasonacox/tinytuya/blob/master/PROTOCOL.md
package tuyalan

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"
)

const (
	commandStart   = 3
	commandRespond = 4
	commandFinish  = 5
	commandControl = 13
	commandQuery   = 16
	headerSize     = 18
	maxFrameSize   = 64 * 1024
)

var ErrProtocol = errors.New("invalid Tuya protocol response")
var ErrAuthentication = errors.New("Tuya authentication failed")
var ErrRejected = errors.New("device rejected the command")

type Client struct {
	conn net.Conn
	key  []byte
	seq  uint32
	stop func() bool
}

type message struct {
	command uint32
	code    uint32
	payload []byte
}

// Connect creates a new session for one HTTP operation. No state survives Close.
func Connect(ctx context.Context, address, localKey string) (*Client, error) {
	if len([]byte(localKey)) != 16 {
		return nil, ErrAuthentication
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(address, "6668"))
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, key: []byte(localKey), seq: 1}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(15 * time.Second)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, err
	}
	// Closing the socket also interrupts a blocked read when the caller disconnects.
	c.stop = context.AfterFunc(ctx, func() { conn.Close() })
	if err := c.handshake(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) Close() {
	if c.stop != nil {
		c.stop()
	}
	c.conn.Close()
	clear(c.key)
}

func (c *Client) handshake() error {
	localNonce := make([]byte, 16)
	if _, err := rand.Read(localNonce); err != nil {
		return err
	}
	if err := c.send(commandStart, localNonce); err != nil {
		return err
	}
	m, err := c.receive()
	if err != nil {
		return err
	}
	if m.command != commandRespond || m.code != 0 || len(m.payload) != 48 {
		return ErrAuthentication
	}
	mac := hmac.New(sha256.New, c.key)
	mac.Write(localNonce)
	if !hmac.Equal(mac.Sum(nil), m.payload[16:]) {
		return ErrAuthentication
	}
	remoteNonce := m.payload[:16]
	mac.Reset()
	mac.Write(remoteNonce)
	if err := c.send(commandFinish, mac.Sum(nil)); err != nil {
		return err
	}
	// Tuya 3.5 derives its 16-byte session key from the GCM ciphertext of
	// the XORed nonces. The authentication tag is not part of this key.
	mixed := make([]byte, 16)
	for i := range mixed {
		mixed[i] = localNonce[i] ^ remoteNonce[i]
	}
	gcm, err := newGCM(c.key)
	if err != nil {
		return err
	}
	derived := gcm.Seal(nil, localNonce[:12], mixed, nil)
	clear(c.key)
	c.key = append([]byte(nil), derived[:16]...)
	return nil
}

func (c *Client) Status() (map[string]json.RawMessage, error) {
	if err := c.send(commandQuery, []byte(`{}`)); err != nil {
		return nil, err
	}
	for {
		m, err := c.receive()
		if err != nil {
			return nil, err
		}
		// Device sequence numbers are global, not echoes of our sequence.
		// Ignore unsolicited reports and acknowledgements for other commands.
		if m.command != commandQuery {
			continue
		}
		if m.code != 0 {
			return nil, ErrRejected
		}
		payload := stripVersion(m.payload)
		if len(payload) == 0 {
			continue
		}
		var response struct {
			DPS  map[string]json.RawMessage `json:"dps"`
			Data struct {
				DPS map[string]json.RawMessage `json:"dps"`
			} `json:"data"`
		}
		if json.Unmarshal(payload, &response) != nil {
			return nil, ErrProtocol
		}
		if len(response.DPS) > 0 {
			return response.DPS, nil
		}
		if len(response.Data.DPS) > 0 {
			return response.Data.DPS, nil
		}
		return nil, ErrProtocol
	}
}

// Control never retries a write. A network error after sending may mean that
// the device applied the change but its acknowledgement was lost.
func (c *Client) Control(dps map[string]any) error {
	payload, err := json.Marshal(map[string]any{
		"protocol": 5,
		"t":        time.Now().Unix(),
		"data":     map[string]any{"dps": dps},
	})
	if err != nil {
		return err
	}
	versioned := append([]byte("3.5"), make([]byte, 12)...)
	versioned = append(versioned, payload...)
	if err := c.send(commandControl, versioned); err != nil {
		return err
	}
	for {
		m, err := c.receive()
		if err != nil {
			return err
		}
		if m.command != commandControl {
			continue
		}
		if m.code != 0 {
			return ErrRejected
		}
		return nil
	}
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrAuthentication
	}
	return cipher.NewGCM(block)
}

func (c *Client) send(command uint32, payload []byte) error {
	gcm, err := newGCM(c.key)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	header := make([]byte, headerSize)
	binary.BigEndian.PutUint32(header[0:4], 0x6699)
	binary.BigEndian.PutUint32(header[6:10], c.seq)
	binary.BigEndian.PutUint32(header[10:14], command)
	binary.BigEndian.PutUint32(header[14:18], uint32(len(nonce)+len(payload)+gcm.Overhead()))
	c.seq++
	frame := append(header, nonce...)
	frame = gcm.Seal(frame, nonce, payload, header[4:])
	frame = binary.BigEndian.AppendUint32(frame, 0x9966)
	for len(frame) > 0 {
		n, err := c.conn.Write(frame)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		frame = frame[n:]
	}
	return nil
}

func (c *Client) receive() (message, error) {
	var m message
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return m, err
	}
	if binary.BigEndian.Uint32(header[:4]) != 0x6699 {
		return m, ErrProtocol
	}
	length := binary.BigEndian.Uint32(header[14:18])
	// 12-byte nonce, at least a 4-byte return code, and 16-byte GCM tag.
	if length < 32 || length > maxFrameSize {
		return m, ErrProtocol
	}
	body := make([]byte, int(length)+4)
	if _, err := io.ReadFull(c.conn, body); err != nil {
		return m, err
	}
	if binary.BigEndian.Uint32(body[length:]) != 0x9966 {
		return m, ErrProtocol
	}
	gcm, err := newGCM(c.key)
	if err != nil {
		return m, err
	}
	plain, err := gcm.Open(nil, body[:12], body[12:length], header[4:])
	if err != nil {
		return m, ErrAuthentication
	}
	if len(plain) < 4 {
		return m, ErrProtocol
	}
	m.command = binary.BigEndian.Uint32(header[10:14])
	m.code = binary.BigEndian.Uint32(plain[:4])
	m.payload = plain[4:]
	return m, nil
}

func stripVersion(payload []byte) []byte {
	if bytes.HasPrefix(payload, []byte("3.5")) {
		if len(payload) < 15 {
			return nil
		}
		payload = payload[15:]
	}
	return bytes.TrimSpace(payload)
}

// SafeError categorizes failures without exposing payloads or cryptographic data.
func SafeError(err error) string {
	var netErr net.Error
	switch {
	case errors.Is(err, ErrAuthentication):
		return "device_authentication_failed"
	case errors.Is(err, ErrRejected):
		return "device_rejected_command"
	case errors.Is(err, ErrProtocol):
		return "invalid_device_response"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "device_timeout"
	default:
		return "device_unreachable"
	}
}
