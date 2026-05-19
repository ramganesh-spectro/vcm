package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"vcm/internal/config"
	"vcm/internal/output"
	"vcm/internal/vsphere"
)

const (
	defaultCloneFolder    = "sp-ramganesh.senthilkumar"
	defaultCloneDatastore = "vsanDatastore1"
)

type Service interface {
	Close(context.Context) error
	ListVMs(context.Context, string) ([]vsphere.VM, error)
	GetVM(context.Context, string) (vsphere.VM, error)
	AttachISO(context.Context, vsphere.CDAttachSpec) (vsphere.VM, error)
	CloneVM(context.Context, vsphere.CloneSpec) (vsphere.VM, error)
	PowerVM(context.Context, string, string) error
	WebURLForVM(context.Context, string) (string, error)
	ListDatastores(context.Context) ([]vsphere.Datastore, error)
	UploadToDatastore(context.Context, string, string) error
}

type Factory func(context.Context, config.Config) (Service, error)

type App struct {
	NewClient Factory
	OpenURL   func(string) error
}

func NewApp() App {
	return App{
		NewClient: func(ctx context.Context, cfg config.Config) (Service, error) {
			return vsphere.NewClient(ctx, cfg)
		},
		OpenURL: openURL,
	}
}

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	return NewApp().Run(ctx, args, stdout, stderr)
}

func (a App) Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	configPath := preparseConfigPath(args)
	cfg, err := config.Load(configPath)
	if err != nil {
		return fail(stderr, err)
	}
	global := flag.NewFlagSet("vcm", flag.ContinueOnError)
	global.SetOutput(io.Discard)

	configFlag := global.String("config", configPath, "config file path")
	urlFlag := global.String("url", cfg.URL, "vCenter URL or host")
	usernameFlag := global.String("username", cfg.Username, "vCenter username")
	passwordFlag := global.String("password", cfg.Password, "vCenter password")
	datacenterFlag := global.String("datacenter", cfg.Datacenter, "vCenter datacenter")
	insecureFlag := global.Bool("insecure", cfg.Insecure, "skip TLS certificate verification")
	jsonFlag := global.Bool("json", false, "emit JSON output")
	helpFlag := global.Bool("help", false, "show help")
	global.BoolVar(helpFlag, "h", false, "show help")

	if err := global.Parse(args); err != nil {
		return fail(stderr, err)
	}

	cfg = config.Config{
		URL:              *urlFlag,
		Username:         *usernameFlag,
		Password:         *passwordFlag,
		Datacenter:       *datacenterFlag,
		Insecure:         *insecureFlag,
		DefaultFolder:    cfg.DefaultFolder,
		DefaultDatastore: cfg.DefaultDatastore,
		DefaultPool:      cfg.DefaultPool,
	}
	_ = configFlag

	rest := global.Args()
	if *helpFlag || len(rest) == 0 || rest[0] == "help" {
		_, _ = fmt.Fprint(stdout, usage())
		return 0
	}

	switch rest[0] {
	case "vm":
		return a.runVM(ctx, cfg, *jsonFlag, rest[1:], stdout, stderr)
	case "datastore":
		return a.runDatastore(ctx, cfg, *jsonFlag, rest[1:], stdout, stderr)
	case "web":
		return a.runWeb(ctx, cfg, rest[1:], stdout, stderr)
	default:
		return fail(stderr, fmt.Errorf("unknown command %q", rest[0]))
	}
}

