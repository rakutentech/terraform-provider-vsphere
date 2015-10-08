package vsphere

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

var DefaultDNSSuffixes = []string{
	"vsphere.local",
}

var DefaultDNSServers = []string{
	"8.8.8.8",
	"8.8.4.4",
}

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

func resourceVSphereVirtualMachine() *schema.Resource {
	return &schema.Resource{
		Create: resourceVSphereVirtualMachineCreate,
		Read:   resourceVSphereVirtualMachineRead,
		Update: resourceVSphereVirtualMachineUpdate,
		Delete: resourceVSphereVirtualMachineDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"vcpu": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: false,
			},

			"memory": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: false,
			},

			"datacenter": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"cluster": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"resource_pool": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"gateway": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"domain": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
				Default:  "vsphere.local",
			},

			"time_zone": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
				Default:  "Etc/UTC",
			},

			"dns_suffix": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				ForceNew: false,
			},

			"dns_server": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				ForceNew: false,
			},

			"network_interface": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"label": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: false,
						},

						"ip_address": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ForceNew: false,
						},

						"subnet_mask": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ForceNew: false,
						},

						"adapter_type": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
						},
					},
				},
			},

			"disk": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"template": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
						},

						"datastore": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
						},

						"size": &schema.Schema{
							Type:     schema.TypeInt,
							Optional: true,
							ForceNew: false,
						},

						"iops": &schema.Schema{
							Type:     schema.TypeInt,
							Optional: true,
							ForceNew: false,
						},
					},
				},
			},

			"boot_delay": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
			},
		},
	}
}

func resourceVSphereVirtualMachineCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govmomi.Client)

	vm := virtualMachine{
		name:     d.Get("name").(string),
		vcpu:     d.Get("vcpu").(int),
		memoryMb: int64(d.Get("memory").(int)),
	}

	if v, ok := d.GetOk("datacenter"); ok {
		vm.datacenter = v.(string)
	}

	if v, ok := d.GetOk("cluster"); ok {
		vm.cluster = v.(string)
	}

	if v, ok := d.GetOk("resource_pool"); ok {
		vm.resourcePool = v.(string)
	}

	if v, ok := d.GetOk("gateway"); ok {
		vm.gateway = v.(string)
	}

	if v, ok := d.GetOk("domain"); ok {
		vm.domain = v.(string)
	}

	if v, ok := d.GetOk("time_zone"); ok {
		vm.timeZone = v.(string)
	}

	dns_suffix := d.Get("dns_suffix.#").(int)
	if dns_suffix > 0 {
		vm.dnsSuffixes = make([]string, 0, dns_suffix)
		for i := 0; i < dns_suffix; i++ {
			s := fmt.Sprintf("dns_suffix.%d", i)
			vm.dnsSuffixes = append(vm.dnsSuffixes, d.Get(s).(string))
		}
	} else {
		vm.dnsSuffixes = DefaultDNSSuffixes
	}

	dns_server := d.Get("dns_server.#").(int)
	if dns_server > 0 {
		vm.dnsServers = make([]string, 0, dns_server)
		for i := 0; i < dns_server; i++ {
			s := fmt.Sprintf("dns_server.%d", i)
			vm.dnsServers = append(vm.dnsServers, d.Get(s).(string))
		}
	} else {
		vm.dnsServers = DefaultDNSServers
	}

	networksCount := d.Get("network_interface.#").(int)
	networks := make([]networkInterface, networksCount)
	for i := 0; i < networksCount; i++ {
		prefix := fmt.Sprintf("network_interface.%d", i)
		networks[i].label = d.Get(prefix + ".label").(string)
		if v, ok := d.GetOk(prefix + ".ip_address"); ok {
			networks[i].ipAddress = v.(string)
		}
		if v, ok := d.GetOk(prefix + ".subnet_mask"); ok {
			networks[i].subnetMask = v.(string)
		}
	}
	vm.networkInterfaces = networks
	log.Printf("[DEBUG] network_interface init: %v", networks)

	diskCount := d.Get("disk.#").(int)
	disks := make([]hardDisk, diskCount)
	for i := 0; i < diskCount; i++ {
		prefix := fmt.Sprintf("disk.%d", i)
		if i == 0 {
			if v, ok := d.GetOk(prefix + ".template"); ok {
				vm.template = v.(string)
			} else {
				if v, ok := d.GetOk(prefix + ".size"); ok {
					disks[i].size = int64(v.(int))
				} else {
					return fmt.Errorf("If template argument is not specified, size argument is required.")
				}
			}
			if v, ok := d.GetOk(prefix + ".datastore"); ok {
				vm.datastore = v.(string)
			}
		} else {
			if v, ok := d.GetOk(prefix + ".size"); ok {
				disks[i].size = int64(v.(int))
			} else {
				return fmt.Errorf("Size argument is required.")
			}
		}
		if v, ok := d.GetOk(prefix + ".iops"); ok {
			disks[i].iops = int64(v.(int))
		}
	}
	vm.hardDisks = disks
	log.Printf("[DEBUG] disk init: %v", disks)

	if vm.template != "" {
		err := vm.deployVirtualMachine(client)
		if err != nil {
			return fmt.Errorf("error: %s", err)
		}
	} else {
		err := vm.createVirtualMachine(client)
		if err != nil {
			return fmt.Errorf("error: %s", err)
		}
	}

	if _, ok := d.GetOk("network_interface.0.ip_address"); !ok {
		if v, ok := d.GetOk("boot_delay"); ok {
			stateConf := &resource.StateChangeConf{
				Pending:    []string{"pending"},
				Target:     "active",
				Refresh:    waitForNetworkingActive(client, vm.datacenter, vm.name),
				Timeout:    600 * time.Second,
				Delay:      time.Duration(v.(int)) * time.Second,
				MinTimeout: 2 * time.Second,
			}

			_, err := stateConf.WaitForState()
			if err != nil {
				return fmt.Errorf("error: %s", err)
			}
		}
	}
	d.SetId(vm.name)
	log.Printf("[INFO] Created virtual machine: %s", d.Id())

	return resourceVSphereVirtualMachineRead(d, meta)
}

func resourceVSphereVirtualMachineRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govmomi.Client)
	dc, err := getDatacenter(client, d.Get("datacenter").(string))
	if err != nil {
		return err
	}
	finder := find.NewFinder(client.Client, true)
	finder = finder.SetDatacenter(dc)

	vm, err := finder.VirtualMachine(context.TODO(), d.Get("name").(string))
	if err != nil {
		log.Printf("[ERROR] Virtual machine not found: %s", d.Get("name").(string))
		d.SetId("")
		return nil
	}

	var mvm mo.VirtualMachine

	collector := property.DefaultCollector(client.Client)
	if err := collector.RetrieveOne(context.TODO(), vm.Reference(), []string{"guest", "summary", "datastore"}, &mvm); err != nil {
		log.Printf("[ERROR] %#v", err)
	}

	log.Printf("[DEBUG] %#v", dc)
	log.Printf("[DEBUG] %#v", mvm.Summary.Config)
	log.Printf("[DEBUG] %#v", mvm.Guest.Net)

	networkInterfaces := make([]map[string]interface{}, 0)
	for _, v := range mvm.Guest.Net {
		if v.DeviceConfigId >= 0 {
			log.Printf("[DEBUG] %#v", v.Network)
			networkInterface := make(map[string]interface{})
			networkInterface["label"] = v.Network
			if len(v.IpAddress) > 0 {
				log.Printf("[DEBUG] %#v", v.IpAddress[0])
				networkInterface["ip_address"] = v.IpAddress[0]

				m := net.CIDRMask(v.IpConfig.IpAddress[0].PrefixLength, 32)
				subnetMask := net.IPv4(m[0], m[1], m[2], m[3])
				networkInterface["subnet_mask"] = subnetMask.String()
				log.Printf("[DEBUG] %#v", subnetMask.String())
			}
			networkInterfaces = append(networkInterfaces, networkInterface)
		}
	}
	d.Set("network_interface", networkInterfaces)

	var rootDatastore string
	for _, v := range mvm.Datastore {
		var md mo.Datastore
		if err := collector.RetrieveOne(context.TODO(), v, []string{"name", "parent"}, &md); err != nil {
			log.Printf("[ERROR] %#v", err)
		}
		if md.Parent.Type == "StoragePod" {
			var msp mo.StoragePod
			if err := collector.RetrieveOne(context.TODO(), *md.Parent, []string{"name"}, &msp); err != nil {
				log.Printf("[ERROR] %#v", err)
			}
			rootDatastore = msp.Name
			log.Printf("[DEBUG] %#v", msp.Name)
		} else {
			rootDatastore = md.Name
			log.Printf("[DEBUG] %#v", md.Name)
		}
		break
	}

	d.Set("datacenter", dc)
	d.Set("memory", mvm.Summary.Config.MemorySizeMB)
	d.Set("cpu", mvm.Summary.Config.NumCpu)
	d.Set("datastore", rootDatastore)

	// Initialize the connection info
	if len(networkInterfaces) > 0 {
		d.SetConnInfo(map[string]string{
			"type": "ssh",
			"host": networkInterfaces[0]["ip_address"].(string),
		})
	} else {
		log.Printf("[ERROR] Could not find a default network interface")
	}

	return nil
}

func resourceVSphereVirtualMachineUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceVSphereVirtualMachineDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govmomi.Client)
	dc, err := getDatacenter(client, d.Get("datacenter").(string))
	if err != nil {
		return err
	}
	finder := find.NewFinder(client.Client, true)
	finder = finder.SetDatacenter(dc)

	vm, err := finder.VirtualMachine(context.TODO(), d.Get("name").(string))
	if err != nil {
		return err
	}

	log.Printf("[INFO] Deleting virtual machine: %s", d.Id())

	task, err := vm.PowerOff(context.TODO())
	if err != nil {
		return err
	}

	err = task.Wait(context.TODO())
	if err != nil {
		return err
	}

	task, err = vm.Destroy(context.TODO())
	if err != nil {
		return err
	}

	err = task.Wait(context.TODO())
	if err != nil {
		return err
	}

	d.SetId("")
	return nil
}

func waitForNetworkingActive(client *govmomi.Client, datacenter, name string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		dc, err := getDatacenter(client, datacenter)
		if err != nil {
			log.Printf("[ERROR] %#v", err)
			return nil, "", err
		}
		finder := find.NewFinder(client.Client, true)
		finder = finder.SetDatacenter(dc)

		vm, err := finder.VirtualMachine(context.TODO(), name)
		if err != nil {
			log.Printf("[ERROR] %#v", err)
			return nil, "", err
		}

		var mvm mo.VirtualMachine
		collector := property.DefaultCollector(client.Client)
		if err := collector.RetrieveOne(context.TODO(), vm.Reference(), []string{"summary"}, &mvm); err != nil {
			log.Printf("[ERROR] %#v", err)
			return nil, "", err
		}

		if mvm.Summary.Guest.IpAddress != "" {
			log.Printf("[DEBUG] IP address with DHCP: %v", mvm.Summary.Guest.IpAddress)
			return mvm.Summary, "active", err
		} else {
			log.Printf("[DEBUG] Waiting for IP address")
			return nil, "pending", err
		}
	}
}

