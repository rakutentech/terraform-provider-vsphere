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

func (vm *VirtualMachine) deployVirtualMachine(c *govmomi.Client) error {
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
	log.Printf("[DEBUG] template: %#v", template)

	resourcePool, err := getResourcePool(finder, vm.ResourcePool, vm.Cluster)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] resource pool: %#v", resourcePool)

	datastore, err := getDatastore(c, finder, dcFolders.DatastoreFolder, dcFolders.VmFolder, template, resourcePool, vm.Datastore)
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

// getDatastore gets Datastore object.
func getDatastore(c *govmomi.Client, finder *find.Finder, datastoreFolder, vmFolder *object.Folder, template *object.VirtualMachine, resourcePool *object.ResourcePool, name string) (*object.Datastore, error) {
	if name == "" {
		datastore, err := finder.DefaultDatastore(context.TODO())
		if err != nil {
			return nil, err
		}
		log.Printf("[DEBUG] getDatastore: datastore: %#v", datastore)
		return datastore, nil
	} else {
		var datastore *object.Datastore
		s := object.NewSearchIndex(c.Client)
		ref, err := s.FindChild(context.TODO(), datastoreFolder, name)
		if err != nil {
			return nil, err
		}
		log.Printf("[DEBUG] getDatastore: reference: %#v", ref)

		mor := ref.Reference()
		if mor.Type == "StoragePod" {
			s := object.NewFolder(c.Client, mor)
			datastore, err = findDatastoreForClone(c, s, template, vmFolder, resourcePool)
			if err != nil {
				return nil, err
			}
		} else {
			datastore = object.NewDatastore(c.Client, mor)
		}
		log.Printf("[DEBUG] getDatastore: datastore: %#v", datastore)
		return datastore, nil
	}
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

// getResourcePool finds ResourcePool object
func getResourcePool(f *find.Finder, name, cluster string) (*object.ResourcePool, error) {
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
