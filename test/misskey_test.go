package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/compose"
)

func TestMisskey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	stack, err := compose.NewDockerCompose("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			dumpLogs(t, stack)
		}
		if err := stack.Down(context.Background(), compose.RemoveOrphans(true), compose.RemoveVolumes(true)); err != nil {
			t.Error(err)
		}
		_ = stack.Close()
	})

	// マイグレーション完了とhealthcheck通過まで待機
	if err := stack.Up(ctx, compose.Wait(true)); err != nil {
		t.Fatal(err)
	}

	app, err := stack.ServiceContainer(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := app.PortEndpoint(ctx, "3000/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}

	var account struct {
		Token string `json:"token"`
	}
	callAPI(ctx, t, baseURL, "admin/accounts/create", map[string]any{
		"username": "test",
		"password": "test-password",
	}, &account)

	var me struct {
		Username string `json:"username"`
	}
	callAPI(ctx, t, baseURL, "i", map[string]any{"i": account.Token}, &me)
	if me.Username != "test" {
		t.Errorf("username = %q, want %q", me.Username, "test")
	}

	noteText := fmt.Sprintf("test note %d", time.Now().UnixNano())
	var created struct {
		CreatedNote struct {
			ID string `json:"id"`
		} `json:"createdNote"`
	}
	callAPI(ctx, t, baseURL, "notes/create", map[string]any{
		"i":    account.Token,
		"text": noteText,
	}, &created)

	var note struct {
		Text string `json:"text"`
	}
	callAPI(ctx, t, baseURL, "notes/show", map[string]any{
		"i":      account.Token,
		"noteId": created.CreatedNote.ID,
	}, &note)
	if note.Text != noteText {
		t.Errorf("note text = %q, want %q", note.Text, noteText)
	}
}

func callAPI(ctx context.Context, t *testing.T, baseURL, name string, body map[string]any, out any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/"+name, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s returned %s: %s", name, resp.Status, respBody)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		t.Fatalf("%s: failed to decode response: %v", name, err)
	}
}

func dumpLogs(t *testing.T, stack *compose.DockerCompose) {
	t.Helper()
	ctx := context.Background()
	for _, svc := range stack.Services() {
		c, err := stack.ServiceContainer(ctx, svc)
		if err != nil {
			t.Logf("%s: %v", svc, err)
			continue
		}
		r, err := c.Logs(ctx)
		if err != nil {
			t.Logf("%s: %v", svc, err)
			continue
		}
		b, _ := io.ReadAll(r)
		r.Close()
		t.Logf("=== %s logs ===\n%s", svc, b)
	}
}
