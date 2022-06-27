package dashboard

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/oursky/github-actions-manager/pkg/github/runners"
	"github.com/oursky/github-actions-manager/pkg/utils/defaults"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type RunnerState interface {
	State() *runners.State
}

type Server struct {
	logger *zap.Logger
	server *http.Server
	assets fs.FS

	state RunnerState
}

func NewServer(logger *zap.Logger, config *Config, state RunnerState) *Server {
	logger = logger.Named("dashboard")

	assets, _ := fs.Sub(assetsFS, "assets")
	if config.AssetsDir != nil {
		assets = os.DirFS(*config.AssetsDir)
	}

	mux := http.NewServeMux()
	server := &Server{
		logger: logger,
		server: &http.Server{
			Addr:         defaults.Value(config.Addr, "127.0.0.1:8000"),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			Handler:      mux,
			ErrorLog:     zap.NewStdLog(logger),
		},
		assets: assets,
		state:  state,
	}

	mux.HandleFunc("/", server.index)
	mux.HandleFunc("/styles.css", server.styles)

	return server
}

func (s *Server) Start(ctx context.Context, g *errgroup.Group) error {
	g.Go(func() error {
		go func() {
			<-ctx.Done()

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s.server.Shutdown(shutdownCtx)
		}()

		s.logger.Info("starting server", zap.String("addr", s.server.Addr))
		err := s.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed to run server: %w", err)
		}
		return nil
	})
	return nil
}

func (s *Server) asset(rw http.ResponseWriter, name string, contentType string) {
	file, err := s.assets.Open(name)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte(fmt.Sprintf("failed to load asset: %s", err)))
		s.logger.Error("failed to load asset", zap.Error(err))
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte(fmt.Sprintf("failed to load asset: %s", err)))
		s.logger.Error("failed to load asset", zap.Error(err))
		return
	}

	rw.Header().Add("Content-Type", contentType)
	rw.WriteHeader(http.StatusOK)
	rw.Write(data)
}

func (s *Server) template(rw http.ResponseWriter, tplName string, data any) {
	tpl, err := template.ParseFS(s.assets, tplName)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte(fmt.Sprintf("failed to load template: %s", err)))
		s.logger.Error("failed to load template", zap.Error(err))
		return
	}

	rw.Header().Add("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(http.StatusOK)
	if err := tpl.Execute(rw, data); err != nil {
		rw.Write([]byte(fmt.Sprintf("failed to execute template: %s", err)))
		s.logger.Error("failed to execute template", zap.Error(err))
	}
}