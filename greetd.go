package main

// greetd IPC 客户端。
//
// 协议：通过 $GREETD_SOCK 这个 Unix socket 通信，每条消息 = 4 字节长度（本机字节序）+ JSON。
// 流程：create_session -> (auth_message -> post_auth_message_response)* -> start_session
// 失败时发 cancel_session 后重新来过。

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

var ErrAuth = errors.New("authentication failed")

type createSession struct {
	Type     string `json:"type"`
	Username string `json:"username"`
}

type authResponse struct {
	Type     string  `json:"type"`
	Response *string `json:"response,omitempty"`
}

type startSession struct {
	Type string   `json:"type"`
	Cmd  []string `json:"cmd"`
	Env  []string `json:"env"`
}

type cancelSession struct {
	Type string `json:"type"`
}

type reply struct {
	Type            string `json:"type"`
	ErrorType       string `json:"error_type"`
	Description     string `json:"description"`
	AuthMessageType string `json:"auth_message_type"`
	AuthMessage     string `json:"auth_message"`
}

func (r *reply) toError() error {
	if r.Type != "error" {
		return nil
	}
	if r.ErrorType == "auth_error" {
		return ErrAuth
	}
	return fmt.Errorf("greetd: %s", r.Description)
}

func roundTrip(conn net.Conn, req any) (*reply, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 4+len(payload))
	binary.NativeEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)

	// PAM 失败后常有 2 秒左右的延迟，给足超时。
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Write(buf); err != nil {
		return nil, err
	}

	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.NativeEndian.Uint32(hdr[:])
	if n > 1<<20 {
		return nil, fmt.Errorf("greetd reply too large: %d bytes", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}

	var r reply
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func cancel(conn net.Conn) {
	_, _ = roundTrip(conn, cancelSession{Type: "cancel_session"})
}

// login 完成一次完整的认证并启动会话。成功返回 nil，
// 之后 greeter 应当立刻退出，由 greetd 去拉起会话。
func login(user, pass string, cmd, env []string) error {
	sock := os.Getenv("GREETD_SOCK")
	if sock == "" {
		return errors.New("GREETD_SOCK is not set")
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return err
	}
	defer conn.Close()

	if env == nil {
		env = []string{}
	}

	r, err := roundTrip(conn, createSession{Type: "create_session", Username: user})
	if err != nil {
		return err
	}

	answered := false
	for r.Type == "auth_message" {
		var answer *string
		switch r.AuthMessageType {
		case "secret", "visible":
			if answered {
				cancel(conn)
				return fmt.Errorf("unsupported extra prompt: %s", r.AuthMessage)
			}
			answered = true
			answer = &pass
		}
		// info / error 类消息不需要回答，回一个空响应即可。
		if r, err = roundTrip(conn, authResponse{Type: "post_auth_message_response", Response: answer}); err != nil {
			return err
		}
	}

	if e := r.toError(); e != nil {
		cancel(conn)
		return e
	}
	if r.Type != "success" {
		cancel(conn)
		return fmt.Errorf("unexpected reply %q", r.Type)
	}

	r, err = roundTrip(conn, startSession{Type: "start_session", Cmd: cmd, Env: env})
	if err != nil {
		return err
	}
	if e := r.toError(); e != nil {
		cancel(conn)
		return e
	}
	return nil
}
