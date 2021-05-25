package main

import (
	"context"
	"errors"
	"os/signal"
	"syscall"

	"github.com/apex/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type config struct {
	Verbosity int
	// Server config
	Serve serveConf
}

func handleErr(f func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
    if err := f(cmd, args); err != nil {
      log.Fatal(err.Error())
    }
	}
}

func main() {
	conf := &config{
		Serve: serveConf{
			Admin: serveAdminConf{
				Grpc: serveAdminGrpcConf{
					Addr: ":3333",
				},
				Rest: serveAdminRestConf{
					Enabled: false,
					Addr:    ":8008",
					Path:    "/api/",
				},
			},
			Public: servePublicConf{
				Addr: ":8080",
			},
		},
		Verbosity: 0,
	}

	configFile := ""

	var rootCmd = &cobra.Command{
		Use:   "glrf",
		Short: "glrf is a light weight url shortener",
		PersistentPreRun: handleErr(func(cmd *cobra.Command, args []string) error {
			// Get config file
			viper.SetConfigType("yaml")

			viper.SetConfigName("config")
			viper.AddConfigPath("/etc/tyny")
			viper.AddConfigPath("$HOME/.config/tyny")
			viper.AddConfigPath("$HOME/.tyny")
			if configFile != "" {
				viper.SetConfigFile(configFile)
			}
			err := viper.ReadInConfig() // Find and read the config file
			if err != nil && !errors.As(err, &viper.ConfigFileNotFoundError{}) {
				return err
			}
			if err != nil && errors.As(err, &viper.ConfigFileNotFoundError{}) && configFile != "" {
				return err
			}
			if err == nil {
				err = viper.UnmarshalExact(conf)
				if err != nil {
					return err
				}
			}

			switch conf.Verbosity {
			case 0:
				log.SetLevel(log.WarnLevel)
			case 1:
				log.SetLevel(log.InfoLevel)
			default:
				log.SetLevel(log.DebugLevel)
			}
			return nil
		}),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	rootCmd.PersistentFlags().CountVarP(&conf.Verbosity, "verbose", "v", "verbosity")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file location")
	rootCmd.AddCommand(newCmdServe(conf))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	rootCmd.ExecuteContext(ctx)
}
