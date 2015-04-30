package vsphere

import (
	"log"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

const (
	defaultTimeZone = "Etc/UTC"
	defaultDomain   = "vsphere.local"
)

type networkInterface struct {
	deviceName string
	label      string
	ipAddress  string
	subnetMask string
}

type additionalHardDisk struct {
	size int64
	iops int64
}

type virtualMachine struct {
	name                string
	datacenter          string
	cluster             string
	resourcePool        string
	datastore           string
	vcpu                int
	memoryMb            int64
	template            string
	networkInterfaces   []networkInterface
	additionalHardDisks []additionalHardDisk
	gateway             string
	domain              string
	timeZone            string
	dnsSuffixes         []string
	dnsServers          []string
}

func (vm *virtualMachine) deployVirtualMachine(c *govmomi.Client) error {
	if len(vm.dnsServers) == 0 {
		vm.dnsServers = []string{
			"8.8.8.8",
			"8.8.4.4",
		}
	}

	if len(vm.dnsSuffixes) == 0 {
		vm.dnsSuffixes = []string{
			defaultDomain,
		}
	}

	if vm.domain == "" {
		vm.domain = defaultDomain
	}

	if vm.timeZone == "" {
		vm.timeZone = defaultTimeZone
	}

	finder := find.NewFinder(c.Client, true)
	dc, err := findDatacenter(finder, vm.datacenter)
	if err != nil {
		return err
	}
	finder = finder.SetDatacenter(dc)

	template, err := finder.VirtualMachine(context.TODO(), vm.template)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] template: %#v", template)

	resourcePool, err := findResourcePool(finder, vm.resourcePool, vm.cluster)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] resource pool: %#v", resourcePool)

	dcFolders, err := dc.Folders(context.TODO())
	if err != nil {
		return err
	}
	datastore, err := findDatastore(c, finder, dcFolders, template, resourcePool, vm.datastore)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] datastore: %#v", datastore)

	relocateSpec, err := getVMRelocateSpec(resourcePool, datastore, template)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] relocate spec: %v", relocateSpec)

	// network
	networkDevices := []types.BaseVirtualDeviceConfigSpec{}
	networkConfigs := []types.CustomizationAdapterMapping{}
	for _, network := range vm.networkInterfaces {
		// network device
		device, err := networkDevice(finder, network.label)
		if err != nil {
			return err
		}
		networkDevices = append(networkDevices, device)

		var ipSetting types.CustomizationIPSettings
		if network.ipAddress == "" {
			ipSetting = types.CustomizationIPSettings{
				Ip: &types.CustomizationDhcpIpGenerator{},
			}
		} else {
			ipSetting = types.CustomizationIPSettings{
				Gateway: []string{
					vm.gateway,
				},
				Ip: &types.CustomizationFixedIp{
					IpAddress: network.ipAddress,
				},
				SubnetMask: network.subnetMask,
			}
		}

		// network config
		config := types.CustomizationAdapterMapping{
			Adapter: ipSetting,
		}
		networkConfigs = append(networkConfigs, config)
	}
	log.Printf("[DEBUG] network configs: %v", networkConfigs[0].Adapter)

	// make config spec
	configSpec := types.VirtualMachineConfigSpec{
		NumCPUs:           vm.vcpu,
		NumCoresPerSocket: 1,
		MemoryMB:          vm.memoryMb,
		DeviceChange:      networkDevices,
	}
	log.Printf("[DEBUG] virtual machine config spec: %v", configSpec)

	// make custom spec
	customSpec := createCustomizationSpec(vm.name, vm.domain, vm.timeZone, vm.dnsSuffixes, vm.dnsServers, networkConfigs)
	log.Printf("[DEBUG] custom spec: %v", customSpec)

	// make vm clone spec
	cloneSpec := types.VirtualMachineCloneSpec{
		Location:      relocateSpec,
		Template:      false,
		Config:        &configSpec,
		Customization: &customSpec,
		PowerOn:       true,
	}
	log.Printf("[DEBUG] clone spec: %v", cloneSpec)

	task, err := template.Clone(context.TODO(), dcFolders.VmFolder, vm.name, cloneSpec)
	if err != nil {
		return err
	}

	err = task.Wait(context.TODO())
	if err != nil {
		return err
	}

	newVM, err := finder.VirtualMachine(context.TODO(), vm.name)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] new vm: %v", newVM)

	ip, err := newVM.WaitForIP(context.TODO())
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] ip address: %v", ip)

	for _, hd := range vm.additionalHardDisks {
		err = addHardDisk(newVM, hd.size, hd.iops)
		if err != nil {
			return err
		}
	}

	return nil
}