// getDatacenter gets datacenter object
func getDatacenter(c *govmomi.Client, dc string) (*object.Datacenter, error) {
	finder := find.NewFinder(c.Client, true)
	if dc != "" {
		d, err := finder.Datacenter(context.TODO(), dc)
		return d, err
	} else {
		d, err := finder.DefaultDatacenter(context.TODO())
		return d, err
	}
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

// getDatastoreObject gets datastore object.
func getDatastoreObject(client *govmomi.Client, f *object.DatacenterFolders, name string) (types.ManagedObjectReference, error) {
	s := object.NewSearchIndex(client.Client)
	ref, err := s.FindChild(context.TODO(), f.DatastoreFolder, name)
	if err != nil {
		return types.ManagedObjectReference{}, err
	}
	log.Printf("[DEBUG] getDatastoreObject: reference: %#v", ref)
	return ref.Reference(), nil
}

// createStoragePlacementSpecCreate creates StoragePlacementSpec for create action.
func createStoragePlacementSpecCreate(f *object.DatacenterFolders, rp *object.ResourcePool, storagePod object.StoragePod, configSpec types.VirtualMachineConfigSpec) types.StoragePlacementSpec {
	vmfr := f.VmFolder.Reference()
	rpr := rp.Reference()
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
	return sps
}

// createStoragePlacementSpecClone creates StoragePlacementSpec for clone action.
func createStoragePlacementSpecClone(c *govmomi.Client, f *object.DatacenterFolders, vm *object.VirtualMachine, rp *object.ResourcePool, storagePod object.StoragePod) types.StoragePlacementSpec {
	vmr := vm.Reference()
	vmfr := f.VmFolder.Reference()
	rpr := rp.Reference()
	spr := storagePod.Reference()

	var o mo.VirtualMachine
	err := vm.Properties(context.TODO(), vmr, []string{"datastore"}, &o)
	if err != nil {
		return types.StoragePlacementSpec{}
	}
	ds := object.NewDatastore(c.Client, o.Datastore[0])
	log.Printf("[DEBUG] findDatastore: datastore: %#v\n", ds)

	devices, err := vm.Device(context.TODO())
	if err != nil {
		return types.StoragePlacementSpec{}
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
	return sps
}

// findDatastore finds Datastore object.
func findDatastore(c *govmomi.Client, sps types.StoragePlacementSpec) (*object.Datastore, error) {
	var datastore *object.Datastore
	log.Printf("[DEBUG] findDatastore: StoragePlacementSpec: %#v\n", sps)

	srm := object.NewStorageResourceManager(c.Client)
	rds, err := srm.RecommendDatastores(context.TODO(), sps)
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] findDatastore: recommendDatastores: %#v\n", rds)

	spa := rds.Recommendations[0].Action[0].(*types.StoragePlacementAction)
	datastore = object.NewDatastore(c.Client, spa.Destination)
	log.Printf("[DEBUG] findDatastore: datastore: %#v", datastore)

	return datastore, nil
}

// createVirtualMchine creates a new VirtualMachine.
func (vm *virtualMachine) createVirtualMachine(c *govmomi.Client) error {
	dc, err := getDatacenter(c, vm.datacenter)
	if err != nil {
		return err
	}
	finder := find.NewFinder(c.Client, true)
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
		datastore, err = finder.Datastore(context.TODO(), vm.datastore)
		if err != nil {
			// TODO: datastore cluster support in govmomi finder function
			d, err := getDatastoreObject(c, dcFolders, vm.datastore)
			if err != nil {
				return err
			}

			if d.Type == "StoragePod" {
				sp := object.StoragePod{
					object.NewFolder(c.Client, d),
				}
				sps := createStoragePlacementSpecCreate(dcFolders, resourcePool, sp, configSpec)
				datastore, err = findDatastore(c, sps)
			} else {
				datastore = object.NewDatastore(c.Client, d)
			}
			if err != nil {
				return err
			}
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
	dc, err := getDatacenter(c, vm.datacenter)
	if err != nil {
		return err
	}
	finder := find.NewFinder(c.Client, true)
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
			d, err := getDatastoreObject(c, dcFolders, vm.datastore)
			if err != nil {
				return err
			}

			if d.Type == "StoragePod" {
				sp := object.StoragePod{
					object.NewFolder(c.Client, d),
				}
				sps := createStoragePlacementSpecClone(c, dcFolders, template, resourcePool, sp)
				datastore, err = findDatastore(c, sps)
			} else {
				datastore = object.NewDatastore(c.Client, d)
			}
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
			log.Printf("[DEBUG] subnet mask: %v", network.subnetMask)
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
				Name: strings.Split(vm.name, ".")[0],
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

	_, err = task.WaitForResult(context.TODO(), nil)
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
