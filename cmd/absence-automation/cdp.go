package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type cdpResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type cdpClient struct {
	conn    *websocket.Conn
	nextID  atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan cdpResponse
}

func waitForWebSocket(ctx context.Context, port int) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/json", port)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		resp, err := client.Get(url)
		if err == nil {
			var tabs []struct {
				Type                 string `json:"type"`
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&tabs)
			_ = resp.Body.Close()
			if decodeErr == nil {
				for _, tab := range tabs {
					if tab.Type == "page" && tab.WebSocketDebuggerURL != "" {
						return tab.WebSocketDebuggerURL, nil
					}
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func newCDPClient(ctx context.Context, wsURL string) (*cdpClient, error) {
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	client := &cdpClient{conn: conn, pending: make(map[int64]chan cdpResponse)}
	go client.readLoop()
	return client, nil
}

func (c *cdpClient) Close() {
	_ = c.conn.Close(websocket.StatusNormalClosure, "done")
}

func (c *cdpClient) readLoop() {
	for {
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
		var response cdpResponse
		if json.Unmarshal(data, &response) != nil || response.ID == 0 {
			continue
		}
		c.mu.Lock()
		ch := c.pending[response.ID]
		delete(c.pending, response.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- response
		}
	}
}

func (c *cdpClient) call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	ch := make(chan cdpResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	message := map[string]any{"id": id, "method": method}
	if params != nil {
		message["params"] = params
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if err := c.conn.Write(ctx, websocket.MessageText, payload); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case response := <-ch:
		if response.Error != nil {
			return fmt.Errorf("CDP %s: %s", method, response.Error.Message)
		}
		if out != nil && len(response.Result) > 0 {
			return json.Unmarshal(response.Result, out)
		}
		return nil
	}
}

func evaluateBool(ctx context.Context, client *cdpClient, expression string) (bool, error) {
	var result struct {
		Result struct {
			Type  string `json:"type"`
			Value bool   `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := client.call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}, &result); err != nil {
		return false, err
	}
	if len(result.ExceptionDetails) > 0 && string(result.ExceptionDetails) != "null" {
		return false, errors.New("javascript evaluation failed")
	}
	if result.Result.Type != "boolean" {
		return false, fmt.Errorf("javascript returned %s instead of boolean", result.Result.Type)
	}
	return result.Result.Value, nil
}
