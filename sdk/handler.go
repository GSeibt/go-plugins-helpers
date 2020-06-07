package sdk

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const activatePath = "/Plugin.Activate"

// Handler is the base to create plugin handlers.
// It initializes connections and sockets to listen to.
type Handler struct {
	mux *http.ServeMux
}

// NewHandler creates a new Handler with an http mux.
func NewHandler(manifest string) Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(activatePath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", DefaultContentTypeV1_1)
		fmt.Fprintln(w, manifest)
	})

	return Handler{mux: mux}
}

// Serve sets up the handler to serve requests on the passed in listener
func (h Handler) Serve(l net.Listener) error {

	// Listen for SIGINT and SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Transport error returned by http.Server#Serve
	srvErr := make(chan error, 1)

	srv := http.Server{
		Addr:    l.Addr().String(),
		Handler: h.mux,
	}

	// Handle requests concurrently
	go func() {
		srvErr <- srv.Serve(l)
	}()

	// Wait for an OS termination signal
	<-sigs

	// Wait at most 10 seconds for the srv.Shutdown method to return
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return err
	} else {
		return <-srvErr
	}
}

// ServeTCP makes the handler to listen for request in a given TCP address.
// It also writes the spec file in the right directory for docker to read.
// Due to constrains for running Docker in Docker on Windows, data-root directory
// of docker daemon must be provided. To get default directory, use
// WindowsDefaultDaemonRootDir() function. On Unix, this parameter is ignored.
func (h Handler) ServeTCP(pluginName, addr, daemonDir string, tlsConfig *tls.Config) error {
	l, spec, err := newTCPListener(addr, pluginName, daemonDir, tlsConfig)
	if err != nil {
		return err
	}
	if spec != "" {
		defer os.Remove(spec)
	}
	return h.Serve(l)
}

// ServeUnix makes the handler to listen for requests in a unix socket.
// It also creates the socket file in the right directory for docker to read.
func (h Handler) ServeUnix(addr string, gid int) error {
	l, spec, err := newUnixListener(addr, gid)
	if err != nil {
		return err
	}
	if spec != "" {
		defer os.Remove(spec)
	}
	return h.Serve(l)
}

// ServeWindows makes the handler to listen for request in a Windows named pipe.
// It also creates the spec file in the right directory for docker to read.
// Due to constrains for running Docker in Docker on Windows, data-root directory
// of docker daemon must be provided. To get default directory, use
// WindowsDefaultDaemonRootDir() function. On Unix, this parameter is ignored.
func (h Handler) ServeWindows(addr, pluginName, daemonDir string, pipeConfig *WindowsPipeConfig) error {
	l, spec, err := newWindowsListener(addr, pluginName, daemonDir, pipeConfig)
	if err != nil {
		return err
	}
	if spec != "" {
		defer os.Remove(spec)
	}
	return h.Serve(l)
}

// HandleFunc registers a function to handle a request path with.
func (h Handler) HandleFunc(path string, fn func(w http.ResponseWriter, r *http.Request)) {
	h.mux.HandleFunc(path, fn)
}
