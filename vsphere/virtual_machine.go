package vsphere

import (
	"fmt"
	"log"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

type networkInterface struct {
	deviceName  string
	label       string
	ipAddress   string
	subnetMask  string
	adapterType string // TODO: Make "adapter_type" argument
}

type hardDisk struct {
	size int64
	iops int64
}

type virtualMachine struct {
	name              string
	datacenter        string
	cluster           string
	resourcePool      string
	datastore         string
	vcpu              int
	memoryMb          int64
	template          string
	networkInterfaces []networkInterface
	hardDisks         []hardDisk
	gateway           string
	domain            string
	timeZone          string
	dnsSuffixes       []string
	dnsServers        []string
}

// createVirtualMchine creates a new VirtualMachine.
func (vm *virtualMachine) createVirtualMachine(c *govmomi.Client) error {
	var dc *object.Datacenter
	var err error

	finder := find.NewFinder(c.Client, true)

	if vm.datacenter != "" {
		dc, err = finder.Datacenter(context.TODO(), vm.datacenter)
		if err != nil {
			return err
		}
	} else {
		dc, err = finder.DefaultDatacenter(context.TODO())
		if err != nil {
			return err
		}
	}
	finder = finder.SetDatacenter(dc)

	var resourcePool *object.ResourcePool
	if vm.resourcePool == "" {
		if vm.cluster == "" {
			resourcePool, err = finder.DefaultResourcePool(context.TODO())
			if err != nil {
				return err
			}
		} else {
			resourcePool, err = finder.ResourcePool(context.TODO(), "*"+vm.cluster+"/Resources")
			if err != nil {
				return err
			}
		}
	} else {
		resourcePool, err = finder.ResourcePool(context.TODO(), vm.resourcePool)
		if err != nil {
			return err
		}
	}
	log.Printf("[DEBUG] resource pool: %#v", resourcePool)

	dcFolders, err := dc.Folders(context.TODO())
	if err != nil {
		return err
	}

	// network
	networkDevices := []types.BaseVirtualDeviceConfigSpec{}
	for _, network := range vm.networkInterfaces {
		// network device
		nd, err := createNetworkDevice(finder, network.label, "e1000")
		if err != nil {
			return err
		}
		networkDevices = append(networkDevices, nd)
	}

	// make config spec
	configSpec := types.VirtualMachineConfigSpec{
		GuestId:           "otherLinux64Guest",
		Name:              vm.name,
		NumCPUs:           vm.vcpu,
		NumCoresPerSocket: 1,
		MemoryMB:          vm.memoryMb,
		DeviceChange:      networkDevices,
	}
	log.Printf("[DEBUG] virtual machine config spec: %v", configSpec)

	var datastore *object.Datastore
	if vm.datastore == "" {
		datastore, err = finder.DefaultDatastore(context.TODO())
		if err != nil {
			return err
		}
	} else {
		s := object.NewSearchIndex(c.Client)
		ref, err := s.FindChild(context.TODO(), dcFolders.DatastoreFolder, vm.datastore)
		if err != nil {
			return err
		}
		log.Printf("[DEBUG] findDatastore: reference: %#v", ref)

		mor := ref.Reference()
		if mor.Type == "StoragePod" {
			storagePod := object.NewFolder(c.Client, mor)

			vmfr := dcFolders.VmFolder.Reference()
			rpr := resourcePool.Reference()
			spr := storagePod.Reference()

			sps := types.StoragePlacementSpec{
				Type:       "create",
				ConfigSpec: &configSpec,
				PodSelectionSpec: types.StorageDrsPodSelectionSpec{
					StoragePod: &spr,
				},
				Folder:       &vmfr,
				ResourcePool: &rpr,
			}
			log.Printf("[DEBUG] findDatastore: StoragePlacementSpec: %#v\n", sps)

			srm := object.NewStorageResourceManager(c.Client)
			rds, err := srm.RecommendDatastores(context.TODO(), sps)
			if err != nil {
				return err
			}
			log.Printf("[DEBUG] findDatastore: recommendDatastores: %#v\n", rds)

			spa := rds.Recommendations[0].Action[0].(*types.StoragePlacementAction)
			datastore = object.NewDatastore(c.Client, spa.Destination)
			if err != nil {
				return err
			}
		} else {
			datastore = object.NewDatastore(c.Client, mor)
		}
	}
	log.Printf("[DEBUG] datastore: %#v", datastore)

	var mds mo.Datastore
	if err = datastore.Properties(context.TODO(), datastore.Reference(), []string{"name"}, &mds); err != nil {
		return err
	}
	log.Printf("[DEBUG] datastore: %#v", mds.Name)
	scsi, err := object.SCSIControllerTypes().CreateSCSIController("scsi")
	if err != nil {
		log.Printf("[ERROR] %s", err)
	}

	configSpec.DeviceChange = append(configSpec.DeviceChange, &types.VirtualDeviceConfigSpec{
		Operation: types.VirtualDeviceConfigSpecOperationAdd,
		Device:    scsi,
	})
	configSpec.Files = &types.VirtualMachineFileInfo{VmPathName: fmt.Sprintf("[%s]", mds.Name)}

	task, err := dcFolders.VmFolder.CreateVM(context.TODO(), configSpec, resourcePool, nil)
	if err != nil {
		log.Printf("[ERROR] %s", err)
	}

	err = task.Wait(context.TODO())
	if err != nil {
		log.Printf("[ERROR] %s", err)
	}

	newVM, err := finder.VirtualMachine(context.TODO(), vm.name)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] new vm: %v", newVM)

	log.Printf("[DEBUG] add hard disk: %v", vm.hardDisks)
	for _, hd := range vm.hardDisks {
		log.Printf("[DEBUG] add hard disk: %v", hd.size)
		log.Printf("[DEBUG] add hard disk: %v", hd.iops)
		err = addHardDisk(newVM, hd.size, hd.iops, "thin")
		if err != nil {
			return err
		}
	}
	return nil
}

