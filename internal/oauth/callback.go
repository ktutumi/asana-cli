package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
)

type CallbackResult struct {
	Code  string
	State string
}

type CallbackServer struct {
	URL    string
	server *http.Server
	result chan CallbackResult
	errs   chan error
}

func StartCallbackServer(ctx context.Context, host string, port int, path string) (*CallbackServer, error) {
	if host != "127.0.0.1" && host != "localhost" {
		return nil, fmt.Errorf("callback host must be localhost or 127.0.0.1")
	}
	if path == "" || path[0] != '/' {
		return nil, fmt.Errorf("callback path must start with /")
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	s := &CallbackServer{result: make(chan CallbackResult, 1), errs: make(chan error, 1)}
	s.URL = "http://" + ln.Addr().String() + path
	mux := http.NewServeMux()
	s.server = &http.Server{Handler: mux}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			select {
			case s.errs <- fmt.Errorf("missing code"):
			default:
			}
			return
		}
		_, _ = fmt.Fprintln(w, "Authentication complete. You may close this window.")
		select {
		case s.result <- CallbackResult{Code: code, State: r.URL.Query().Get("state")}:
		default:
		}
	})
	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			select {
			case s.errs <- err:
			default:
			}
		}
	}()
	go func() {
		<-ctx.Done()
		_ = s.server.Shutdown(context.Background())
		select {
		case s.errs <- ctx.Err():
		default:
		}
	}()
	return s, nil
}

func (s *CallbackServer) WaitForCode(ctx context.Context) (CallbackResult, error) {
	select {
	case r := <-s.result:
		return r, nil
	case err := <-s.errs:
		return CallbackResult{}, err
	case <-ctx.Done():
		return CallbackResult{}, ctx.Err()
	}
}

func (s *CallbackServer) Shutdown() { _ = s.server.Shutdown(context.Background()) }