func (a App) runVM(ctx context.Context, cfg config.Config, jsonOutput bool, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		return fail(stderr, errors.New("vm subcommand is required"))
	}

	switch args[0] {
	case "cd":
		return a.runVMCD(ctx, cfg, jsonOutput, args[1:], stdout, stderr)
	case "list":
		fs := newFlagSet("vm list")
		folder := fs.String("folder", "*", "folder path or glob")
		if err := fs.Parse(args[1:]); err != nil {
			return fail(stderr, err)
		}
		if fs.NArg() != 0 {
			return fail(stderr, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
		}
		return a.withClient(ctx, cfg, stderr, func(svc Service) int {
			vms, err := svc.ListVMs(ctx, *folder)
			if err != nil {
				return fail(stderr, err)
			}
			if jsonOutput {
				return write(stderr, output.JSON(stdout, vms))
			}
			rows := make([][]string, 0, len(vms))
			for _, vm := range vms {
				rows = append(rows, []string{vm.Name, vm.PowerState, vm.IPAddress, vm.GuestOS, vm.Host, vm.Datastore})
			}
			return write(stderr, output.Table(stdout, []string{"NAME", "POWER", "IP", "GUEST", "HOST", "DATASTORE"}, rows))
		})
	case "info":
		name, err := oneArg(args[1:], "vm info <vm>")
		if err != nil {
			return fail(stderr, err)
		}
		return a.withClient(ctx, cfg, stderr, func(svc Service) int {
			vm, err := svc.GetVM(ctx, name)
			if err != nil {
				return fail(stderr, err)
			}
			if jsonOutput {
				return write(stderr, output.JSON(stdout, vm))
			}
			return writeVMInfo(stdout, stderr, vm)
		})
	case "clone":
		fs := newFlagSet("vm clone")
		source := fs.String("source", "", "source VM or template name/path")
		name := fs.String("name", "", "new VM name")
		folder := fs.String("folder", cloneDefault(cfg.DefaultFolder, defaultCloneFolder), "target VM folder")
		datastore := fs.String("datastore", cloneDefault(cfg.DefaultDatastore, defaultCloneDatastore), "target datastore")
		pool := fs.String("pool", cfg.DefaultPool, "target resource pool")
		powerOn := fs.Bool("power-on", false, "power on after cloning")
		if err := fs.Parse(args[1:]); err != nil {
			return fail(stderr, err)
		}
		if fs.NArg() != 0 {
			return fail(stderr, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
		}
		spec := vsphere.CloneSpec{
			Source:    *source,
			Name:      *name,
			Folder:    *folder,
			Datastore: *datastore,
			Pool:      *pool,
			PowerOn:   *powerOn,
		}
		if strings.TrimSpace(spec.Source) == "" {
			return fail(stderr, errors.New("usage: vcm vm clone --source <source-vm-or-template> --name <new-vm-name>"))
		}
		if strings.TrimSpace(spec.Name) == "" {
			return fail(stderr, errors.New("usage: vcm vm clone --source <source-vm-or-template> --name <new-vm-name>"))
		}
		return a.withClient(ctx, cfg, stderr, func(svc Service) int {
			vm, err := svc.CloneVM(ctx, spec)
			if err != nil {
				return fail(stderr, err)
			}
			if jsonOutput {
				return write(stderr, output.JSON(stdout, vm))
			}
			return writeVMInfo(stdout, stderr, vm)
		})
	case "start", "stop", "restart":
		action := args[0]
		name, err := oneArg(args[1:], "vm "+action+" <vm>")
		if err != nil {
			return fail(stderr, err)
		}
		return a.withClient(ctx, cfg, stderr, func(svc Service) int {
			if err := svc.PowerVM(ctx, name, action); err != nil {
				return fail(stderr, err)
			}
			_, _ = fmt.Fprintf(stdout, "VM %q %s completed\n", name, action)
			return 0
		})
	default:
		return fail(stderr, fmt.Errorf("unknown vm subcommand %q", args[0]))
	}
}

func (a App) runVMCD(ctx context.Context, cfg config.Config, jsonOutput bool, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		return fail(stderr, errors.New("vm cd subcommand is required"))
	}
	if args[0] != "attach" {
		return fail(stderr, fmt.Errorf("unknown vm cd subcommand %q", args[0]))
	}

	spec, err := parseCDAttachArgs(args[1:])
	if err != nil {
		return fail(stderr, err)
	}

	return a.withClient(ctx, cfg, stderr, func(svc Service) int {
		vm, err := svc.AttachISO(ctx, spec)
		if err != nil {
			return fail(stderr, err)
		}
		if jsonOutput {
			return write(stderr, output.JSON(stdout, vm))
		}
		_, _ = fmt.Fprintf(stdout, "Attached %q to CD/DVD device %d on %q\n", spec.ISOPath, spec.Device, spec.VM)
		return writeVMInfo(stdout, stderr, vm)
	})
}

func (a App) runDatastore(ctx context.Context, cfg config.Config, jsonOutput bool, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		return fail(stderr, errors.New("datastore subcommand is required"))
	}

	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fail(stderr, fmt.Errorf("unexpected argument %q", args[1]))
		}
		return a.withClient(ctx, cfg, stderr, func(svc Service) int {
			stores, err := svc.ListDatastores(ctx)
			if err != nil {
				return fail(stderr, err)
			}
			if jsonOutput {
				return write(stderr, output.JSON(stdout, stores))
			}
			rows := make([][]string, 0, len(stores))
			for _, store := range stores {
				rows = append(rows, []string{
					store.Name,
					store.Type,
					formatBytes(store.CapacityBytes),
					formatBytes(store.FreeBytes),
					store.URL,
				})
			}
			return write(stderr, output.Table(stdout, []string{"NAME", "TYPE", "CAPACITY", "FREE", "URL"}, rows))
		})
	case "upload":
		if len(args) != 3 {
			return fail(stderr, errors.New("usage: vcm datastore upload <local-file> <datastore-path>"))
		}
		localFile := args[1]
		remotePath := args[2]
		return a.withClient(ctx, cfg, stderr, func(svc Service) int {
			if err := svc.UploadToDatastore(ctx, localFile, remotePath); err != nil {
				return fail(stderr, err)
			}
			_, _ = fmt.Fprintf(stdout, "Uploaded %q to %q\n", localFile, remotePath)
			return 0
		})
	default:
		return fail(stderr, fmt.Errorf("unknown datastore subcommand %q", args[0]))
	}
}