// deployVirtualMchine deploys a new VirtualMachine.
func (vm *virtualMachine) deployVirtualMachine(c *govmomi.Client) error {
	var dc *object.Datacenter
	var err error

	finder := find.NewFinder(c.Client, true)

	if vm.datacenter != "" {
		dc, err = finder.Datacenter(context.TODO(), vm.datacenter)
		if err != nil {
			return err
		}
	} else {
		dc, err = finder.DefaultDatacenter(context.TODO())
		if err != nil {
			return err
		}
	}
	finder = finder.SetDatacenter(dc)

	template, err := finder.VirtualMachine(context.TODO(), vm.template)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] template: %#v", template)

	var resourcePool *object.ResourcePool
	if vm.resourcePool == "" {
		if vm.cluster == "" {
			resourcePool, err = finder.DefaultResourcePool(context.TODO())
			if err != nil {
				return err
			}
		} else {
			resourcePool, err = finder.ResourcePool(context.TODO(), "*"+vm.cluster+"/Resources")
			if err != nil {
				return err
			}
		}
	} else {
		resourcePool, err = finder.ResourcePool(context.TODO(), vm.resourcePool)
		if err != nil {
			return err
		}
	}
	log.Printf("[DEBUG] resource pool: %#v", resourcePool)

	dcFolders, err := dc.Folders(context.TODO())
	if err != nil {
		return err
	}

	var datastore *object.Datastore
	if vm.datastore == "" {
		datastore, err = finder.DefaultDatastore(context.TODO())
		if err != nil {
			return err
		}
	} else {
		datastore, err = finder.Datastore(context.TODO(), vm.datastore)
		if err != nil {
			// TODO: datastore cluster support in govmomi finder function
			datastore, err = findDatastore(c, dcFolders, template, resourcePool, vm.datastore)
			if err != nil {
				return err
			}
		}
	}
	log.Printf("[DEBUG] datastore: %#v", datastore)

	relocateSpec, err := createVMRelocateSpec(resourcePool, datastore, template)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] relocate spec: %v", relocateSpec)

	// network
	networkDevices := []types.BaseVirtualDeviceConfigSpec{}
	networkConfigs := []types.CustomizationAdapterMapping{}
	for _, network := range vm.networkInterfaces {
		// network device
		nd, err := createNetworkDevice(finder, network.label, "vmxnet3")
		if err != nil {
			return err
		}
		networkDevices = append(networkDevices, nd)

		var ipSetting types.CustomizationIPSettings
		if network.ipAddress == "" {
			ipSetting = types.CustomizationIPSettings{
				Ip: &types.CustomizationDhcpIpGenerator{},
			}
		} else {
			log.Printf("[DEBUG] gateway: %v", vm.gateway)
			log.Printf("[DEBUG] ip address: %v", network.ipAddress)
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

	// create CustomizationSpec
	customSpec := types.CustomizationSpec{
		Identity: &types.CustomizationLinuxPrep{
			HostName: &types.CustomizationFixedName{
				Name: vm.name,
			},
			Domain:     vm.domain,
			TimeZone:   vm.timeZone,
			HwClockUTC: types.NewBool(true),
		},
		GlobalIPSettings: types.CustomizationGlobalIPSettings{
			DnsSuffixList: vm.dnsSuffixes,
			DnsServerList: vm.dnsServers,
		},
		NicSettingMap: networkConfigs,
	}
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

	for i := 1; i < len(vm.hardDisks); i++ {
		err = addHardDisk(newVM, vm.hardDisks[i].size, vm.hardDisks[i].iops, "eager_zeroed")
		if err != nil {
			return err
		}
	}
	return nil
}

// findDatastore finds Datastore object.
func findDatastore(c *govmomi.Client, f *object.DatacenterFolders, vm *object.VirtualMachine, rp *object.ResourcePool, name string) (*object.Datastore, error) {
	var datastore *object.Datastore
	s := object.NewSearchIndex(c.Client)
	ref, err := s.FindChild(context.TODO(), f.DatastoreFolder, name)
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] findDatastore: reference: %#v", ref)

	mor := ref.Reference()
	if mor.Type == "StoragePod" {
		storagePod := object.NewFolder(c.Client, mor)

		vmr := vm.Reference()
		vmfr := f.VmFolder.Reference()
		rpr := rp.Reference()
		spr := storagePod.Reference()

		var o mo.VirtualMachine
		err := vm.Properties(context.TODO(), vmr, []string{"datastore"}, &o)
		if err != nil {
			return nil, err
		}
		ds := object.NewDatastore(c.Client, o.Datastore[0])
		log.Printf("[DEBUG] findDatastore: datastore: %#v\n", ds)

		devices, err := vm.Device(context.TODO())
		if err != nil {
			return nil, err
		}

		var key int
		for _, d := range devices.SelectByType((*types.VirtualDisk)(nil)) {
			key = d.GetVirtualDevice().Key
			log.Printf("[DEBUG] findDatastore: virtual devices: %#v\n", d.GetVirtualDevice())
		}

		sps := types.StoragePlacementSpec{
			Type: "clone",
			Vm:   &vmr,
			PodSelectionSpec: types.StorageDrsPodSelectionSpec{
				StoragePod: &spr,
			},
			CloneSpec: &types.VirtualMachineCloneSpec{
				Location: types.VirtualMachineRelocateSpec{
					Disk: []types.VirtualMachineRelocateSpecDiskLocator{
						types.VirtualMachineRelocateSpecDiskLocator{
							Datastore:       ds.Reference(),
							DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{},
							DiskId:          key,
						},
					},
					Pool: &rpr,
				},
				PowerOn:  false,
				Template: false,
			},
			CloneName: "dummy",
			Folder:    &vmfr,
		}
		log.Printf("[DEBUG] findDatastore: StoragePlacementSpec: %#v\n", sps)

		srm := object.NewStorageResourceManager(c.Client)
		rds, err := srm.RecommendDatastores(context.TODO(), sps)
		if err != nil {
			return nil, err
		}
		log.Printf("[DEBUG] findDatastore: recommendDatastores: %#v\n", rds)

		spa := rds.Recommendations[0].Action[0].(*types.StoragePlacementAction)
		datastore = object.NewDatastore(c.Client, spa.Destination)
	} else {
		datastore = object.NewDatastore(c.Client, mor)
	}
	log.Printf("[DEBUG] findDatastore: datastore: %#v", datastore)

	return datastore, nil
}

