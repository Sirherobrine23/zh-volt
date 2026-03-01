package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	zhvolt "sirherobrine23.com.br/Sirherobrine23/zh-volt/olt"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources/pcap"

	"github.com/urfave/cli/v3"
)

var app = &cli.Command{
	Name:  "zhvolt",
	Usage: "OLT monitor for Ainopol/Pacetech GPON OLTs",

	Commands: []*cli.Command{
		{
			Name: "daemon",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "interface",
					Aliases: []string{"i"},
					Value:   "eth0",
					Usage:   "Network interface to capture packets",
				},
				&cli.DurationFlag{
					Name:    "wait-dev-timeout",
					Aliases: []string{"w"},
					Value:   time.Second * 5,
				},
				&cli.Uint16Flag{
					Name:    "http-port",
					Aliases: []string{"H"},
					Value:   8081,
				},
				&cli.StringFlag{
					Name:    "log",
					Aliases: []string{"L"},
					Value:   "-",
				},
				&cli.Uint8Flag{
					Name:    "verbose-level",
					Aliases: []string{"v"},
					Value:   1,
					Validator: func(u uint8) error {
						if u > 0 && u <= 7 {
							return nil
						}
						return fmt.Errorf("verbose valid in range 1~7, u set %d", u)
					},
				},
			},

			Action: func(ctx context.Context, c *cli.Command) error {
				ifaceName := c.String("interface")
				pcapSource, err := pcap.New(ifaceName)
				if err != nil {
					return fmt.Errorf("cannot open pcap for %s: %s", ifaceName, err)
				}
				defer pcapSource.Close()

				var logPrint io.Writer = io.Discard
				if log := c.String("log"); log != "" {
					switch strings.ToLower(log) {
					case "-", "stdout", "out":
						logPrint = os.Stdout
					case "stderr", "err":
						logPrint = os.Stderr
					default:
						file, err := os.Create(log)
						if err != nil {
							return fmt.Errorf("cannot make log file: %s", err)
						}
						defer file.Close()
						fmt.Fprintf(os.Stderr, "Log file %q\n", file.Name())
						logPrint = file
					}
				}

				olts, err := zhvolt.NewOltProcess(ctx, pcapSource, logPrint)
				if err != nil {
					return fmt.Errorf("cannot create olt process: %s", err)
				}
				olts.Verbose = c.Uint8("verbose-level")

				if port := c.Uint16("http-port"); port > 0 {
					tcp, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
					if err != nil {
						return fmt.Errorf("cannot listen http: %s", err)
					}
					fmt.Fprintf(os.Stderr, "HTTP Listened on http://%s\n", tcp.Addr())
					defer tcp.Close()

					go http.Serve(tcp, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						data := olts.Olts()
						w.Header().Set("Content-Type", "application/json; utf-8")
						js := json.NewEncoder(w)
						js.SetIndent("", "  ")
						if err = js.Encode(data); err != nil {
							olts.Log.Printf("error on encode olt: %s\n", err)
							w.Header().Set("Content-Type", "text/plain; utf-8")
							w.WriteHeader(500)
							fmt.Fprintf(w, "error on encode olt data: %s\n\n", err)
						}
					}))
				}

				defer olts.Close()
				olts.Start()
				timeout := c.Duration("wait-dev-timeout")
				if !olts.Wait(timeout) {
					return fmt.Errorf("timeout (%s) to wait firt device!", timeout)
				}

				<-ctx.Done()
				return err
			},
		},
	},
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()
	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Exit error: %s\n", err)
		os.Exit(1)
	}
}