func findDatastoreForClone(c *govmomi.Client, storagePod *object.Folder, template *object.VirtualMachine, vmFolder *object.Folder, resourcePool *object.ResourcePool) (*object.Datastore, error) {

	templateRef := template.Reference()
	vmFolderRef := vmFolder.Reference()
	resourcePoolRef := resourcePool.Reference()
	storagePodRef := storagePod.Reference()

	var o mo.VirtualMachine
	err := template.Properties(context.TODO(), templateRef, []string{"datastore"}, &o)
	if err != nil {
		return nil, err
	}
	templateDatastore := object.NewDatastore(c.Client, o.Datastore[0])
	log.Printf("[DEBUG] %#v\n", templateDatastore)

	devices, err := template.Device(context.TODO())
	if err != nil {
		return nil, err
	}

	var key int
	for _, d := range devices.SelectByType((*types.VirtualDisk)(nil)) {
		key = d.GetVirtualDevice().Key
		log.Printf("[DEBUG] %#v\n", d.GetVirtualDevice())
	}

	sps := types.StoragePlacementSpec{
		Type: "clone",
		Vm:   &templateRef,
		PodSelectionSpec: types.StorageDrsPodSelectionSpec{
			StoragePod: &storagePodRef,
		},
		CloneSpec: &types.VirtualMachineCloneSpec{
			Location: types.VirtualMachineRelocateSpec{
				Disk: []types.VirtualMachineRelocateSpecDiskLocator{
					types.VirtualMachineRelocateSpecDiskLocator{
						Datastore:       templateDatastore.Reference(),
						DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{},
						DiskId:          key,
					},
				},
				Pool: &resourcePoolRef,
			},
			PowerOn:  false,
			Template: false,
		},
		CloneName: "dummy",
		Folder:    &vmFolderRef,
	}
	log.Printf("[DEBUG] findDatastoreForClone: StoragePlacementSpec: %v", sps)

	srm := object.NewStorageResourceManager(c.Client)
	result, err := srm.RecommendDatastores(context.TODO(), sps)
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] findDatastoreForClone: result: %v", result)
	spa := result.Recommendations[0].Action[0].(*types.StoragePlacementAction)
	datastore := object.NewDatastore(c.Client, spa.Destination)

	return datastore, nil
}

// findDatastore finds Datastore object.
func findDatastore(c *govmomi.Client, finder *find.Finder, f *object.DatacenterFolders, template *object.VirtualMachine, resourcePool *object.ResourcePool, name string) (*object.Datastore, error) {
	if name == "" {
		datastore, err := finder.DefaultDatastore(context.TODO())
		if err != nil {
			return nil, err
		}
		log.Printf("[DEBUG] findDatastore: datastore: %#v", datastore)
		return datastore, nil
	} else {
		var datastore *object.Datastore
		s := object.NewSearchIndex(c.Client)
		ref, err := s.FindChild(context.TODO(), f.DatastoreFolder, name)
		if err != nil {
			return nil, err
		}
		log.Printf("[DEBUG] findDatastore: reference: %#v", ref)

		mor := ref.Reference()
		if mor.Type == "StoragePod" {
			s := object.NewFolder(c.Client, mor)
			datastore, err = findDatastoreForClone(c, s, template, f.VmFolder, resourcePool)
			if err != nil {
				return nil, err
			}
		} else {
			datastore = object.NewDatastore(c.Client, mor)
		}
		log.Printf("[DEBUG] findDatastore: datastore: %#v", datastore)
		return datastore, nil
	}
}

