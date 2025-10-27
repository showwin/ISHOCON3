package cmd

import (
	"github.com/spf13/cobra"

	"github.com/showwin/ISHOCON3/benchmark/bench"
)

var (
	targetURL string
	logLevel  string

	rootCmd = &cobra.Command{
		Use:   "bench",
		Short: "A benchmark tool for ISHOCON3",
		Run: func(cmd *cobra.Command, args []string) {
			bench.Run(targetURL, logLevel)
		},
	}
)

// Execute executes the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func init() {
	rootCmd.Flags().StringVar(&targetURL, "target", "http://localhost:8080", "target URL for benchmark")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
}
