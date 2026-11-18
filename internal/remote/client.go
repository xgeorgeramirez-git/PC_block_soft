package remote

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MaxResponse — ответ сервера читается не больше 64 КБ (протокол, раздел 7).
const MaxResponse = 64 << 10

// Машинные коды состояния связи (LinkStatus.LastErr); тексты делает окно.
const (
	StatusNoServer     = "no_server"    // сеть, тайм-аут, 5xx
	StatusBadAnswer    = "bad_answer"   // ответ больше 64 КБ, не JSON, перенаправление, прочие 4xx
	StatusBadAddress   = "bad_address"  // 404: по адресу не сервер связи
	StatusTooMany      = "too_many"     // 429: ограничение сервера
	StatusUnauthorized = "unauthorized" // 401: сервер не узнал компьютер
)

var (
	// ErrTooLarge — ответ больше MaxResponse.
	ErrTooLarge = errors.New("remote: ответ сервера слишком большой")
	// ErrBadAnswer — ответ не разобран.
	ErrBadAnswer = errors.New("remote: неверный ответ сервера")
	// ErrBadURL — адрес сервера не подходит.
	ErrBadURL = errors.New("remote: неверный адрес сервера")
)

// StatusError — ответ сервера не 200: код HTTP и поле error.
type StatusError struct {
	Status int
	Code   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("remote: сервер ответил %d %s", e.Status, e.Code)
}

// errCode — машинный код ошибки запроса для LinkStatus ("" — ошибки нет).
func errCode(err error) string {
	if err == nil {
		return ""
	}
	var se *StatusError
	if errors.As(err, &se) {
		switch {
		case se.Status == http.StatusUnauthorized:
			return StatusUnauthorized
		case se.Status == http.StatusTooManyRequests:
			return StatusTooMany
		case se.Status == http.StatusNotFound:
			return StatusBadAddress
		case se.Status >= 500:
			return StatusNoServer
		}
		return StatusBadAnswer
	}
	if errors.Is(err, ErrTooLarge) || errors.Is(err, ErrBadAnswer) {
		return StatusBadAnswer
	}
	return StatusNoServer
}

func isUnauthorized(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == http.StatusUnauthorized
}

// ValidateURL проверяет адрес сервера: https с именем хоста; http — только 127.0.0.1 и
// localhost и только при allowLoopbackHTTP (тестовый режим службы). Без логина в адресе,
// запроса и якоря; конечный «/» убирается.
func ValidateURL(raw string, allowLoopbackHTTP bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return nil, ErrBadURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := u.Hostname()
	switch u.Scheme {
	case "https":
		if host == "" {
			return nil, ErrBadURL
		}
	case "http":
		if !allowLoopbackHTTP || (host != "127.0.0.1" && !strings.EqualFold(host, "localhost")) {
			return nil, ErrBadURL
		}
	default:
		return nil, ErrBadURL
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u, nil
}

// newTransport — свой транспорт: без прокси из окружения (Proxy: nil), с тайм-аутами.
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 40 * time.Second,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
		ForceAttemptHTTP2:     true,
	}
}

// noRedirect — перенаправления не выполняются: связь только с адресом из настроек.
func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// Client — запросы компьютера к серверу связи (протокол, раздел 3). Тайм-ауты — контекстом.
type Client struct {
	Base  *url.URL
	HTTP  *http.Client
	Token string
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	if c.Base == nil {
		return ErrBadURL
	}
	u := c.Base.JoinPath(path)
	if query != nil {
		u.RawQuery = query.Encode()
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Transport: newTransport(), CheckRedirect: noRedirect}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponse+1))
	if err != nil {
		return err
	}
	if len(raw) > MaxResponse {
		return ErrTooLarge
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return &StatusError{Status: resp.StatusCode, Code: e.Error}
	}
	if !isObject(raw) {
		return ErrBadAnswer
	}
	if out != nil && json.Unmarshal(raw, out) != nil {
		return ErrBadAnswer
	}
	return nil
}

// Register — POST /v1/pc/register.
func (c *Client) Register(ctx context.Context) (pcID, token string, err error) {
	var out struct {
		PCID    string `json:"pcId"`
		PCToken string `json:"pcToken"`
	}
	if err := c.do(ctx, http.MethodPost, "v1/pc/register", nil, struct{}{}, &out); err != nil {
		return "", "", err
	}
	if out.PCID == "" || out.PCToken == "" {
		return "", "", ErrBadAnswer
	}
	return out.PCID, out.PCToken, nil
}

// PairCode — POST /v1/pc/pair-code: шесть цифр (прежний код компьютера погашен).
func (c *Client) PairCode(ctx context.Context) (string, error) {
	var out struct {
		Code string `json:"code"`
	}
	if err := c.do(ctx, http.MethodPost, "v1/pc/pair-code", nil, struct{}{}, &out); err != nil {
		return "", err
	}
	if len(out.Code) != 6 || strings.Trim(out.Code, "0123456789") != "" {
		return "", ErrBadAnswer
	}
	return out.Code, nil
}

// Poll — GET /v1/pc/poll?wait=N: ждёт сообщений до wait (сервер — не больше 25 с).
func (c *Client) Poll(ctx context.Context, wait time.Duration) ([]Msg, error) {
	q := url.Values{"wait": {strconv.Itoa(int(wait / time.Second))}}
	var out struct {
		Messages []Msg `json:"messages"`
	}
	if err := c.do(ctx, http.MethodGet, "v1/pc/poll", q, nil, &out); err != nil {
		return nil, err
	}
	return out.Messages, nil
}

// Reply — POST /v1/pc/reply: ответ телефону на сообщение replyTo.
func (c *Client) Reply(ctx context.Context, to, replyTo, kind string, body any) error {
	in := map[string]any{"to": to, "replyTo": replyTo, "kind": kind, "body": body}
	return c.do(ctx, http.MethodPost, "v1/pc/reply", nil, in, nil)
}

// SendReport — POST /v1/pc/report.
func (c *Client) SendReport(ctx context.Context, s Signed) error {
	return c.do(ctx, http.MethodPost, "v1/pc/report", nil, map[string]any{"body": s}, nil)
}

// Unpair — POST /v1/pc/unpair: токен телефона погашен на сервере.
func (c *Client) Unpair(ctx context.Context, phoneID string) error {
	return c.do(ctx, http.MethodPost, "v1/pc/unpair", nil, map[string]string{"phoneId": phoneID}, nil)
}
