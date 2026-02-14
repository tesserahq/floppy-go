package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"floppy-go/internal/config"
	"floppy-go/internal/context"
	"floppy-go/internal/manager"

	"github.com/spf13/cobra"
)

var (
	configPath string
	version    = "dev"
)

func main() {
	root := &cobra.Command{
		Use:          "floppy",
		Short:        "Floppy - Service orchestration tool",
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVarP(&configPath, "file", "f", "", "Path to services.yaml file")

	root.AddCommand(cmdUp())
	root.AddCommand(cmdStop())
	root.AddCommand(cmdDown())
	root.AddCommand(cmdPs())
	root.AddCommand(cmdList())
	root.AddCommand(cmdExec())
	root.AddCommand(cmdPull())
	root.AddCommand(cmdReset())
	root.AddCommand(cmdUpdateLib())
	root.AddCommand(cmdAddLib())
	root.AddCommand(cmdSetup())
	root.AddCommand(cmdLogs())
	root.AddCommand(cmdDoctor())
	root.AddCommand(cmdSetContext())
	root.AddCommand(cmdVersion())

	if err := root.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func loadManager() (*manager.Manager, error) {
	cfg, resolved, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return manager.New(cfg, resolved), nil
}

func cmdUp() *cobra.Command {
	var detached bool
	var force bool
	var build bool
	var noPTY bool
	cmd := &cobra.Command{
		Use:   "up [service-or-bundle ...]",
		Short: "Start services",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			_ = build
			if !noPTY && os.Getenv("FLOPPY_NO_PTY") == "1" {
				noPTY = true
			}
			return mgr.Up(args, detached, force, noPTY)
		},
	}
	cmd.Flags().BoolVarP(&detached, "detached", "d", false, "Run in background")
	cmd.Flags().BoolVar(&force, "force", false, "Kill existing processes using required ports")
	cmd.Flags().BoolVar(&build, "build", false, "Build services before starting (reserved)")
	cmd.Flags().BoolVar(&noPTY, "no-pty", false, "Disable PTY (useful if PTY is blocked)")
	return cmd
}

func cmdStop() *cobra.Command {
	var remove bool
	cmd := &cobra.Command{
		Use:   "stop [service ...]",
		Short: "Stop services",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			_ = remove
			return mgr.Stop(args)
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove stopped services (reserved)")
	return cmd
}

func cmdDown() *cobra.Command {
	var remove bool
	cmd := &cobra.Command{
		Use:   "down [service ...]",
		Short: "Stop services (alias for stop)",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			_ = remove
			return mgr.Stop(args)
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove stopped services (reserved)")
	return cmd
}

func cmdPs() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List running services",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			mgr.Ps(quiet)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only display service names")
	return cmd
}

func cmdList() *cobra.Command {
	var simple bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available services and bundles",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			mgr.List(!simple)
			return nil
		},
	}
	cmd.Flags().BoolVar(&simple, "simple", false, "Display services in simple list format")
	return cmd
}

func cmdExec() *cobra.Command {
	var serviceType string
	var exclude string
	cmd := &cobra.Command{
		Use:   "exec COMMAND [args...]",
		Short: "Run a command in each service directory",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			excludeList := splitComma(exclude)
			return mgr.Exec(args, serviceType, excludeList)
		},
	}
	cmd.Flags().StringVar(&serviceType, "type", "", "Filter by service type (api, worker, webapp, library, portal)")
	cmd.Flags().StringVar(&exclude, "exclude", "", "Comma-separated list of services to exclude")
	return cmd
}

func cmdPull() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull [service ...]",
		Short: "Pull latest changes for services with repos",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			mgr.Pull(args)
			return nil
		},
	}
	return cmd
}

func cmdReset() *cobra.Command {
	var serviceType string
	var exclude string
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset git repositories for services",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			excludeList := splitComma(exclude)
			mgr.Reset(serviceType, excludeList)
			return nil
		},
	}
	cmd.Flags().StringVar(&serviceType, "type", "", "Filter by service type")
	cmd.Flags().StringVar(&exclude, "exclude", "", "Comma-separated list of services to exclude")
	return cmd
}

func cmdUpdateLib() *cobra.Command {
	var serviceType string
	var exclude string
	cmd := &cobra.Command{
		Use:   "update-lib LIB_NAME",
		Short: "Update a dependency across services",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			excludeList := splitComma(exclude)
			mgr.UpdateLib(args[0], serviceType, excludeList)
			return nil
		},
	}
	cmd.Flags().StringVar(&serviceType, "type", "", "Filter by service type")
	cmd.Flags().StringVar(&exclude, "exclude", "", "Comma-separated list of services to exclude")
	return cmd
}

func cmdAddLib() *cobra.Command {
	var serviceType string
	var exclude string
	cmd := &cobra.Command{
		Use:   "add-lib LIB_NAME",
		Short: "Add a dependency across services",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			excludeList := splitComma(exclude)
			mgr.AddLib(args[0], serviceType, excludeList)
			return nil
		},
	}
	cmd.Flags().StringVar(&serviceType, "type", "", "Filter by service type")
	cmd.Flags().StringVar(&exclude, "exclude", "", "Comma-separated list of services to exclude")
	return cmd
}

func cmdSetup() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Setup dependencies, databases, and migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			mgr.Setup()
			return nil
		},
	}
	return cmd
}

func cmdLogs() *cobra.Command {
	var follow bool
	var tail int
	cmd := &cobra.Command{
		Use:   "logs SERVICE",
		Short: "Show logs for a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			mgr.Logs(args[0], follow, tail)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVar(&tail, "tail", 100, "Number of lines to show from the end")
	return cmd
}

func cmdSetContext() *cobra.Command {
	var file string
	var show bool
	var clear bool
	cmd := &cobra.Command{
		Use:   "set-context",
		Short: "Set the default services.yaml file location",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clear {
				return context.Clear()
			}
			if show {
				ctxPath, servicesPath, exists := context.Info()
				fmt.Printf("Current context:\n  Context file: %s\n", ctxPath)
				if servicesPath != "" {
					status := "NOT FOUND"
					if exists {
						status = "exists"
					}
					fmt.Printf("  Services file: %s (%s)\n", servicesPath, status)
				} else {
					fmt.Println("  Services file: Not set")
				}
				return nil
			}
			if file == "" {
				if _, err := os.Stat("services.yaml"); err == nil {
					file = "services.yaml"
				} else {
					return fmt.Errorf("no services.yaml found in current directory. Use -f to specify the path")
				}
			}
			if err := context.SetServicesFilePath(file); err != nil {
				return err
			}
			abs, _ := filepath.Abs(file)
			fmt.Printf("Context set successfully. Services file: %s\n", abs)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to services.yaml file")
	cmd.Flags().BoolVar(&clear, "clear", false, "Clear stored context")
	cmd.Flags().BoolVar(&show, "show", false, "Show current context")
	return cmd
}

func cmdDoctor() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Show resolved tool paths and environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := loadManager()
			if err != nil {
				return err
			}
			mgr.Doctor()
			return nil
		},
	}
	return cmd
}

func cmdVersion() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("floppy version %s\n", version)
			return nil
		},
	}
	return cmd
}

func splitComma(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
