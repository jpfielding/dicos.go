package cmd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/jpfielding/dicos.go/pkg/dicos"
	"github.com/jpfielding/dicos.go/pkg/logging"
	"github.com/spf13/cobra"
)

func NewRoot(ctx context.Context, gitsha string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dicosctl",
		Short: "a CLI to manage clearscan configuration/validation",
		Long:  "the long story",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logLevel, _ := cmd.Flags().GetString("log-level")

			// Parse log level
			var level slog.Level
			if err := level.UnmarshalText([]byte(strings.ToUpper(logLevel))); err != nil {
				level = slog.LevelInfo
			}
			slog.SetDefault(logging.Logger(os.Stdout, false, level))

			if err := level.UnmarshalText([]byte(strings.ToUpper(logLevel))); err != nil {
				slog.WarnContext(ctx, "Invalid log level, defaulting to INFO", "level", logLevel, "error", err)
			}

		},
		Run: func(cmd *cobra.Command, args []string) {
			printCommandTree(cmd, 0)
		},
	}
	cmd.AddCommand(
		NewVersionCmd(ctx, gitsha),
		NewDecodeCmd(ctx),
		NewAnalyzeCmd(ctx),
	)
	pf := cmd.PersistentFlags()
	pf.String("log-level", "INFO", "Log level (DEBUG, INFO, WARN, ERROR)")
	return cmd
}

func printCommandTree(cmd *cobra.Command, indent int) {
	fmt.Println(strings.Repeat("\t", indent), cmd.Use+":", cmd.Short)
	for _, subCmd := range cmd.Commands() {
		printCommandTree(subCmd, indent+1)
	}
}

func NewVersionCmd(ctx context.Context, gitsha string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "git sha for this build",
		Long:  "git sha for this build",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(gitsha)
		},
	}
	return cmd
}

// NewToolsDICOSCmd is a command to dump DICOS certs
func NewDecodeCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decode",
		Short: "DICOS decode",
		Long:  "DICOS decode",
		RunE: func(cmd *cobra.Command, args []string) error {
			var in io.Reader
			dcsPath, _ := cmd.Flags().GetString("uri")
			dcsPath = strings.TrimPrefix(dcsPath, "file://")
			switch {
			case dcsPath == "-":
				in = os.Stdin
			case strings.HasPrefix(dcsPath, "http"):
				// TODO make this a param
				cl := &http.Client{
					Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, dcsPath, nil)
				if err != nil {
					return fmt.Errorf("failed to create request: %v", err)
				}
				resp, err := cl.Do(req)
				if err != nil {
					return fmt.Errorf("failed to download: %v", err)
				}
				verbose, _ := cmd.Flags().GetBool("verbose")
				if verbose {
					reqDump, _ := httputil.DumpRequest(req, true)
					os.Stderr.Write(reqDump)
					resDump, _ := httputil.DumpResponse(resp, false)
					os.Stderr.Write(resDump)
				}
				in = resp.Body
				defer resp.Body.Close()
			default:
				f, err := os.Open(dcsPath)
				if err != nil {
					return fmt.Errorf("failed to open file: %v", err)
				}
				in = f
				defer f.Close()
			}
			dataset, _ := dicos.Parse(in)
			switch uioType, _ := cmd.Flags().GetString("format"); uioType {
			case "text": // Dataset will nicely print the DICOM dataset data out of the box.
				fmt.Println(dataset)
			default: // Dataset is also JSON serializable out of the box.
				j, _ := json.Marshal(dataset)
				os.Stdout.Write(j)
			}
			return nil
		},
	}
	pf := cmd.PersistentFlags()
	pf.StringP("uri", "u", "", "DICOS URI to fetch certificates from")
	pf.StringP("format", "f", "json", "output format (text|json)")
	return cmd
}
