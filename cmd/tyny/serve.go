package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/cli"
	"github.com/spf13/cobra"

	"github.com/glrf/tyny"
	"github.com/glrf/tyny/api"
	"github.com/glrf/tyny/store/mem"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

type serveConf struct {
	Admin  serveAdminConf
	Public servePublicConf
}

type servePublicConf struct {
	Addr string
}

type serveAdminConf struct {
	Grpc serveAdminGrpcConf
	Rest serveAdminRestConf
}
type serveAdminGrpcConf struct {
	Addr string
}
type serveAdminRestConf struct {
	Enabled bool
	Addr    string
	Path    string
}

type serveOpt struct {
	log log.Interface

	grpcServer   *grpc.Server
	grpcListener net.Listener

	restServer *http.Server
	httpServer *http.Server
}

func newCmdServe(conf *config) *cobra.Command {
	opt := &serveOpt{
		log: log.WithFields(log.Fields{}),
	}
	serveCmd := &cobra.Command{
		Use:    "serve",
		Short:  "Start the redirector",
		PreRun: handleErr(opt.init(conf)),
		Run:    handleErr(opt.run),
	}
	serveCmd.PersistentFlags().StringVarP(&conf.Serve.Public.Addr, "addr", "", conf.Serve.Public.Addr, "Address to listen on")

	serveCmd.PersistentFlags().StringVarP(&conf.Serve.Admin.Grpc.Addr, "grpc-addr", "", conf.Serve.Admin.Grpc.Addr, "Address for the grpc API")

	serveCmd.PersistentFlags().BoolVarP(&conf.Serve.Admin.Rest.Enabled, "rest", "", conf.Serve.Admin.Rest.Enabled, "Enable rest API")
	serveCmd.PersistentFlags().StringVarP(&conf.Serve.Admin.Rest.Addr, "rest-addr", "", conf.Serve.Admin.Rest.Addr, "Address for the rest API")
	serveCmd.PersistentFlags().StringVarP(&conf.Serve.Admin.Rest.Path, "rest-path", "", conf.Serve.Admin.Rest.Path, "Path for the rest API")

	log.SetHandler(cli.New(serveCmd.OutOrStderr()))

	return serveCmd
}

func (opt *serveOpt) init(conf *config) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		var err error
		opt.log.Debug("setting up sever")

		// Setup storage
		db := mem.New()
		s := tyny.Server{
			Store: db,
			Log:   opt.log,
		}

		// Setup redicter
		mux := http.NewServeMux()
		mux.Handle("/", s)
		opt.httpServer = &http.Server{
			Addr:    conf.Serve.Public.Addr,
			Handler: mux,
		}

		// Setup grpc api
		grpcAddrSlice := strings.Split(conf.Serve.Admin.Grpc.Addr, ":")
		if len(grpcAddrSlice) != 2 {
			return fmt.Errorf("grpc address invalid")
		}
		grpcPort := grpcAddrSlice[1]
		opt.grpcServer = grpc.NewServer(grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			grpc_recovery.UnaryServerInterceptor(),
		)))
		api.RegisterTynyServer(opt.grpcServer, s)
		opt.grpcListener, err = net.Listen("tcp", conf.Serve.Admin.Grpc.Addr)
		if err != nil {
			return err
		}

		// Setup rest api
		if conf.Serve.Admin.Rest.Enabled {
			gw := runtime.NewServeMux()
			opts := []grpc.DialOption{grpc.WithInsecure()}
			err = api.RegisterTynyHandlerFromEndpoint(cmd.Context(), gw, fmt.Sprintf("localhost:%s", grpcPort), opts)
			if err != nil {
				return err
			}
			path := conf.Serve.Admin.Rest.Path
			if !strings.HasSuffix(path, "/") {
				path = path + "/"
			}
			if conf.Serve.Admin.Rest.Addr == conf.Serve.Public.Addr {
				mux.Handle(path, http.StripPrefix(strings.TrimSuffix(path, "/"), gw))
			} else {
				gwmux := http.NewServeMux()

				gwmux.Handle(path, http.StripPrefix(strings.TrimSuffix(path, "/"), gw))
				opt.restServer = &http.Server{
					Addr:    conf.Serve.Admin.Rest.Addr,
					Handler: gwmux,
				}
			}
		}

		return nil
	}
}

func (opt *serveOpt) run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	go func() {
		opt.log.Info("starting grpc API")
		if err := opt.grpcServer.Serve(opt.grpcListener); err != nil {
			opt.log.WithError(err).Fatal("grpc server failed")
		}
	}()
	if opt.restServer != nil {
		go func() {
			opt.log.Info("starting rest API")
			if err := opt.restServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				opt.log.WithError(err).Fatal("rest server failed")
			}
		}()
	}
	go func() {
		opt.log.Info("starting server")
		if err := opt.httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			opt.log.WithError(err).Fatal("http server failed")
		}
	}()

	<-ctx.Done()
	opt.log.Warn("terminating..")
	termCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := opt.httpServer.Shutdown(termCtx); err != nil {
		opt.log.Error("failed to shutdown http server")
	}
	opt.log.Warn("http sever terminated")

	if opt.restServer != nil {
		if err := opt.restServer.Shutdown(termCtx); err != nil {
			opt.log.Error("failed to shutdown rest server")
		}
		opt.log.Warn("rest sever terminated")
	}

	c := make(chan struct{}, 1)
	go func() {
		opt.grpcServer.GracefulStop()
		c <- struct{}{}
	}()
	select {
	case <-c:
		opt.log.Warn("grpc sever terminated")
	case <-termCtx.Done():
		opt.log.Error("grpc sever termination timed out")
	}

	return nil
}
