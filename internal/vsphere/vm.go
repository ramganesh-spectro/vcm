package vsphere

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

func (c *Client) ListVMs(ctx context.Context, folder string) ([]VM, error) {
	vms, err := c.finder.VirtualMachineList(ctx, vmListPattern(folder))
	if err != nil {
		return nil, err
	}

	result := make([]VM, 0, len(vms))
	for _, vm := range vms {
		info, err := c.vmInfo(ctx, vm)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}

func (c *Client) GetVM(ctx context.Context, name string) (VM, error) {
	vm, err := c.findVM(ctx, name)
	if err != nil {
		return VM{}, err
	}
	return c.vmInfo(ctx, vm)
}

func (c *Client) AttachISO(ctx context.Context, spec CDAttachSpec) (VM, error) {
	if err := validateCDAttachSpec(spec); err != nil {
		return VM{}, err
	}

	vm, err := c.findVM(ctx, spec.VM)
	if err != nil {
		return VM{}, err
	}

	devices, err := vm.Device(ctx)
	if err != nil {
		return VM{}, err
	}

	cdroms := cdromDevices(devices)
	var cdrom *types.VirtualCdrom
	operation := types.VirtualDeviceConfigSpecOperationEdit
	switch {
	case spec.Device < len(cdroms):
		cdrom = cdroms[spec.Device]
	case spec.Device == len(cdroms):
		controller, err := devices.FindSATAController("")
		if err != nil {
			return VM{}, fmt.Errorf("find SATA/AHCI controller for new CD/DVD drive: %w", err)
		}
		cdrom, err = devices.CreateCdrom(controller)
		if err != nil {
			return VM{}, err
		}
		operation = types.VirtualDeviceConfigSpecOperationAdd
	default:
		return VM{}, fmt.Errorf("CD/DVD device %d does not exist; next available device index is %d", spec.Device, len(cdroms))
	}

	devices.InsertIso(cdrom, spec.ISOPath)
	cdrom.Connectable = &types.VirtualDeviceConnectInfo{
		AllowGuestControl: true,
		Connected:         true,
		StartConnected:    true,
	}

	task, err := vm.Reconfigure(ctx, types.VirtualMachineConfigSpec{
		DeviceChange: []types.BaseVirtualDeviceConfigSpec{
			&types.VirtualDeviceConfigSpec{
				Operation: operation,
				Device:    cdrom,
			},
		},
	})
	if err != nil {
		return VM{}, err
	}
	if err := task.Wait(ctx); err != nil {
		return VM{}, err
	}

	return c.vmInfo(ctx, vm)
}

func (c *Client) CloneVM(ctx context.Context, spec CloneSpec) (VM, error) {
	if err := validateCloneSpec(spec); err != nil {
		return VM{}, err
	}

	source, err := c.findVM(ctx, spec.Source)
	if err != nil {
		return VM{}, fmt.Errorf("find source VM %q: %w", spec.Source, err)
	}

	folder, err := c.finder.Folder(ctx, spec.Folder)
	if err != nil {
		return VM{}, fmt.Errorf("find target folder %q: %w", spec.Folder, err)
	}

	datastore, err := c.finder.Datastore(ctx, spec.Datastore)
	if err != nil {
		return VM{}, fmt.Errorf("find target datastore %q: %w", spec.Datastore, err)
	}

	pool, err := c.cloneResourcePool(ctx, source, spec.Pool)
	if err != nil {
		return VM{}, err
	}

	folderRef := folder.Reference()
	datastoreRef := datastore.Reference()
	poolRef := pool.Reference()
	task, err := source.Clone(ctx, folder, spec.Name, types.VirtualMachineCloneSpec{
		Location: types.VirtualMachineRelocateSpec{
			Folder:    &folderRef,
			Datastore: &datastoreRef,
			Pool:      &poolRef,
		},
		PowerOn: spec.PowerOn,
	})
	if err != nil {
		return VM{}, err
	}
	if err := task.Wait(ctx); err != nil {
		return VM{}, err
	}

	return c.GetVM(ctx, clonedVMPath(folder.InventoryPath, spec.Name))
}

func (c *Client) PowerVM(ctx context.Context, name string, action string) error {
	vm, err := c.findVM(ctx, name)
	if err != nil {
		return err
	}

	var task *object.Task
	switch action {
	case "start":
		task, err = vm.PowerOn(ctx)
	case "stop":
		task, err = vm.PowerOff(ctx)
	case "restart":
		task, err = vm.Reset(ctx)
	default:
		return fmt.Errorf("unsupported power action %q", action)
	}
	if err != nil {
		return err
	}
	return task.Wait(ctx)
}

func validateCloneSpec(spec CloneSpec) error {
	if strings.TrimSpace(spec.Source) == "" {
		return fmt.Errorf("source VM or template is required")
	}
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("new VM name is required")
	}
	if strings.TrimSpace(spec.Folder) == "" {
		return fmt.Errorf("target folder is required")
	}
	if strings.TrimSpace(spec.Datastore) == "" {
		return fmt.Errorf("target datastore is required")
	}
	return nil
}

