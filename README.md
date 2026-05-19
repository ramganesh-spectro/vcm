# vcm

`vcm` is an MVP vCenter manager CLI for common VM and datastore workflows.

It is built in Go on top of VMware's `[govmomi](https://github.com/vmware/govmomi)` SDK. The first version focuses on proving reliable vCenter authentication, VM inventory, VM power actions, datastore listing/upload, and vCenter web UI links.

## Install

```sh
go build -o vcm ./cmd/vcm
```

## Configuration

By default, `vcm` reads:

```sh
~/.config/vcm/config.yaml
```

Example:

```yaml
url: vcenter.spectrocloud.dev
username: your-user
datacenter: Datacenter
insecure: true
defaultFolder: sp-ramganesh.senthilkumar
defaultDatastore: vsanDatastore1
defaultPool: /Datacenter/host/Cluster1/Resources
```

Keep the password in `VCM_PASSWORD` or pass it with `--password` instead of storing it in plaintext.

Use a different config file with:

```sh
./vcm --config ./config.yaml vm list
```

You can still pass connection settings as flags:

```sh
./vcm --url vcsa.example.com \
  --username administrator@vsphere.local \
  --password 'secret' \
  --datacenter Lab \
  vm list
```

Or set environment variables:

```sh
export VCM_URL=vcsa.example.com
export VCM_USERNAME=administrator@vsphere.local
export VCM_PASSWORD='secret'
export VCM_DATACENTER=Lab
export VCM_INSECURE=true
export VCM_DEFAULT_FOLDER=Lab
export VCM_DEFAULT_DATASTORE=datastore1
export VCM_DEFAULT_POOL='/Datacenter/host/Cluster1/Resources'
```

Use `--insecure` or `VCM_INSECURE=true` for lab vCenters with self-signed certificates.

## Commands

List VMs:

```sh
./vcm vm list --folder Lab
./vcm --json vm list --folder Lab
```

For a plain folder name, `vm list` searches **recursively** under that folder (nested subfolders included). To list only VMs directly in a folder, pass an explicit glob, for example `--folder 'Lab/*'`.

Inspect a VM:

```sh
./vcm vm info router-01
./vcm vm info /Lab/vm/router-01
```

Clone a VM or template:

```sh
./vcm vm clone --source base-template --name ram-test-01
./vcm vm clone --source base-template --name ram-test-02 --power-on
```

By default, clones are placed in folder `sp-ramganesh.senthilkumar` on datastore `vsanDatastore1`. Override placement when needed:

```sh
./vcm vm clone \
  --source base-template \
  --name ram-test-03 \
  --folder sp-ramganesh.senthilkumar \
  --datastore vsanDatastore1 \
  --pool Resources
```

Attach datastore ISO files to CD/DVD drives:

```sh
./vcm vm cd attach \
  /Datacenter/vm/sp-ramganesh.senthilkumar/ram-clone-1 \
  "[vsanDatastore1] 5dc7c766-cdc6-c946-2a69-0cc47ac0f068/ram-test-isos/installer.iso" \
  --device 0

./vcm vm cd attach \
  /Datacenter/vm/sp-ramganesh.senthilkumar/ram-clone-1 \
  "[vsanDatastore1] 5dc7c766-cdc6-c946-2a69-0cc47ac0f068/ram-test-isos/seed.iso" \
  --device 1
```

The command edits an existing CD/DVD device at that index, or adds the next CD/DVD device when the index is the next available slot. It sets both connected and connect-at-power-on.

Power operations:

```sh
./vcm vm start router-01
./vcm vm stop router-01
./vcm vm restart router-01
```

List datastores:

```sh
./vcm datastore list
```

Upload a local file to a datastore:

```sh
./vcm datastore upload ./installer.iso "[datastore1] iso/installer.iso"
./vcm datastore upload ./installer.iso "datastore1:iso/installer.iso"
```

Print or open a VM web UI link:

```sh
./vcm web vm router-01
./vcm web vm --open router-01
```

## Required vCenter Permissions

The account used by `vcm` needs enough privileges for the commands you run:

- Read-only inventory access for `vm list`, `vm info`, `datastore list`, and `web vm`.
- VM power privileges for `vm start`, `vm stop`, and `vm restart`.
- VM clone/provision privileges plus resource pool assignment for `vm clone`.
- VM hardware reconfigure privileges for `vm cd attach`.
- Datastore browse and file management privileges for `datastore upload`.

## MVP Boundaries

This version can clone an existing VM/template and attach datastore ISOs to CD/DVD devices, but it intentionally does not create blank VMs, customize guest identity/IP addresses, reconfigure CPU/memory/networking, or provide a TUI. Those should build on the current `internal/vsphere` package after the basic CLI is proven against a real vCenter.