func (a App) runWeb(ctx context.Context, cfg config.Config, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		return fail(stderr, errors.New("web subcommand is required"))
	}
	if args[0] != "vm" {
		return fail(stderr, fmt.Errorf("unknown web subcommand %q", args[0]))
	}

	fs := newFlagSet("web vm")
	open := fs.Bool("open", false, "open URL in the default browser")
	if err := fs.Parse(args[1:]); err != nil {
		return fail(stderr, err)
	}
	name, err := oneArg(fs.Args(), "web vm [--open] <vm>")
	if err != nil {
		return fail(stderr, err)
	}

	return a.withClient(ctx, cfg, stderr, func(svc Service) int {
		link, err := svc.WebURLForVM(ctx, name)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, link)
		if *open {
			if a.OpenURL == nil {
				return fail(stderr, errors.New("open URL behavior is not configured"))
			}
			if err := a.OpenURL(link); err != nil {
				return fail(stderr, err)
			}
		}
		return 0
	})
}

func (a App) withClient(ctx context.Context, cfg config.Config, stderr io.Writer, fn func(Service) int) int {
	if a.NewClient == nil {
		return fail(stderr, errors.New("client factory is not configured"))
	}
	svc, err := a.NewClient(ctx, cfg)
	if err != nil {
		return fail(stderr, err)
	}
	defer func() {
		if err := svc.Close(ctx); err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: logout failed: %v\n", err)
		}
	}()
	return fn(svc)
}

func preparseConfigPath(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return ""
		}
		if arg == "--config" {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config=")
		}
	}
	return ""
}

func parseCDAttachArgs(args []string) (vsphere.CDAttachSpec, error) {
	spec := vsphere.CDAttachSpec{Device: 0}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--device":
			i++
			if i >= len(args) {
				return spec, errors.New("usage: vcm vm cd attach <vm> <datastore-iso-path> [--device <index>]")
			}
			device, err := strconv.Atoi(args[i])
			if err != nil {
				return spec, fmt.Errorf("invalid --device value %q", args[i])
			}
			spec.Device = device
		case strings.HasPrefix(arg, "--device="):
			value := strings.TrimPrefix(arg, "--device=")
			device, err := strconv.Atoi(value)
			if err != nil {
				return spec, fmt.Errorf("invalid --device value %q", value)
			}
			spec.Device = device
		case strings.HasPrefix(arg, "-"):
			return spec, fmt.Errorf("unknown flag %q", arg)
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) != 2 {
		return spec, errors.New("usage: vcm vm cd attach <vm> <datastore-iso-path> [--device <index>]")
	}
	spec.VM = positional[0]
	spec.ISOPath = positional[1]
	if spec.Device < 0 {
		return spec, fmt.Errorf("CD/DVD device index must be >= 0")
	}
	return spec, nil
}

func cloneDefault(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func oneArg(args []string, usage string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("usage: vcm %s", usage)
	}
	value := strings.TrimSpace(args[0])
	if value == "" {
		return "", fmt.Errorf("usage: vcm %s", usage)
	}
	return value, nil
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
	return 1
}

func write(stderr io.Writer, err error) int {
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}

func writeVMInfo(stdout io.Writer, stderr io.Writer, vm vsphere.VM) int {
	rows := [][]string{
		{"Name", vm.Name},
		{"Path", vm.Path},
		{"Power", vm.PowerState},
		{"IP", vm.IPAddress},
		{"Guest", vm.GuestOS},
		{"Host", vm.Host},
		{"Datastore", vm.Datastore},
		{"MoID", vm.MoID},
	}
	return write(stderr, output.Table(stdout, []string{"FIELD", "VALUE"}, rows))
}

func formatBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return strconv.FormatInt(value, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func openURL(link string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", link).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", link).Start()
	default:
		return exec.Command("xdg-open", link).Start()
	}
}

func usage() string {
	return `vcm manages common vCenter VM and datastore workflows.

Usage:
  vcm [global flags] vm list [--folder <path>]
  vcm [global flags] vm info <vm>
  vcm [global flags] vm clone --source <source-vm-or-template> --name <new-vm-name>
  vcm [global flags] vm cd attach <vm> <datastore-iso-path> [--device <index>]
  vcm [global flags] vm start|stop|restart <vm>
  vcm [global flags] datastore list
  vcm [global flags] datastore upload <local-file> "[datastore] path/file"
  vcm [global flags] web vm [--open] <vm>

Global flags:
  --config      Config file path (env: VCM_CONFIG, default: ~/.config/vcm/config.yaml)
  --url          vCenter URL or host (env: VCM_URL)
  --username     vCenter username (env: VCM_USERNAME)
  --password     vCenter password (env: VCM_PASSWORD)
  --datacenter   Datacenter name or inventory path (env: VCM_DATACENTER)
  --insecure     Skip TLS certificate verification (env: VCM_INSECURE)
  --json         Emit JSON output where supported

Examples:
  vcm --url vcsa.example.com --username administrator@vsphere.local --password secret vm list --folder Lab
  vcm vm clone --source base-template --name ram-test-01
  vcm vm cd attach ram-test-01 "[vsanDatastore1] iso/installer.iso" --device 0
  vcm vm restart /Datacenter/vm/Lab/router-01
  vcm datastore upload ./installer.iso "[datastore1] iso/installer.iso"
`
}
