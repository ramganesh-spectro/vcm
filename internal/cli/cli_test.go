package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vcm/internal/config"
	"vcm/internal/vsphere"
)

func TestVMListUsesConfigDefaultFolder(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
url: vcsa.example.com
username: user
password: secret
datacenter: Datacenter
defaultFolder: sp-ramganesh.senthilkumar
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	svc := &fakeService{}
	app := App{
		NewClient: func(_ context.Context, cfg config.Config) (Service, error) {
			return svc, nil
		},
	}

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--config", configPath,
		"vm", "list",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if svc.listFolder != "sp-ramganesh.senthilkumar" {
		t.Fatalf("list folder = %q", svc.listFolder)
	}
}

func TestVMListDispatchesFolderAndPrintsTable(t *testing.T) {
	svc := &fakeService{
		vms: []vsphere.VM{{
			Name:       "router-01",
			PowerState: "poweredOn",
			IPAddress:  "192.0.2.10",
		}},
	}
	app := testApp(t, svc)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"vm", "list", "--folder", "Lab",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if svc.listFolder != "Lab" {
		t.Fatalf("list folder = %q", svc.listFolder)
	}
	if !strings.Contains(stdout.String(), "router-01") || !strings.Contains(stdout.String(), "poweredOn") {
		t.Fatalf("stdout did not contain VM row: %q", stdout.String())
	}
}

func TestVMRestartDispatchesPowerAction(t *testing.T) {
	svc := &fakeService{}
	app := testApp(t, svc)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"vm", "restart", "router-01",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if svc.powerName != "router-01" || svc.powerAction != "restart" {
		t.Fatalf("power call = (%q, %q)", svc.powerName, svc.powerAction)
	}
}

func TestVMCloneDispatchesDefaultsAndPrintsResult(t *testing.T) {
	svc := &fakeService{
		cloneResult: vsphere.VM{
			Name:       "ram-test-01",
			Path:       "/Datacenter/vm/sp-ramganesh.senthilkumar/ram-test-01",
			PowerState: "poweredOff",
		},
	}
	app := testApp(t, svc)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"vm", "clone", "--source", "base-template", "--name", "ram-test-01",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if svc.cloneSpec.Source != "base-template" {
		t.Fatalf("clone source = %q", svc.cloneSpec.Source)
	}
	if svc.cloneSpec.Name != "ram-test-01" {
		t.Fatalf("clone name = %q", svc.cloneSpec.Name)
	}
	if svc.cloneSpec.Folder != defaultCloneFolder {
		t.Fatalf("clone folder = %q", svc.cloneSpec.Folder)
	}
	if svc.cloneSpec.Datastore != defaultCloneDatastore {
		t.Fatalf("clone datastore = %q", svc.cloneSpec.Datastore)
	}
	if svc.cloneSpec.PowerOn {
		t.Fatal("clone PowerOn = true, want false")
	}
	if !strings.Contains(stdout.String(), "ram-test-01") {
		t.Fatalf("stdout did not contain clone result: %q", stdout.String())
	}
}

func TestVMCloneDispatchesOverridesAndPowerOn(t *testing.T) {
	svc := &fakeService{cloneResult: vsphere.VM{Name: "ram-test-02"}}
	app := testApp(t, svc)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"vm", "clone",
		"--source", "base-template",
		"--name", "ram-test-02",
		"--folder", "CustomFolder",
		"--datastore", "datastore2",
		"--pool", "Resources",
		"--power-on",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if svc.cloneSpec.Folder != "CustomFolder" {
		t.Fatalf("clone folder = %q", svc.cloneSpec.Folder)
	}
	if svc.cloneSpec.Datastore != "datastore2" {
		t.Fatalf("clone datastore = %q", svc.cloneSpec.Datastore)
	}
	if svc.cloneSpec.Pool != "Resources" {
		t.Fatalf("clone pool = %q", svc.cloneSpec.Pool)
	}
	if !svc.cloneSpec.PowerOn {
		t.Fatal("clone PowerOn = false, want true")
	}
}

func TestVMCloneRequiresSourceAndName(t *testing.T) {
	svc := &fakeService{}
	app := testApp(t, svc)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"vm", "clone", "--source", "base-template",
	}, &stdout, &stderr)

	if code == 0 {
		t.Fatal("exit code = 0, want failure")
	}
	if svc.cloneSpec.Source != "" {
		t.Fatalf("clone should not dispatch, got source %q", svc.cloneSpec.Source)
	}
	if !strings.Contains(stderr.String(), "usage: vcm vm clone") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestVMCloneUsesConfigDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
url: vcsa.example.com
username: user
password: secret
datacenter: Datacenter
defaultFolder: configured-folder
defaultDatastore: configured-datastore
defaultPool: /Datacenter/host/Cluster1/Resources
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	for _, key := range []string{
		config.EnvURL,
		config.EnvUsername,
		config.EnvPassword,
		config.EnvDatacenter,
		config.EnvDefaultFolder,
		config.EnvDefaultDatastore,
		config.EnvDefaultPool,
	} {
		t.Setenv(key, "")
	}

	svc := &fakeService{cloneResult: vsphere.VM{Name: "ram-test-01"}}
	app := App{
		NewClient: func(_ context.Context, cfg config.Config) (Service, error) {
			if cfg.URL != "vcsa.example.com" {
				t.Fatalf("cfg.URL = %q", cfg.URL)
			}
			if cfg.Username != "user" {
				t.Fatalf("cfg.Username = %q", cfg.Username)
			}
			if cfg.Password != "secret" {
				t.Fatalf("cfg.Password = %q", cfg.Password)
			}
			return svc, nil
		},
	}

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--config", configPath,
		"vm", "clone", "--source", "base-template", "--name", "ram-test-01",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if svc.cloneSpec.Folder != "configured-folder" {
		t.Fatalf("clone folder = %q", svc.cloneSpec.Folder)
	}
	if svc.cloneSpec.Datastore != "configured-datastore" {
		t.Fatalf("clone datastore = %q", svc.cloneSpec.Datastore)
	}
	if svc.cloneSpec.Pool != "/Datacenter/host/Cluster1/Resources" {
		t.Fatalf("clone pool = %q", svc.cloneSpec.Pool)
	}
}

