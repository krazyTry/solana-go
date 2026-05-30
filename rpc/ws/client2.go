package ws

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func Connect2WithOptions(
	ctx context.Context,
	rpcEndpoint string,
	dialContext func(ctx context.Context, network, addr string) (net.Conn, error),
	dialTLSContext func(ctx context.Context, network, addr string) (net.Conn, error),
	opt *Options,
) (c *Client, err error) {
	c = &Client{
		rpcURL:                  rpcEndpoint,
		subscriptionByRequestID: map[uint64]*Subscription{},
		subscriptionByWSSubID:   map[uint64]*Subscription{},
	}

	dialer := &websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  DefaultHandshakeTimeout,
		NetDialContext:    dialContext,
		NetDialTLSContext: dialTLSContext,
		EnableCompression: true,
	}

	if opt != nil && opt.ShortID {
		c.shortID = opt.ShortID
	}

	if opt != nil && opt.HandshakeTimeout > 0 {
		dialer.HandshakeTimeout = opt.HandshakeTimeout
	}

	var httpHeader http.Header = nil
	if opt != nil && opt.HttpHeader != nil && len(opt.HttpHeader) > 0 {
		httpHeader = opt.HttpHeader
	}
	var resp *http.Response
	c.conn, resp, err = dialer.DialContext(ctx, rpcEndpoint, httpHeader)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			err = fmt.Errorf("new ws client: dial: %w, status: %s, body: %q", err, resp.Status, string(body))
		} else {
			err = fmt.Errorf("new ws client: dial: %w", err)
		}
		return nil, err
	}

	c.connCtx, c.connCtxCancel = context.WithCancel(context.Background())
	c.wg.Add(2)
	go func() {
		defer c.wg.Done()
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-c.connCtx.Done():
				return
			case <-ticker.C:
				c.sendPing()
			}
		}
	}()
	go c.receiveMessages()
	return c, nil
}