func networkDevice(f *find.Finder, label string) (*types.VirtualDeviceConfigSpec, error) {
	network, err := f.Network(context.TODO(), "*"+label)
	if err != nil {
		return nil, err
	}
	backing, err := network.EthernetCardBackingInfo(context.TODO())
	if err != nil {
		return nil, err
	}

	d := types.VirtualDeviceConfigSpec{
		Operation: types.VirtualDeviceConfigSpecOperationAdd,
		Device: &types.VirtualVmxnet3{
			types.VirtualVmxnet{
				types.VirtualEthernetCard{
					VirtualDevice: types.VirtualDevice{
						Key:     -1,
						Backing: backing,
					},
					AddressType: string(types.VirtualEthernetCardMacTypeGenerated),
				},
			},
		},
	}
	return &d, nil
}

// findDatacenter finds Datacenter object.
func findDatacenter(f *find.Finder, name string) (*object.Datacenter, error) {
	if name != "" {
		dc, err := f.Datacenter(context.TODO(), name)
		if err != nil {
			return nil, err
		}
		return dc, nil
	} else {
		dc, err := f.DefaultDatacenter(context.TODO())
		if err != nil {
			return nil, err
		}
		return dc, nil
	}
}

// findResourcePool finds ResourcePool object
func findResourcePool(f *find.Finder, name, cluster string) (*object.ResourcePool, error) {
	if name == "" {
		if cluster == "" {
			resourcePool, err := f.DefaultResourcePool(context.TODO())
			if err != nil {
				return nil, err
			}
			return resourcePool, nil
		} else {
			resourcePool, err := f.ResourcePool(context.TODO(), "*"+cluster+"/Resources")
			if err != nil {
				return nil, err
			}
			return resourcePool, nil
		}
	} else {
		resourcePool, err := f.ResourcePool(context.TODO(), name)
		if err != nil {
			return nil, err
		}
		return resourcePool, nil
	}
}

func getVMRelocateSpec(rp *object.ResourcePool, ds *object.Datastore, vm *object.VirtualMachine) (types.VirtualMachineRelocateSpec, error) {
	var key int

	devices, err := vm.Device(context.TODO())
	if err != nil {
		return types.VirtualMachineRelocateSpec{}, err
	}
	for _, d := range devices {
		if devices.Type(d) == "disk" {
			key = d.GetVirtualDevice().Key
		}
	}

	rpr := rp.Reference()
	dsr := ds.Reference()
	return types.VirtualMachineRelocateSpec{
		Datastore: &dsr,
		Pool:      &rpr,
		Disk: []types.VirtualMachineRelocateSpecDiskLocator{
			types.VirtualMachineRelocateSpecDiskLocator{
				Datastore: dsr,
				DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
					DiskMode:        "persistent",
					ThinProvisioned: types.NewBool(false),
					EagerlyScrub:    types.NewBool(true),
				},
				DiskId: key,
			},
		},
	}, nil
}

// createCustomizationSpec creates the CustomizationSpec object.
func createCustomizationSpec(name, domain, tz string, suffixes, servers []string, nics []types.CustomizationAdapterMapping) types.CustomizationSpec {
	return types.CustomizationSpec{
		Identity: &types.CustomizationLinuxPrep{
			HostName: &types.CustomizationFixedName{
				Name: name,
			},
			Domain:     domain,
			TimeZone:   tz,
			HwClockUTC: types.NewBool(true),
		},
		GlobalIPSettings: types.CustomizationGlobalIPSettings{
			DnsSuffixList: suffixes,
			DnsServerList: servers,
		},
		NicSettingMap: nics,
	}
}

func addHardDisk(vm *object.VirtualMachine, size, iops int64) error {
	devices, err := vm.Device(context.TODO())
	if err != nil {
		return err
	}

	controller, err := devices.FindDiskController("scsi")
	if err != nil {
		log.Printf("[ERROR] %s", err)
	}
	log.Printf("[DEBUG] %#v\n", controller)

	disk := devices.CreateDisk(controller, "")

	existing := devices.SelectByBackingInfo(disk.Backing)
	log.Printf("[DEBUG] %#v\n", existing)

	if len(existing) == 0 {
		disk.CapacityInKB = int64(size * 1024 * 1024)
		disk.StorageIOAllocation = &types.StorageIOAllocationInfo{
			Limit: iops,
		}
	} else {
		log.Printf("Disk already present\n")
	}

	backing := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)

	// eager zeroed thick virtual disk
	backing.ThinProvisioned = types.NewBool(false)
	backing.EagerlyScrub = types.NewBool(true)

	return vm.AddDevice(context.TODO(), disk)
}