func validateCDAttachSpec(spec CDAttachSpec) error {
	if strings.TrimSpace(spec.VM) == "" {
		return fmt.Errorf("VM name or path is required")
	}
	if strings.TrimSpace(spec.ISOPath) == "" {
		return fmt.Errorf("datastore ISO path is required")
	}
	if spec.Device < 0 {
		return fmt.Errorf("CD/DVD device index must be >= 0")
	}
	return nil
}

func cdromDevices(devices object.VirtualDeviceList) []*types.VirtualCdrom {
	selected := devices.SelectByType((*types.VirtualCdrom)(nil))
	cdroms := make([]*types.VirtualCdrom, 0, len(selected))
	for _, device := range selected {
		if cdrom, ok := device.(*types.VirtualCdrom); ok {
			cdroms = append(cdroms, cdrom)
		}
	}
	sort.SliceStable(cdroms, func(i, j int) bool {
		leftUnit, rightUnit := int32(-1), int32(-1)
		if cdroms[i].UnitNumber != nil {
			leftUnit = *cdroms[i].UnitNumber
		}
		if cdroms[j].UnitNumber != nil {
			rightUnit = *cdroms[j].UnitNumber
		}
		if cdroms[i].ControllerKey != cdroms[j].ControllerKey {
			return cdroms[i].ControllerKey < cdroms[j].ControllerKey
		}
		if leftUnit != rightUnit {
			return leftUnit < rightUnit
		}
		return cdroms[i].Key < cdroms[j].Key
	})
	return cdroms
}

func (c *Client) cloneResourcePool(ctx context.Context, source *object.VirtualMachine, poolName string) (*object.ResourcePool, error) {
	if strings.TrimSpace(poolName) != "" {
		pool, err := c.finder.ResourcePool(ctx, poolName)
		if err != nil {
			return nil, fmt.Errorf("find target resource pool %q: %w", poolName, err)
		}
		return pool, nil
	}

	if pool, err := source.ResourcePool(ctx); err == nil && pool != nil {
		return pool, nil
	}
	pool, err := c.finder.DefaultResourcePool(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve default resource pool: %w", err)
	}
	return pool, nil
}

func clonedVMPath(folderPath string, name string) string {
	folderPath = strings.TrimRight(strings.TrimSpace(folderPath), "/")
	name = strings.TrimSpace(name)
	if folderPath == "" {
		return name
	}
	return folderPath + "/" + name
}

func (c *Client) WebURLForVM(ctx context.Context, name string) (string, error) {
	vm, err := c.findVM(ctx, name)
	if err != nil {
		return "", err
	}

	serverGUID := strings.TrimSpace(c.client.ServiceContent.About.InstanceUuid)
	if serverGUID == "" {
		return "", fmt.Errorf("vCenter did not report an instance UUID")
	}

	ref := vm.Reference()
	urn := fmt.Sprintf("urn:vmomi:%s:%s:%s", ref.Type, ref.Value, serverGUID)
	return fmt.Sprintf("%s/ui/app/vm;nav=h/%s/summary", c.baseURL, url.PathEscape(urn)), nil
}

func (c *Client) findVM(ctx context.Context, name string) (*object.VirtualMachine, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("VM name or path is required")
	}
	return c.finder.VirtualMachine(ctx, name)
}

func (c *Client) vmInfo(ctx context.Context, vm *object.VirtualMachine) (VM, error) {
	var entity mo.VirtualMachine
	if err := c.client.RetrieveOne(ctx, vm.Reference(), []string{"name", "summary", "guest", "runtime", "datastore"}, &entity); err != nil {
		return VM{}, err
	}

	info := VM{
		Name:       entity.Name,
		Path:       vm.InventoryPath,
		PowerState: string(entity.Runtime.PowerState),
		MoID:       vm.Reference().Value,
	}

	if entity.Summary.Guest != nil {
		info.IPAddress = entity.Summary.Guest.IpAddress
	}
	if entity.Summary.Config.GuestFullName != "" {
		info.GuestOS = entity.Summary.Config.GuestFullName
	} else if entity.Guest != nil {
		info.GuestOS = entity.Guest.GuestFullName
	}
	if entity.Runtime.Host != nil {
		info.Host = c.nameForRef(ctx, *entity.Runtime.Host)
	}
	if len(entity.Datastore) > 0 {
		names := make([]string, 0, len(entity.Datastore))
		for _, ref := range entity.Datastore {
			if name := c.nameForRef(ctx, ref); name != "" {
				names = append(names, name)
			}
		}
		info.Datastore = strings.Join(names, ",")
	}

	return info, nil
}

func (c *Client) nameForRef(ctx context.Context, ref types.ManagedObjectReference) string {
	var entity mo.ManagedEntity
	if err := c.client.RetrieveOne(ctx, ref, []string{"name"}, &entity); err != nil {
		return ""
	}
	return entity.Name
}

func vmListPattern(folder string) string {
	folder = strings.TrimSpace(folder)
	if folder == "" || folder == "/" {
		return "*"
	}
	if strings.ContainsAny(folder, "*?[") {
		return folder
	}
	// govmomi "list" mode uses path + "/*" and only returns direct children of
	// that folder. Nested VM folders need the "..." wildcard (see find/doc.go).
	base := strings.TrimRight(folder, "/")
	if strings.HasSuffix(base, "...") {
		return base
	}
	return base + "/..."
}
