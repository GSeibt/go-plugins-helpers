package sdk

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

const activatePath = "/Plugin.Activate"

// serverCloser is an io.Closer whose Close method shuts down the referenced http.Server using its ShutDown method.
// The channel 'err' receives the error produced by the Serve method of the server. This error is returned by the
// Close method.
type serverCloser struct {
	server *http.Server
	err    chan error
}

func (s serverCloser) Close() error {
	// Wait at most 10 seconds for the server.Shutdown method to return
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return err
	}

	return <-s.err
}

// specDeleter extends serverCloser to additionally delete the spec file.
type specDeleter struct {
	serverCloser io.Closer
	spec         string
}

func (s specDeleter) Close() error {
	err := s.serverCloser.Close()

	if s.spec != "" {
		os.Remove(s.spec)
	}

	return err
}

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

// Serve sets up the Handler to asynchronously serve requests using the passed in Listener.
// Calling the Close method of the returned Closer will gracefully shut down the server answering requests
// and return the resulting error (see http.Server#Serve(net.Listener)).
func (h Handler) Serve(l net.Listener) io.Closer {
	srv := http.Server{
		Addr:    l.Addr().String(),
		Handler: h.mux,
	}

	// Transport error returned by http.Server#Serve
	srvErr := make(chan error, 1)

	closer := serverCloser{
		server: &srv,
		err:    srvErr,
	}

	go func() {
		srvErr <- srv.Serve(l)
	}()

	return closer
}

// ServeTCP sets up the Handler to asynchronously listen for requests to a TCP address.
// It also writes the spec file in the right directory for docker to read.
// Calling the Close method of the returned Closer will delete the spec file, gracefully shut down the
// server answering requests and return the resulting error (see http.Server#Serve(net.Listener)).
//
// Due to constrains for running Docker in Docker on Windows, data-root directory
// of docker daemon must be provided. To get default directory, use
// WindowsDefaultDaemonRootDir() function. On Unix, this parameter is ignored.
func (h Handler) ServeTCP(pluginName, addr, daemonDir string, tlsConfig *tls.Config) (io.Closer, error) {
	l, spec, err := newTCPListener(addr, pluginName, daemonDir, tlsConfig)
	if err != nil {
		return nil, err
	}

	closer := h.Serve(l)
	deleter := specDeleter{
		serverCloser: closer,
		spec:         spec,
	}

	return deleter, nil
}

// ServeUnix sets up the Handler to asynchronously listen for requests to a unix socket.
// It also creates the socket file in the right directory for docker to read.
// Calling the Close method of the returned Closer will delete the spec file, gracefully shut down the
// server answering requests and return the resulting error (see http.Server#Serve(net.Listener)).
func (h Handler) ServeUnix(addr string, gid int) (io.Closer, error) {
	l, spec, err := newUnixListener(addr, gid)
	if err != nil {
		return nil, err
	}

	closer := h.Serve(l)
	deleter := specDeleter{
		serverCloser: closer,
		spec:         spec,
	}

	return deleter, nil
}

// ServeWindows sets up the Handler to asynchronously listen for requests to a Windows named pipe.
// It also creates the spec file in the right directory for docker to read.
// Calling the Close method of the returned Closer will delete the spec file, gracefully shut down the
// server answering requests and return the resulting error (see http.Server#Serve(net.Listener)).
//
// Due to constrains for running Docker in Docker on Windows, data-root directory
// of docker daemon must be provided. To get default directory, use
// WindowsDefaultDaemonRootDir() function. On Unix, this parameter is ignored.
func (h Handler) ServeWindows(addr, pluginName, daemonDir string, pipeConfig *WindowsPipeConfig) (io.Closer, error) {
	l, spec, err := newWindowsListener(addr, pluginName, daemonDir, pipeConfig)
	if err != nil {
		return nil, err
	}

	closer := h.Serve(l)
	deleter := specDeleter{
		serverCloser: closer,
		spec:         spec,
	}

	return deleter, nil
}

// HandleFunc registers a function to handle a request path with.
func (h Handler) HandleFunc(path string, fn func(w http.ResponseWriter, r *http.Request)) {
	h.mux.HandleFunc(path, fn)
}