func TestVMCDAttachDispatchesSpecWithTrailingDeviceFlag(t *testing.T) {
	svc := &fakeService{
		attachResult: vsphere.VM{
			Name:       "ram-clone-1",
			PowerState: "poweredOff",
		},
	}
	app := testApp(t, svc)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"vm", "cd", "attach",
		"/Datacenter/vm/sp-ramganesh.senthilkumar/ram-clone-1",
		"[vsanDatastore1] iso/seed.iso",
		"--device", "1",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if svc.attachSpec.VM != "/Datacenter/vm/sp-ramganesh.senthilkumar/ram-clone-1" {
		t.Fatalf("attach VM = %q", svc.attachSpec.VM)
	}
	if svc.attachSpec.ISOPath != "[vsanDatastore1] iso/seed.iso" {
		t.Fatalf("attach ISO = %q", svc.attachSpec.ISOPath)
	}
	if svc.attachSpec.Device != 1 {
		t.Fatalf("attach device = %d", svc.attachSpec.Device)
	}
	if !strings.Contains(stdout.String(), "Attached") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestVMCDAttachDefaultsToFirstDevice(t *testing.T) {
	svc := &fakeService{attachResult: vsphere.VM{Name: "ram-clone-1"}}
	app := testApp(t, svc)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"vm", "cd", "attach",
		"ram-clone-1",
		"[vsanDatastore1] iso/installer.iso",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if svc.attachSpec.Device != 0 {
		t.Fatalf("attach device = %d", svc.attachSpec.Device)
	}
}

func TestVMCDAttachRejectsInvalidArgs(t *testing.T) {
	svc := &fakeService{}
	app := testApp(t, svc)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"vm", "cd", "attach", "ram-clone-1",
	}, &stdout, &stderr)

	if code == 0 {
		t.Fatal("exit code = 0, want failure")
	}
	if !strings.Contains(stderr.String(), "usage: vcm vm cd attach") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDatastoreUploadDispatchesPaths(t *testing.T) {
	svc := &fakeService{}
	app := testApp(t, svc)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"datastore", "upload", "./installer.iso", "[datastore1] iso/installer.iso",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if svc.uploadLocal != "./installer.iso" {
		t.Fatalf("upload local = %q", svc.uploadLocal)
	}
	if svc.uploadRemote != "[datastore1] iso/installer.iso" {
		t.Fatalf("upload remote = %q", svc.uploadRemote)
	}
}

func TestWebVMOpenCallsBrowserOpener(t *testing.T) {
	svc := &fakeService{webURL: "https://vcsa.example.com/ui/app/vm"}
	var opened string
	app := testApp(t, svc)
	app.OpenURL = func(link string) error {
		opened = link
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{
		"--url", "vcsa.example.com",
		"--username", "user",
		"--password", "secret",
		"web", "vm", "--open", "router-01",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if opened != svc.webURL {
		t.Fatalf("opened = %q", opened)
	}
	if !strings.Contains(stdout.String(), svc.webURL) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func testApp(t *testing.T, svc *fakeService) App {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	t.Setenv(config.EnvConfig, configPath)
	return App{
		NewClient: func(_ context.Context, cfg config.Config) (Service, error) {
			if cfg.URL != "vcsa.example.com" {
				t.Fatalf("cfg.URL = %q", cfg.URL)
			}
			if cfg.Username != "user" {
				t.Fatalf("cfg.Username = %q", cfg.Username)
			}
			if cfg.Password != "secret" {
				t.Fatalf("cfg.Password = %q", cfg.Password)
			}
			return svc, nil
		},
	}
}

type fakeService struct {
	vms          []vsphere.VM
	stores       []vsphere.Datastore
	webURL       string
	cloneResult  vsphere.VM
	cloneSpec    vsphere.CloneSpec
	attachResult vsphere.VM
	attachSpec   vsphere.CDAttachSpec
	listFolder   string
	powerName    string
	powerAction  string
	uploadLocal  string
	uploadRemote string
}

func (f *fakeService) Close(context.Context) error {
	return nil
}

func (f *fakeService) ListVMs(_ context.Context, folder string) ([]vsphere.VM, error) {
	f.listFolder = folder
	return f.vms, nil
}

func (f *fakeService) GetVM(context.Context, string) (vsphere.VM, error) {
	return vsphere.VM{Name: "router-01"}, nil
}

func (f *fakeService) AttachISO(_ context.Context, spec vsphere.CDAttachSpec) (vsphere.VM, error) {
	f.attachSpec = spec
	return f.attachResult, nil
}

func (f *fakeService) CloneVM(_ context.Context, spec vsphere.CloneSpec) (vsphere.VM, error) {
	f.cloneSpec = spec
	return f.cloneResult, nil
}

func (f *fakeService) PowerVM(_ context.Context, name string, action string) error {
	f.powerName = name
	f.powerAction = action
	return nil
}

func (f *fakeService) WebURLForVM(context.Context, string) (string, error) {
	return f.webURL, nil
}

func (f *fakeService) ListDatastores(context.Context) ([]vsphere.Datastore, error) {
	return f.stores, nil
}

func (f *fakeService) UploadToDatastore(_ context.Context, local string, remote string) error {
	f.uploadLocal = local
	f.uploadRemote = remote
	return nil
}
