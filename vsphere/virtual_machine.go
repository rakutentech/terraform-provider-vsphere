package vsphere

import (
	"log"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

const (
	DefaultTimeZone = "Etc/UTC"
	DefaultDomain   = "vsphere.local"
)

type NetworkInterface struct {
	DeviceName string
	Label      string
	IPAddress  string
	SubnetMask string
}

type VirtualMachine struct {
	Name              string
	Datacenter        string
	Cluster           string
	ResourcePool      string
	Datastore         string
	VCPU              int
	MemoryMB          int64
	Template          string
	NetworkInterfaces []NetworkInterface
	Gateway           string
	Domain            string
	DNSSuffixes       []string
	DNSServers        []string
}

func (vm *VirtualMachine) RunVirtualMachine(c *govmomi.Client) error {
	if len(vm.DNSServers) == 0 {
		vm.DNSServers = []string{
			"8.8.8.8",
			"8.8.4.4",
		}
	}

	if len(vm.DNSSuffixes) == 0 {
		vm.DNSSuffixes = []string{
			DefaultDomain,
		}
	}

	if vm.Domain == "" {
		vm.Domain = DefaultDomain
	}

	finder := find.NewFinder(c.Client, true)
	dc, err := getDatacenter(finder, vm.Datacenter)
	if err != nil {
		return err
	}
	finder = finder.SetDatacenter(dc)
	dcFolders, err := dc.Folders(context.TODO())
	if err != nil {
		return err
	}

	vmFolder := dcFolders.VmFolder
	template, err := getVirtualMachine(c, vmFolder, vm.Template)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] template: %v", template)

	var datastore *object.Datastore
	if vm.Datastore == "" {
		datastore, err = finder.DefaultDatastore(context.TODO())
		if err != nil {
			return err
		}
		log.Printf("[DEBUG] datastore: %v", datastore)
	} else {
		storage, err := getStorage(c, dcFolders.DatastoreFolder, vm.Datastore)
		if err != nil {
			return err
		}
		log.Printf("[DEBUG] storage: %v", storage)

		if storage.Type == "StoragePod" {
			datastore, err = findDatastoreForClone(c, storage, template, vmFolder)
			if err != nil {
				return err
			}
		}
		log.Printf("[DEBUG] datastore: %v", datastore)
	}

	// find ResourcePool object
	var resourcePool *object.ResourcePool
	if vm.ResourcePool == "" {
		if vm.Cluster == "" {
			resourcePool, err = finder.DefaultResourcePool(context.TODO())
			if err != nil {
				return err
			}
		} else {
			resourcePool, err = finder.ResourcePool(context.TODO(), "*"+vm.Cluster+"/Resources")
			if err != nil {
				return err
			}
		}
		log.Printf("[DEBUG] resource pool: %v", resourcePool)
	} else {
		resourcePool, err = finder.ResourcePool(context.TODO(), vm.ResourcePool)
		if err != nil {
			return err
		}
	}

	relocateSpec, err := getVMRelocateSpec(resourcePool, datastore, template)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] relocate spec: %v", relocateSpec)

	// network
	networkDevices := []types.BaseVirtualDeviceConfigSpec{}
	networkConfigs := []types.CustomizationAdapterMapping{}
	for _, network := range vm.NetworkInterfaces {
		// network device
		device, err := networkDevice(finder, network.Label)
		if err != nil {
			return err
		}
		networkDevices = append(networkDevices, device)

		var ipSetting types.CustomizationIPSettings
		if network.IPAddress == "" {
			ipSetting = types.CustomizationIPSettings{
				Ip: &types.CustomizationDhcpIpGenerator{},
			}
		} else {
			ipSetting = types.CustomizationIPSettings{
				Gateway: []string{
					vm.Gateway,
				},
				Ip: &types.CustomizationFixedIp{
					IpAddress: network.IPAddress,
				},
				SubnetMask: network.SubnetMask,
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
		NumCPUs:           vm.VCPU,
		NumCoresPerSocket: 1,
		MemoryMB:          vm.MemoryMB,
		DeviceChange:      networkDevices,
	}
	log.Printf("[DEBUG] virtual machine config spec: %v", configSpec)

	// make custom spec
	customSpec := createCustomizationSpec(vm.Name, vm.Domain, vm.DNSSuffixes, vm.DNSServers, networkConfigs)
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

	task, err := template.Clone(context.TODO(), vmFolder, vm.Name, cloneSpec)
	if err != nil {
		return err
	}

	err = task.Wait(context.TODO())
	if err != nil {
		return err
	}

	newVM, err := getVirtualMachine(c, vmFolder, vm.Name)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] new vm: %v", newVM)

	ip, err := newVM.WaitForIP(context.TODO())
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] ip address: %v", ip)

	return nil
}