// createNetworkDevice creates VirtualDeviceConfigSpec for Network Device.
func createNetworkDevice(f *find.Finder, label, adapterType string) (*types.VirtualDeviceConfigSpec, error) {
	network, err := f.Network(context.TODO(), "*"+label)
	if err != nil {
		return nil, err
	}

	backing, err := network.EthernetCardBackingInfo(context.TODO())
	if err != nil {
		return nil, err
	}

	if adapterType == "vmxnet3" {
		return &types.VirtualDeviceConfigSpec{
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
		}, nil
	} else if adapterType == "e1000" {
		return &types.VirtualDeviceConfigSpec{
			Operation: types.VirtualDeviceConfigSpecOperationAdd,
			Device: &types.VirtualE1000{
				types.VirtualEthernetCard{
					VirtualDevice: types.VirtualDevice{
						Key:     -1,
						Backing: backing,
					},
					AddressType: string(types.VirtualEthernetCardMacTypeGenerated),
				},
			},
		}, nil
	} else {
		return nil, fmt.Errorf("Invalid network adapter type.")
	}
}

// createVMRelocateSpec creates VirtualMachineRelocateSpec to set a place for a new VirtualMachine.
func createVMRelocateSpec(rp *object.ResourcePool, ds *object.Datastore, vm *object.VirtualMachine) (types.VirtualMachineRelocateSpec, error) {
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

// addHardDisk adds a new Hard Disk to the VirtualMachine.
func addHardDisk(vm *object.VirtualMachine, size, iops int64, diskType string) error {
	devices, err := vm.Device(context.TODO())
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] vm devices: %#v\n", devices)

	controller, err := devices.FindDiskController("scsi")
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] disk controller: %#v\n", controller)

	disk := devices.CreateDisk(controller, "")
	existing := devices.SelectByBackingInfo(disk.Backing)
	log.Printf("[DEBUG] disk: %#v\n", disk)

	if len(existing) == 0 {
		disk.CapacityInKB = int64(size * 1024 * 1024)
		if iops != 0 {
			disk.StorageIOAllocation = &types.StorageIOAllocationInfo{
				Limit: iops,
			}
		}
		backing := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)

		if diskType == "eager_zeroed" {
			// eager zeroed thick virtual disk
			backing.ThinProvisioned = types.NewBool(false)
			backing.EagerlyScrub = types.NewBool(true)
		} else if diskType == "thin" {
			// thin provisioned virtual disk
			backing.ThinProvisioned = types.NewBool(true)
		}

		log.Printf("[DEBUG] addHardDisk: %#v\n", disk)
		log.Printf("[DEBUG] addHardDisk: %#v\n", disk.CapacityInKB)

		return vm.AddDevice(context.TODO(), disk)
	} else {
		log.Printf("[DEBUG] addHardDisk: Disk already present.\n")

		return nil
	}
}
