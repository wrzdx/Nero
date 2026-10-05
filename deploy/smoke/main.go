// A deployment smoke check. Creates two temporary accounts and an empty direct
// chat, removes the test message, and anonymizes the accounts through the API.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
)

type account struct {
	password string
	User     struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
	Token string `json:"access_token"`
}

func checkPublicPages(ctx context.Context, base string) error {
	assets := make(map[string]bool)
	assetURL := regexp.MustCompile(`(?:href|src)="(/static/(?:css/app\.css|js/(?:auth|app|participants)\.js)\?v=[0-9a-f]{16})"`)
	for _, path := range []string{"/", "/login", "/register"} {
		r, err := http.NewRequestWithContext(ctx, "GET", base+path, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 || !strings.Contains(string(body), "Nero") {
			return fmt.Errorf("UI %s: expected Nero HTML, got %d", path, resp.StatusCode)
		}
		matches := assetURL.FindAllStringSubmatch(string(body), -1)
		if len(matches) != 4 {
			return fmt.Errorf("UI %s: missing versioned CSS/JS", path)
		}
		for _, match := range matches {
			assets[match[1]] = true
		}
	}
	for path := range assets {
		r, err := http.NewRequestWithContext(ctx, "GET", base+path, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("asset %s: got %d", path, resp.StatusCode)
		}
		hash := sha256.Sum256(body)
		if !strings.HasSuffix(path, fmt.Sprintf("?v=%x", hash[:8])) {
			return fmt.Errorf("asset %s: content hash does not match HTML", path)
		}
	}
	fmt.Println("homepage, login, registration and versioned CSS/JS: OK")
	return nil
}

func checkBrowserSession(ctx context.Context, base string, a account) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	form := url.Values{"username": {a.User.Username}, "password": {a.password}}
	r, err := http.NewRequestWithContext(ctx, "POST", base+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", base)
	resp, err := client.Do(r)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 || !strings.Contains(string(body), `id="app-root"`) {
		return fmt.Errorf("HTML login: expected authenticated app, got %d", resp.StatusCode)
	}
	for path, marker := range map[string]string{"/app/profile": "profile-content", "/app/new?type=group": "participant-picker"} {
		r, err := http.NewRequestWithContext(ctx, "GET", base+path, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(r)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 || !strings.Contains(string(body), marker) {
			return fmt.Errorf("HTML %s: expected authenticated view, got %d", path, resp.StatusCode)
		}
	}
	fmt.Println("HTML login cookies, profile and group picker: OK")
	return nil
}

func request(ctx context.Context, base, method, path, token string, body any, expected int, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, method, base+"/api/v1"+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expected {
		return fmt.Errorf("%s %s: expected %d, got %d", method, path, expected, resp.StatusCode)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	return json.Unmarshal(envelope.Data, out)
}

func run(base string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := checkPublicPages(ctx, base); err != nil {
		return err
	}
	var accounts []account
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		for _, a := range accounts {
			if err := request(cleanup, base, "DELETE", "/users/me", a.Token, nil, 204, nil); err != nil {
				fmt.Fprintln(os.Stderr, "account cleanup:", err)
			}
		}
	}()
	for range 2 {
		var a account
		password := uuid.NewString()
		err := request(ctx, base, "POST", "/auth/register", "", map[string]string{
			"username":   "smoke_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16],
			"first_name": "Deployment check", "password": password,
		}, 201, &a)
		if err != nil {
			return err
		}
		a.password = password
		accounts = append(accounts, a)
	}
	fmt.Println("registration: OK (two accounts)")
	if err := checkBrowserSession(ctx, base, accounts[0]); err != nil {
		return err
	}
	var search struct {
		Users []struct {
			Username string `json:"username"`
		} `json:"users"`
	}
	if err := request(ctx, base, "GET", "/users/search?q="+url.QueryEscape(accounts[1].User.Username), accounts[0].Token, nil, 200, &search); err != nil {
		return err
	}
	if len(search.Users) == 0 || search.Users[0].Username != accounts[1].User.Username {
		return fmt.Errorf("username search did not find the second account")
	}
	fmt.Println("username search: OK")
	wsURL := "ws" + strings.TrimPrefix(base, "http") + "/api/v1/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{base}}})
	if err != nil {
		return fmt.Errorf("websocket handshake: %w", err)
	}
	defer conn.CloseNow()
	if err = wsjson.Write(ctx, conn, map[string]string{"type": "authenticate", "access_token": accounts[1].Token}); err != nil {
		return err
	}
	var event struct {
		Type string `json:"type"`
		Data struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"data"`
	}
	if err = wsjson.Read(ctx, conn, &event); err != nil {
		return err
	}
	if event.Type != "authenticated" {
		return fmt.Errorf("unexpected auth response: %s", event.Type)
	}
	fmt.Println("WebSocket upgrade and authentication: OK")
	var chat struct {
		ID string `json:"id"`
	}
	if err = request(ctx, base, "POST", "/chats/directs", accounts[0].Token, map[string]string{"peer_username": accounts[1].User.Username}, 201, &chat); err != nil {
		return err
	}
	path := "/chats/" + chat.ID + "/messages"
	var msg struct {
		ID string `json:"id"`
	}
	if err = request(ctx, base, "POST", path+"/", accounts[0].Token, map[string]string{"client_message_id": uuid.NewString(), "content": "Deployment smoke check"}, 201, &msg); err != nil {
		return err
	}
	deleted := false
	defer func() {
		if !deleted {
			cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			if err := request(cleanup, base, "DELETE", path+"/"+msg.ID, accounts[0].Token, nil, 204, nil); err != nil {
				fmt.Fprintln(os.Stderr, "message cleanup:", err)
			}
		}
	}()
	for _, kind := range []string{"message_created", "message_edited", "message_deleted"} {
		switch kind {
		case "message_edited":
			err = request(ctx, base, "PATCH", path+"/"+msg.ID, accounts[0].Token, map[string]string{"content": "Updated deployment check"}, 200, nil)
		case "message_deleted":
			err = request(ctx, base, "PUT", path+"/read", accounts[1].Token, map[string]string{"message_id": msg.ID}, 204, nil)
			if err == nil {
				err = request(ctx, base, "DELETE", path+"/"+msg.ID, accounts[0].Token, nil, 204, nil)
				deleted = err == nil
			}
		}
		if err != nil {
			return err
		}
		if err = wsjson.Read(ctx, conn, &event); err != nil {
			return err
		}
		if event.Type != kind || event.Data.ID != msg.ID {
			return fmt.Errorf("unexpected event while waiting for %s", kind)
		}
		fmt.Println(kind + ": OK")
	}
	var page struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err = request(ctx, base, "GET", path+"/", accounts[1].Token, nil, 200, &page); err != nil {
		return err
	}
	if len(page.Messages) != 0 {
		return fmt.Errorf("deleted message remains in history")
	}
	fmt.Println("read marker and empty history after deletion: OK")
	return nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: smoke https://nero.example.com")
		os.Exit(2)
	}
	if err := run(strings.TrimRight(os.Args[1], "/")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
