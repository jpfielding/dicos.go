package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jpfielding/dicos.go/pkg/logging"
	cmd "github.com/jpfielding/dicos.go/cmd/ctl/cmd"
)

var (
	GitSHA string = "NA"
)

func main() {
	// register sigterm for graceful shutdown
	ctx, cnc := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cnc()
	go func() {
		defer cnc() // this cnc is from notify and removes the signal so subsequent ctrl-c will restore kill functions
		<-ctx.Done()
	}()
	slog.SetDefault(logging.Logger(os.Stdout, false, slog.LevelInfo))
	ctx = logging.AppendCtx(ctx,
		slog.Group("prosight",
			slog.String("name", "ctl"),
			slog.String("git", GitSHA),
		))
	cmd.NewRoot(ctx, GitSHA).Execute()
}