func findDatastoreForClone(c *govmomi.Client, d *types.ManagedObjectReference, t *object.VirtualMachine, f *object.Folder) (*object.Datastore, error) {
	tr := t.Reference()
	fr := f.Reference()

	sps := types.StoragePlacementSpec{
		Type: "clone",
		Vm:   &tr,
		PodSelectionSpec: types.StorageDrsPodSelectionSpec{
			StoragePod: d,
		},
		CloneSpec: &types.VirtualMachineCloneSpec{
			Location: types.VirtualMachineRelocateSpec{},
			PowerOn:  false,
			Template: false,
		},
		CloneName: "dummy",
		Folder:    &fr,
	}

	srm := object.NewStorageResourceManager(c.Client)
	result, err := srm.RecommendDatastores(context.TODO(), sps)
	if err != nil {
		return nil, err
	}
	spa := result.Recommendations[0].Action[0].(*types.StoragePlacementAction)
	datastore := object.NewDatastore(c.Client, spa.Destination)

	return datastore, nil
}

func networkDevice(f *find.Finder, label string) (*types.VirtualDeviceConfigSpec, error) {
	network, err := f.NetworkList(context.TODO(), "*"+label)
	if err != nil {
		return nil, err
	}
	backing, err := network[0].EthernetCardBackingInfo(context.TODO())
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

// getDatacenter gets Datacenter object.
func getDatacenter(f *find.Finder, name string) (*object.Datacenter, error) {
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

func getStorage(c *govmomi.Client, f *object.Folder, name string) (*types.ManagedObjectReference, error) {
	s := object.NewSearchIndex(c.Client)
	storageRef, err := s.FindChild(context.TODO(), f, name)
	if err != nil {
		return nil, err
	}
	storage := storageRef.Reference()
	return &storage, nil
}

// getVirtualMachine finds VirtualMachine or Template object
func getVirtualMachine(c *govmomi.Client, f *object.Folder, name string) (*object.VirtualMachine, error) {
	s := object.NewSearchIndex(c.Client)
	vmRef, err := s.FindChild(context.TODO(), f, name)
	if err != nil {
		return nil, err
	}
	vm := object.NewVirtualMachine(c.Client, vmRef.Reference())
	return vm, nil
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
					ThinProvisioned: false,
					EagerlyScrub:    true,
				},
				DiskId: key,
			},
		},
	}, nil
}

// createCustomizationSpec creates the CustomizationSpec object.
func createCustomizationSpec(name, domain string, suffixes, servers []string, nics []types.CustomizationAdapterMapping) types.CustomizationSpec {
	return types.CustomizationSpec{
		Identity: &types.CustomizationLinuxPrep{
			HostName: &types.CustomizationFixedName{
				Name: name,
			},
			Domain:     domain,
			TimeZone:   DefaultTimeZone,
			HwClockUTC: true,
		},
		GlobalIPSettings: types.CustomizationGlobalIPSettings{
			DnsSuffixList: suffixes,
			DnsServerList: servers,
		},
		NicSettingMap: nics,
	}
}
